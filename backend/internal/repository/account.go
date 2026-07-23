// Package repository is the only layer that touches PostgreSQL (BACKEND.md §7).
package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/model"
)

// AccountRepo manages public.accounts and account_memberships.
type AccountRepo struct {
	db bun.IDB
}

// NewAccountRepo constructs an AccountRepo.
func NewAccountRepo(db bun.IDB) *AccountRepo {
	return &AccountRepo{db: db}
}

// WithDB returns a copy that uses db (e.g. a transaction).
func (r *AccountRepo) WithDB(db bun.IDB) *AccountRepo {
	return &AccountRepo{db: db}
}

// CreateAccount inserts a new tenant account.
func (r *AccountRepo) CreateAccount(ctx context.Context, name string) (*model.Account, error) {
	now := time.Now().UTC()
	acct := &model.Account{
		ID:        uuid.New(),
		Name:      name,
		Status:    model.AccountActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := r.db.NewInsert().Model(acct).Exec(ctx); err != nil {
		return nil, err
	}
	return acct, nil
}

// GetAccountByID returns an account or sql.ErrNoRows.
func (r *AccountRepo) GetAccountByID(ctx context.Context, id uuid.UUID) (*model.Account, error) {
	acct := new(model.Account)
	err := r.db.NewSelect().Model(acct).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return acct, nil
}

// CreateMembership inserts a membership row.
func (r *AccountRepo) CreateMembership(ctx context.Context, m *model.AccountMembership) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	now := time.Now().UTC()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	if m.Role == "" {
		m.Role = model.RoleCustomer
	}
	if m.Status == "" {
		m.Status = model.MembershipActive
	}
	_, err := r.db.NewInsert().Model(m).Exec(ctx)
	return err
}

// GetMembershipByUserID returns the unique membership for a user.
func (r *AccountRepo) GetMembershipByUserID(ctx context.Context, userID string) (*model.AccountMembership, error) {
	m := new(model.AccountMembership)
	err := r.db.NewSelect().Model(m).Where("user_id = ?", userID).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// GetMembershipByID returns a membership by primary key.
func (r *AccountRepo) GetMembershipByID(ctx context.Context, id uuid.UUID) (*model.AccountMembership, error) {
	m := new(model.AccountMembership)
	err := r.db.NewSelect().Model(m).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// ListMemberships returns paginated memberships ordered by created_at desc.
func (r *AccountRepo) ListMemberships(ctx context.Context, limit, offset int) ([]model.AccountMembership, int, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var rows []model.AccountMembership
	count, err := r.db.NewSelect().Model(&rows).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		ScanAndCount(ctx)
	if err != nil {
		return nil, 0, err
	}
	return rows, count, nil
}

// UpdateMembershipRoleStatus updates role and/or status.
func (r *AccountRepo) UpdateMembershipRoleStatus(ctx context.Context, id uuid.UUID, role, status string) (*model.AccountMembership, error) {
	m, err := r.GetMembershipByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if role != "" {
		m.Role = role
	}
	if status != "" {
		m.Status = status
	}
	m.UpdatedAt = time.Now().UTC()
	_, err = r.db.NewUpdate().Model(m).
		Column("role", "status", "updated_at").
		WherePK().
		Exec(ctx)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// CountAdmins returns the number of active admin memberships.
func (r *AccountRepo) CountAdmins(ctx context.Context) (int, error) {
	return r.db.NewSelect().
		Model((*model.AccountMembership)(nil)).
		Where("role = ?", model.RoleAdmin).
		Where("status = ?", model.MembershipActive).
		Count(ctx)
}

// EnsureMembership creates account+membership if the user has none.
// Safe for retries: UNIQUE(user_id) makes concurrent inserts converge.
func (r *AccountRepo) EnsureMembership(ctx context.Context, userID, accountName string) (*model.AccountMembership, *model.Account, error) {
	existing, err := r.GetMembershipByUserID(ctx, userID)
	if err == nil {
		acct, aerr := r.GetAccountByID(ctx, existing.AccountID)
		if aerr != nil {
			return nil, nil, aerr
		}
		return existing, acct, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, err
	}

	if accountName == "" {
		accountName = "Workspace"
	}

	// Prefer a real transaction when the handle supports it.
	type beginTx interface {
		BeginTx(context.Context, *sql.TxOptions) (bun.Tx, error)
	}
	if bt, ok := r.db.(beginTx); ok {
		tx, err := bt.BeginTx(ctx, nil)
		if err != nil {
			return nil, nil, err
		}
		defer tx.Rollback() //nolint:errcheck

		txRepo := r.WithDB(tx)
		// Re-check inside the tx.
		existing, err = txRepo.GetMembershipByUserID(ctx, userID)
		if err == nil {
			acct, aerr := txRepo.GetAccountByID(ctx, existing.AccountID)
			if aerr != nil {
				return nil, nil, aerr
			}
			if err := tx.Commit(); err != nil {
				return nil, nil, err
			}
			return existing, acct, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, nil, err
		}

		acct, err := txRepo.CreateAccount(ctx, accountName)
		if err != nil {
			return nil, nil, err
		}
		m := &model.AccountMembership{
			AccountID: acct.ID,
			UserID:    userID,
			Role:      model.RoleCustomer,
			Status:    model.MembershipActive,
		}
		if err := txRepo.CreateMembership(ctx, m); err != nil {
			// Concurrent insert: another worker won the race.
			existing, rerr := txRepo.GetMembershipByUserID(ctx, userID)
			if rerr == nil {
				acct2, aerr := txRepo.GetAccountByID(ctx, existing.AccountID)
				if aerr != nil {
					return nil, nil, aerr
				}
				_ = tx.Rollback()
				return existing, acct2, nil
			}
			return nil, nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, nil, err
		}
		return m, acct, nil
	}

	// Fallback without tx (tests with mocked IDB).
	acct, err := r.CreateAccount(ctx, accountName)
	if err != nil {
		return nil, nil, err
	}
	m := &model.AccountMembership{
		AccountID: acct.ID,
		UserID:    userID,
		Role:      model.RoleCustomer,
		Status:    model.MembershipActive,
	}
	if err := r.CreateMembership(ctx, m); err != nil {
		existing, rerr := r.GetMembershipByUserID(ctx, userID)
		if rerr == nil {
			acct2, aerr := r.GetAccountByID(ctx, existing.AccountID)
			if aerr != nil {
				return nil, nil, aerr
			}
			return existing, acct2, nil
		}
		return nil, nil, err
	}
	return m, acct, nil
}

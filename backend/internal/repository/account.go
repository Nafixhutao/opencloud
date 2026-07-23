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

const adminMutationLockKey int64 = 0x4f434c4f55444144

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

// UpdateAccountName updates one tenant account display name.
func (r *AccountRepo) UpdateAccountName(ctx context.Context, id uuid.UUID, name string) error {
	result, err := r.db.NewUpdate().
		Model((*model.Account)(nil)).
		Set("name = ?", name).
		Set("updated_at = now()").
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated == 0 {
		return sql.ErrNoRows
	}
	return nil
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

// GetMembershipByIDForUpdate locks a membership until the surrounding
// transaction ends.
func (r *AccountRepo) GetMembershipByIDForUpdate(ctx context.Context, id uuid.UUID) (*model.AccountMembership, error) {
	m := new(model.AccountMembership)
	err := r.db.NewSelect().Model(m).Where("id = ?", id).For("UPDATE").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// AdminUserRow is the explicit cross-schema projection used only by platform
// admin routes. It deliberately exposes no credential or token fields.
type AdminUserRow struct {
	MembershipID uuid.UUID `bun:"membership_id"`
	AccountID    uuid.UUID `bun:"account_id"`
	UserID       string    `bun:"user_id"`
	Role         string    `bun:"role"`
	Status       string    `bun:"status"`
	AccountName  string    `bun:"account_name"`
	UserName     string    `bun:"user_name"`
	UserEmail    string    `bun:"user_email"`
	CreatedAt    time.Time `bun:"created_at"`
	UpdatedAt    time.Time `bun:"updated_at"`
}

// ListAdminUsers returns paginated memberships with safe identity fields in one
// query. This avoids the former account lookup per row.
func (r *AccountRepo) ListAdminUsers(ctx context.Context, limit, offset int) ([]AdminUserRow, int, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := r.db.NewRaw(`SELECT count(*) FROM public.account_memberships`).Scan(ctx, &total); err != nil {
		return nil, 0, err
	}

	var rows []AdminUserRow
	err := r.db.NewRaw(`
		SELECT
			m.id AS membership_id,
			m.account_id,
			m.user_id,
			m.role,
			m.status,
			a.name AS account_name,
			COALESCE(u.name, '') AS user_name,
			COALESCE(u.email, '') AS user_email,
			m.created_at,
			m.updated_at
		FROM public.account_memberships AS m
		JOIN public.accounts AS a ON a.id = m.account_id
		LEFT JOIN auth."user" AS u ON u.id = m.user_id
		ORDER BY m.created_at DESC
		LIMIT ? OFFSET ?`, limit, offset).Scan(ctx, &rows)
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// GetAdminUser returns one safe platform-admin projection.
func (r *AccountRepo) GetAdminUser(ctx context.Context, id uuid.UUID) (*AdminUserRow, error) {
	row := new(AdminUserRow)
	err := r.db.NewRaw(`
		SELECT
			m.id AS membership_id,
			m.account_id,
			m.user_id,
			m.role,
			m.status,
			a.name AS account_name,
			COALESCE(u.name, '') AS user_name,
			COALESCE(u.email, '') AS user_email,
			m.created_at,
			m.updated_at
		FROM public.account_memberships AS m
		JOIN public.accounts AS a ON a.id = m.account_id
		LEFT JOIN auth."user" AS u ON u.id = m.user_id
		WHERE m.id = ?
		LIMIT 1`, id).Scan(ctx, row)
	if err != nil {
		return nil, err
	}
	return row, nil
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

// LockAdminMutations serializes all role/status/bootstrap writes that could
// remove the last active platform admin. The lock is transaction-scoped.
func (r *AccountRepo) LockAdminMutations(ctx context.Context) error {
	_, err := r.db.NewRaw(`SELECT pg_advisory_xact_lock(?)`, adminMutationLockKey).Exec(ctx)
	return err
}

// EnsureMembership creates account+membership if the user has none.
// PostgreSQL callers serialize per user and use ON CONFLICT, so a losing
// concurrent caller never queries an aborted transaction or leaves an orphan
// account.
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

	// bun.Tx also exposes BeginTx by translating it to a savepoint. Starting a
	// nested transaction here is unnecessary and, more importantly, can leave
	// the caller's transaction aborted if savepoint cleanup fails. Reuse the
	// service-owned transaction directly.
	if _, inTx := r.db.(bun.Tx); inTx {
		return r.ensureMembershipLocked(ctx, userID, accountName)
	}

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
		m, acct, err := txRepo.ensureMembershipLocked(ctx, userID, accountName)
		if err != nil {
			return nil, nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, nil, err
		}
		return m, acct, nil
	}

	// Non-transactional handles reach this path only in lightweight repository
	// fakes; production *bun.DB always takes the branch above.
	return r.ensureMembershipLocked(ctx, userID, accountName)
}

func (r *AccountRepo) ensureMembershipLocked(ctx context.Context, userID, accountName string) (*model.AccountMembership, *model.Account, error) {
	if _, err := r.db.NewRaw(`SELECT pg_advisory_xact_lock(hashtextextended(?, 0))`, userID).Exec(ctx); err != nil {
		return nil, nil, err
	}

	existing, err := r.GetMembershipByUserID(ctx, userID)
	if err == nil {
		acct, aerr := r.GetAccountByID(ctx, existing.AccountID)
		return existing, acct, aerr
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, err
	}

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
	now := time.Now().UTC()
	m.ID = uuid.New()
	m.CreatedAt = now
	m.UpdatedAt = now
	result, err := r.db.NewInsert().Model(m).
		On("CONFLICT (user_id) DO NOTHING").
		Exec(ctx)
	if err != nil {
		return nil, nil, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return nil, nil, err
	}
	if inserted == 0 {
		if _, err := r.db.NewDelete().Model(acct).WherePK().Exec(ctx); err != nil {
			return nil, nil, err
		}
		winner, err := r.GetMembershipByUserID(ctx, userID)
		if err != nil {
			return nil, nil, err
		}
		winnerAccount, err := r.GetAccountByID(ctx, winner.AccountID)
		return winner, winnerAccount, err
	}
	return m, acct, nil
}

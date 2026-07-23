// Package service holds business rules and multi-repo transactions.
package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

// AccountService owns tenant membership and profile/admin user management.
type AccountService struct {
	db    *bun.DB
	accts *repository.AccountRepo
	audit *repository.AuditRepo
}

// NewAccountService constructs an AccountService.
func NewAccountService(db *bun.DB, accts *repository.AccountRepo, audit *repository.AuditRepo) *AccountService {
	return &AccountService{db: db, accts: accts, audit: audit}
}

// Me is the caller profile returned by GET /api/v1/me.
type Me struct {
	UserID    string         `json:"user_id"`
	AccountID uuid.UUID      `json:"account_id"`
	Role      string         `json:"role"`
	Status    string         `json:"status"`
	Account   *model.Account `json:"account"`
}

// GetMe loads membership + account for the authenticated user.
// If membership is missing (legacy user), it is ensured as customer.
func (s *AccountService) GetMe(ctx context.Context, userID, displayName string) (*Me, error) {
	if userID == "" {
		return nil, apperr.Unauthenticated("missing user")
	}
	m, acct, err := s.accts.EnsureMembership(ctx, userID, displayName)
	if err != nil {
		return nil, apperr.Internal("failed to load membership").Wrap(err)
	}
	if m.Status == model.MembershipDisabled || m.Status == model.MembershipSuspended {
		return nil, apperr.Forbidden("account is " + m.Status)
	}
	return &Me{
		UserID:    m.UserID,
		AccountID: m.AccountID,
		Role:      m.Role,
		Status:    m.Status,
		Account:   acct,
	}, nil
}

// UpdateProfileRequest is the safe profile patch body.
type UpdateProfileRequest struct {
	Name string `json:"name"`
}

// UpdateProfile updates the tenant account display name for the caller's account.
func (s *AccountService) UpdateProfile(ctx context.Context, userID string, accountID uuid.UUID, req UpdateProfileRequest) (*Me, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, apperr.Validation("name is required", apperr.FieldIssue{Field: "name", Issue: "required"})
	}
	if len(name) > 100 {
		return nil, apperr.Validation("name must be at most 100 characters", apperr.FieldIssue{Field: "name", Issue: "max 100"})
	}

	var result *Me
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		accts := s.accts.WithDB(tx)
		audit := s.audit.WithDB(tx)
		m, err := accts.GetMembershipByUserID(ctx, userID)
		if err != nil {
			return err
		}
		// Tenant isolation: never update another account even if JWT was stale.
		if m.AccountID != accountID {
			return apperr.NotFound("account not found")
		}
		if m.Status != model.MembershipActive {
			return apperr.Forbidden("account is " + m.Status)
		}
		if err := accts.UpdateAccountName(ctx, accountID, name); err != nil {
			return err
		}
		actor := userID
		aid := accountID
		if err := audit.Append(ctx, repository.Entry{
			AccountID: &aid,
			ActorID:   &actor,
			Action:    model.AuditProfileUpdated,
			Target:    strPtr(accountID.String()),
			Metadata:  map[string]any{"fields": []string{"name"}},
		}); err != nil {
			return err
		}
		acct, err := accts.GetAccountByID(ctx, accountID)
		if err != nil {
			return err
		}
		result = &Me{
			UserID:    m.UserID,
			AccountID: m.AccountID,
			Role:      m.Role,
			Status:    m.Status,
			Account:   acct,
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("membership not found")
		}
		if apperr.As(err) != nil {
			return nil, err
		}
		return nil, apperr.Internal("failed to update profile").Wrap(err)
	}
	return result, nil
}

// AdminUser is the safe membership/account/identity projection for platform admins.
type AdminUser struct {
	MembershipID uuid.UUID `json:"membership_id"`
	UserID       string    `json:"user_id"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	AccountID    uuid.UUID `json:"account_id"`
	Role         string    `json:"role"`
	Status       string    `json:"status"`
	AccountName  string    `json:"account_name,omitempty"`
	CreatedAt    string    `json:"created_at"`
	UpdatedAt    string    `json:"updated_at"`
}

// ListUsers returns paginated memberships for a platform admin and records the
// cross-account read before any data is returned.
func (s *AccountService) ListUsers(ctx context.Context, actorUserID string, page, perPage int) ([]AdminUser, int, error) {
	if page < 1 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}
	offset := (page - 1) * perPage
	rows, total, err := s.accts.ListAdminUsers(ctx, perPage, offset)
	if err != nil {
		return nil, 0, apperr.Internal("failed to list users").Wrap(err)
	}
	out := make([]AdminUser, 0, len(rows))
	for _, row := range rows {
		out = append(out, adminUserFromRow(&row))
	}
	actor := actorUserID
	if err := s.audit.Append(ctx, repository.Entry{
		ActorID:  &actor,
		Action:   model.AuditAdminUsersListed,
		Target:   strPtr("account_memberships"),
		Metadata: map[string]any{"page": page, "per_page": perPage, "result_count": len(out)},
	}); err != nil {
		return nil, 0, apperr.Internal("failed to audit admin user list").Wrap(err)
	}
	return out, total, nil
}

// GetUser returns one membership for admin detail and audits the cross-account read.
func (s *AccountService) GetUser(ctx context.Context, actorUserID string, membershipID uuid.UUID) (*AdminUser, error) {
	row, err := s.accts.GetAdminUser(ctx, membershipID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("user not found")
		}
		return nil, apperr.Internal("failed to load user").Wrap(err)
	}
	actor := actorUserID
	if err := s.audit.Append(ctx, repository.Entry{
		AccountID: &row.AccountID,
		ActorID:   &actor,
		Action:    model.AuditAdminUserViewed,
		Target:    strPtr(row.UserID),
	}); err != nil {
		return nil, apperr.Internal("failed to audit admin user view").Wrap(err)
	}
	user := adminUserFromRow(row)
	return &user, nil
}

// UpdateUserRequest is the admin patch for role/status.
type UpdateUserRequest struct {
	Role   string `json:"role"`
	Status string `json:"status"`
}

// UpdateUser applies safe admin role/status changes.
func (s *AccountService) UpdateUser(ctx context.Context, actorUserID string, membershipID uuid.UUID, req UpdateUserRequest) (*AdminUser, error) {
	role := strings.TrimSpace(req.Role)
	status := strings.TrimSpace(req.Status)
	if role == "" && status == "" {
		return nil, apperr.Validation("role or status is required")
	}
	if role != "" && !model.ValidRole(role) {
		return nil, apperr.Validation("invalid role", apperr.FieldIssue{Field: "role", Issue: "must be customer or admin"})
	}
	if status != "" && !model.ValidMembershipStatus(status) {
		return nil, apperr.Validation("invalid status", apperr.FieldIssue{Field: "status", Issue: "must be active, suspended, or disabled"})
	}

	var result *AdminUser
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		accts := s.accts.WithDB(tx)
		audit := s.audit.WithDB(tx)
		if err := accts.LockAdminMutations(ctx); err != nil {
			return err
		}
		target, err := accts.GetMembershipByIDForUpdate(ctx, membershipID)
		if err != nil {
			return err
		}

		// Prevent self-lockout: admin cannot disable/suspend or demote themselves.
		if target.UserID == actorUserID {
			if status != "" && status != model.MembershipActive {
				return apperr.Conflict("cannot change your own status")
			}
			if role != "" && role != model.RoleAdmin {
				return apperr.Conflict("cannot demote yourself")
			}
		}

		demotingAdmin := target.Role == model.RoleAdmin &&
			target.Status == model.MembershipActive &&
			role == model.RoleCustomer
		disablingAdmin := target.Role == model.RoleAdmin &&
			target.Status == model.MembershipActive &&
			status != "" && status != model.MembershipActive
		if demotingAdmin || disablingAdmin {
			n, err := accts.CountAdmins(ctx)
			if err != nil {
				return err
			}
			if n <= 1 {
				return apperr.Conflict("cannot remove the last active admin")
			}
		}

		updated, err := accts.UpdateMembershipRoleStatus(ctx, membershipID, role, status)
		if err != nil {
			return err
		}
		actor := actorUserID
		aid := updated.AccountID
		if role != "" && role != target.Role {
			if err := audit.Append(ctx, repository.Entry{
				AccountID: &aid,
				ActorID:   &actor,
				Action:    model.AuditUserRoleChanged,
				Target:    strPtr(updated.UserID),
				Metadata:  map[string]any{"from": target.Role, "to": role},
			}); err != nil {
				return err
			}
		}
		if status != "" && status != target.Status {
			if err := audit.Append(ctx, repository.Entry{
				AccountID: &aid,
				ActorID:   &actor,
				Action:    model.AuditUserStatusChanged,
				Target:    strPtr(updated.UserID),
				Metadata:  map[string]any{"from": target.Status, "to": status},
			}); err != nil {
				return err
			}
		}
		row, err := accts.GetAdminUser(ctx, membershipID)
		if err != nil {
			return err
		}
		user := adminUserFromRow(row)
		result = &user
		return nil
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("user not found")
		}
		if apperr.As(err) != nil {
			return nil, err
		}
		return nil, apperr.Internal("failed to update user").Wrap(err)
	}
	return result, nil
}

// BootstrapAdmin promotes a user to admin by user_id (idempotent, explicit).
func (s *AccountService) BootstrapAdmin(ctx context.Context, userID string) (*AdminUser, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, apperr.Validation("user_id is required")
	}
	var result *AdminUser
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		accts := s.accts.WithDB(tx)
		audit := s.audit.WithDB(tx)
		if err := accts.LockAdminMutations(ctx); err != nil {
			return err
		}
		m, _, err := accts.EnsureMembership(ctx, userID, "Admin Workspace")
		if err != nil {
			return err
		}
		wasAdmin := m.Role == model.RoleAdmin && m.Status == model.MembershipActive
		if !wasAdmin {
			m, err = accts.UpdateMembershipRoleStatus(ctx, m.ID, model.RoleAdmin, model.MembershipActive)
			if err != nil {
				return err
			}
		}
		aid := m.AccountID
		actor := userID
		if err := audit.Append(ctx, repository.Entry{
			AccountID: &aid,
			ActorID:   &actor,
			Action:    model.AuditAdminBootstrap,
			Target:    strPtr(userID),
			Metadata:  map[string]any{"method": "bootstrap", "idempotent": wasAdmin},
		}); err != nil {
			return err
		}
		row, err := accts.GetAdminUser(ctx, m.ID)
		if err != nil {
			return err
		}
		user := adminUserFromRow(row)
		result = &user
		return nil
	})
	if err != nil {
		return nil, apperr.Internal("failed to bootstrap admin").Wrap(err)
	}
	return result, nil
}

func adminUserFromRow(row *repository.AdminUserRow) AdminUser {
	return AdminUser{
		MembershipID: row.MembershipID,
		UserID:       row.UserID,
		Name:         row.UserName,
		Email:        row.UserEmail,
		AccountID:    row.AccountID,
		Role:         row.Role,
		Status:       row.Status,
		AccountName:  row.AccountName,
		CreatedAt:    row.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    row.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

func strPtr(s string) *string { return &s }

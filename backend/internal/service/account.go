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

	m, err := s.accts.GetMembershipByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("membership not found")
		}
		return nil, apperr.Internal("failed to load membership").Wrap(err)
	}
	// Tenant isolation: never update another account even if JWT was somehow wrong.
	if m.AccountID != accountID {
		return nil, apperr.NotFound("account not found")
	}

	_, err = s.db.NewUpdate().
		Model((*model.Account)(nil)).
		Set("name = ?", name).
		Set("updated_at = now()").
		Where("id = ?", accountID).
		Exec(ctx)
	if err != nil {
		return nil, apperr.Internal("failed to update profile").Wrap(err)
	}

	actor := userID
	aid := accountID
	_ = s.audit.Append(ctx, repository.Entry{
		AccountID: &aid,
		ActorID:   &actor,
		Action:    model.AuditProfileUpdated,
		Target:    strPtr(accountID.String()),
		Metadata:  map[string]any{"name": name},
	})

	return s.GetMe(ctx, userID, name)
}

// AdminUser is a membership row for admin listings (identity fields filled by handler/BFF).
type AdminUser struct {
	MembershipID uuid.UUID `json:"membership_id"`
	UserID       string    `json:"user_id"`
	AccountID    uuid.UUID `json:"account_id"`
	Role         string    `json:"role"`
	Status       string    `json:"status"`
	AccountName  string    `json:"account_name,omitempty"`
	CreatedAt    string    `json:"created_at"`
	UpdatedAt    string    `json:"updated_at"`
}

// ListUsers returns paginated memberships for admin.
func (s *AccountService) ListUsers(ctx context.Context, page, perPage int) ([]AdminUser, int, error) {
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
	rows, total, err := s.accts.ListMemberships(ctx, perPage, offset)
	if err != nil {
		return nil, 0, apperr.Internal("failed to list users").Wrap(err)
	}
	out := make([]AdminUser, 0, len(rows))
	for _, m := range rows {
		acctName := ""
		if acct, aerr := s.accts.GetAccountByID(ctx, m.AccountID); aerr == nil {
			acctName = acct.Name
		}
		out = append(out, AdminUser{
			MembershipID: m.ID,
			UserID:       m.UserID,
			AccountID:    m.AccountID,
			Role:         m.Role,
			Status:       m.Status,
			AccountName:  acctName,
			CreatedAt:    m.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:    m.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	return out, total, nil
}

// GetUser returns one membership for admin detail.
func (s *AccountService) GetUser(ctx context.Context, membershipID uuid.UUID) (*AdminUser, error) {
	m, err := s.accts.GetMembershipByID(ctx, membershipID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("user not found")
		}
		return nil, apperr.Internal("failed to load user").Wrap(err)
	}
	acctName := ""
	if acct, aerr := s.accts.GetAccountByID(ctx, m.AccountID); aerr == nil {
		acctName = acct.Name
	}
	return &AdminUser{
		MembershipID: m.ID,
		UserID:       m.UserID,
		AccountID:    m.AccountID,
		Role:         m.Role,
		Status:       m.Status,
		AccountName:  acctName,
		CreatedAt:    m.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    m.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}, nil
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

	target, err := s.accts.GetMembershipByID(ctx, membershipID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("user not found")
		}
		return nil, apperr.Internal("failed to load user").Wrap(err)
	}

	// Prevent self-lockout: admin cannot disable/suspend or demote themselves.
	if target.UserID == actorUserID {
		if status != "" && status != model.MembershipActive {
			return nil, apperr.Conflict("cannot change your own status")
		}
		if role != "" && role != model.RoleAdmin {
			return nil, apperr.Conflict("cannot demote yourself")
		}
	}

	// Prevent removing the last active admin.
	demotingAdmin := target.Role == model.RoleAdmin && role == model.RoleCustomer
	disablingAdmin := target.Role == model.RoleAdmin &&
		target.Status == model.MembershipActive &&
		status != "" && status != model.MembershipActive
	if demotingAdmin || disablingAdmin {
		n, err := s.accts.CountAdmins(ctx)
		if err != nil {
			return nil, apperr.Internal("failed to count admins").Wrap(err)
		}
		if n <= 1 {
			return nil, apperr.Conflict("cannot remove the last active admin")
		}
	}

	updated, err := s.accts.UpdateMembershipRoleStatus(ctx, membershipID, role, status)
	if err != nil {
		return nil, apperr.Internal("failed to update user").Wrap(err)
	}

	actor := actorUserID
	aid := updated.AccountID
	if role != "" && role != target.Role {
		_ = s.audit.Append(ctx, repository.Entry{
			AccountID: &aid,
			ActorID:   &actor,
			Action:    model.AuditUserRoleChanged,
			Target:    strPtr(updated.UserID),
			Metadata:  map[string]any{"from": target.Role, "to": role},
		})
	}
	if status != "" && status != target.Status {
		_ = s.audit.Append(ctx, repository.Entry{
			AccountID: &aid,
			ActorID:   &actor,
			Action:    model.AuditUserStatusChanged,
			Target:    strPtr(updated.UserID),
			Metadata:  map[string]any{"from": target.Status, "to": status},
		})
	}

	return s.GetUser(ctx, membershipID)
}

// BootstrapAdmin promotes a user to admin by user_id (idempotent, explicit).
func (s *AccountService) BootstrapAdmin(ctx context.Context, userID string) (*AdminUser, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, apperr.Validation("user_id is required")
	}
	m, err := s.accts.GetMembershipByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Ensure a membership first so bootstrap works for brand-new users.
			m, _, err = s.accts.EnsureMembership(ctx, userID, "Admin Workspace")
			if err != nil {
				return nil, apperr.Internal("failed to ensure membership").Wrap(err)
			}
		} else {
			return nil, apperr.Internal("failed to load membership").Wrap(err)
		}
	}
	if m.Role == model.RoleAdmin && m.Status == model.MembershipActive {
		return s.GetUser(ctx, m.ID)
	}
	updated, err := s.accts.UpdateMembershipRoleStatus(ctx, m.ID, model.RoleAdmin, model.MembershipActive)
	if err != nil {
		return nil, apperr.Internal("failed to promote admin").Wrap(err)
	}
	aid := updated.AccountID
	actor := userID
	_ = s.audit.Append(ctx, repository.Entry{
		AccountID: &aid,
		ActorID:   &actor,
		Action:    model.AuditAdminBootstrap,
		Target:    strPtr(userID),
		Metadata:  map[string]any{"method": "bootstrap"},
	})
	return s.GetUser(ctx, updated.ID)
}

func strPtr(s string) *string { return &s }

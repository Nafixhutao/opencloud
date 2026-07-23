package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Well-known audit actions for Phase 1 auth/account events.
const (
	AuditLoginSuccess      = "auth.login.success"
	AuditLoginFailure      = "auth.login.failure"
	AuditPasswordResetReq  = "auth.password_reset.request"
	AuditPasswordResetDone = "auth.password_reset.complete"
	AuditPasswordChanged   = "auth.password.change"
	AuditProfileUpdated    = "account.profile.update"
	AuditUserRoleChanged   = "admin.user.role_change"
	AuditUserStatusChanged = "admin.user.status_change"
	AuditAdminUsersListed  = "admin.users.list"
	AuditAdminUserViewed   = "admin.user.view"
	AuditAdminBootstrap    = "admin.bootstrap"
	AuditMembershipEnsured = "account.membership.ensure"
)

// AuditLog is an append-only sensitive-action record (SECURITY §12).
type AuditLog struct {
	bun.BaseModel `bun:"table:audit_logs,alias:al"`

	ID        int64           `bun:"id,pk,autoincrement" json:"id"`
	AccountID *uuid.UUID      `bun:"account_id,type:uuid" json:"account_id,omitempty"`
	ActorID   *string         `bun:"actor_id" json:"actor_id,omitempty"`
	Action    string          `bun:"action,notnull" json:"action"`
	Target    *string         `bun:"target" json:"target,omitempty"`
	Metadata  json.RawMessage `bun:"metadata,type:jsonb,notnull,default:'{}'" json:"metadata"`
	CreatedAt time.Time       `bun:"created_at,notnull,default:now()" json:"created_at"`
}

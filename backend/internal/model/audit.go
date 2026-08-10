package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Well-known audit actions for Phase 1 auth/account events.
const (
	AuditLoginSuccess                = "auth.login.success"
	AuditLoginFailure                = "auth.login.failure"
	AuditPasswordResetReq            = "auth.password_reset.request"
	AuditPasswordResetDone           = "auth.password_reset.complete"
	AuditPasswordChanged             = "auth.password.change"
	AuditProfileUpdated              = "account.profile.update"
	AuditUserRoleChanged             = "admin.user.role_change"
	AuditUserStatusChanged           = "admin.user.status_change"
	AuditAdminUsersListed            = "admin.users.list"
	AuditAdminUserViewed             = "admin.user.view"
	AuditAdminBootstrap              = "admin.bootstrap"
	AuditMembershipEnsured           = "account.membership.ensure"
	AuditSiteCreateQueued            = "site.create.queued"
	AuditSiteProvisioned             = "site.provision.complete"
	AuditSiteSuspendQueued           = "site.suspend.queued"
	AuditSiteSuspended               = "site.suspend.complete"
	AuditSiteResumeQueued            = "site.resume.queued"
	AuditSiteResumed                 = "site.resume.complete"
	AuditSiteDeleteQueued            = "site.delete.queued"
	AuditSiteDeleted                 = "site.delete.complete"
	AuditSiteFailed                  = "site.lifecycle.failed"
	AuditSiteReconciled              = "site.reconcile"
	AuditSiteReconcileDeferred       = "site.reconcile.deferred"
	AuditNodeRegistered              = "admin.node.register"
	AuditAdminNodesListed            = "admin.nodes.list"
	AuditNodeStatusChanged           = "admin.node.status_change"
	AuditDatabaseCreateQueued        = "database.create.queued"
	AuditDatabaseProvisioned         = "database.provision.complete"
	AuditDatabaseCredentialRevealed  = "database.credentials.reveal"
	AuditDatabaseDeleteQueued        = "database.delete.queued"
	AuditDatabaseDeleted             = "database.delete.complete"
	AuditDatabaseFailed              = "database.lifecycle.failed"
	AuditDatabaseCleanupCompleted    = "database.cleanup.complete"
	AuditDatabaseProvisionSuperseded = "database.provision.superseded"

	// Storage bucket audit actions (Phase 4M).
	AuditStorageBucketCreateRequested = "storage.bucket.create_requested"
	AuditStorageBucketProvisioned     = "storage.bucket.provisioned"
	AuditStorageBucketProvisionFailed = "storage.bucket.provision_failed"
	AuditStorageBucketDeleteRequested = "storage.bucket.delete_requested"
	AuditStorageBucketDeleted         = "storage.bucket.deleted"
	AuditStorageBucketDeleteFailed    = "storage.bucket.delete_failed"
	AuditStorageBucketReconciled      = "storage.bucket.reconciled"
	AuditStorageBucketDeleteAttempted = "storage.bucket.delete_attempted"
	AuditStorageBucketDeleteBlocked   = "storage.bucket.delete_blocked"
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

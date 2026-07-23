package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Account is the tenant boundary (DATABASE.md §3).
type Account struct {
	bun.BaseModel `bun:"table:accounts,alias:a"`

	ID        uuid.UUID `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	Name      string    `bun:"name,notnull" json:"name"`
	Status    string    `bun:"status,notnull" json:"status"`
	CreatedAt time.Time `bun:"created_at,notnull,default:now()" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull,default:now()" json:"updated_at"`
}

// Membership roles enforced by DB CHECK and middleware.RequireRole.
const (
	RoleCustomer = "customer"
	RoleAdmin    = "admin"
)

// Membership statuses.
const (
	MembershipActive    = "active"
	MembershipSuspended = "suspended"
	MembershipDisabled  = "disabled"
)

// Account statuses.
const (
	AccountActive    = "active"
	AccountSuspended = "suspended"
	AccountClosed    = "closed"
)

// AccountMembership links an auth.user.id to a tenant account.
type AccountMembership struct {
	bun.BaseModel `bun:"table:account_memberships,alias:m"`

	ID        uuid.UUID `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	AccountID uuid.UUID `bun:"account_id,notnull,type:uuid" json:"account_id"`
	UserID    string    `bun:"user_id,notnull" json:"user_id"`
	Role      string    `bun:"role,notnull" json:"role"`
	Status    string    `bun:"status,notnull" json:"status"`
	CreatedAt time.Time `bun:"created_at,notnull,default:now()" json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull,default:now()" json:"updated_at"`
}

// ValidRole reports whether role is an allowed RBAC value.
func ValidRole(role string) bool {
	return role == RoleCustomer || role == RoleAdmin
}

// ValidMembershipStatus reports whether status is allowed.
func ValidMembershipStatus(status string) bool {
	switch status {
	case MembershipActive, MembershipSuspended, MembershipDisabled:
		return true
	default:
		return false
	}
}

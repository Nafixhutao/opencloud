package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// DatabaseConsoleSession represents a short-lived authenticated session for
// accessing a customer database via the OpenCloud console. The session does not
// store credentials; it is validated by the gateway before any database access.
type DatabaseConsoleSession struct {
	bun.BaseModel `bun:"table:database_console_sessions,alias:dcs"`

	ID         uuid.UUID  `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	AccountID  uuid.UUID  `bun:"account_id,notnull,type:uuid" json:"-"`
	DatabaseID uuid.UUID  `bun:"database_id,notnull,type:uuid" json:"-"`
	ActorID    string     `bun:"actor_id,notnull,type:text" json:"-"`
	Engine     string     `bun:"engine,notnull" json:"-"`
	Status     string     `bun:"status,notnull" json:"-"`
	ExpiresAt  time.Time  `bun:"expires_at,notnull" json:"-"`
	CreatedAt  time.Time  `bun:"created_at,notnull,default:now()" json:"-"`
	RevokedAt  *time.Time `bun:"revoked_at" json:"-"`
}

// Console session lifecycle states.
const (
	ConsoleSessionActive = "active"
	ConsoleSessionExpired = "expired"
	ConsoleSessionRevoked = "revoked"
)

// Session duration constants.
const (
	DefaultSessionTTL = 30 * time.Minute // 30 minutes default
	MinSessionTTL     = 15 * time.Minute // minimum allowed
	MaxSessionTTL     = 60 * time.Minute // maximum allowed
)

package model

import (
	"time"
)

// DatabaseConsoleSession states.
const (
	ConsoleSessionActive  = "active"
	ConsoleSessionRevoked = "revoked"
	ConsoleSessionExpired = "expired"
)

// DatabaseEngine values supported by the console.
const (
	EnginePostgres = "postgres"
	EngineMariaDB  = "mariadb"
)

// DatabaseConsoleSession represents a short-lived, account-scoped console
// session used to open the database manager or SQL console.
type DatabaseConsoleSession struct {
	ID             string     `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	AccountID      string     `json:"account_id" bun:",type:uuid,notnull"`
	ActorID        string     `json:"actor_id" bun:",type:text"`
	DatabaseID     string     `json:"database_id" bun:",type:uuid,notnull"`
	Engine         string     `json:"engine" bun:",type:varchar(20),notnull"`
	Status         string     `json:"status" bun:",type:varchar(20),notnull"`
	CreatedAt      time.Time  `json:"created_at" bun:",type:timestamp,timezone,notnull,default:now()"`
	ExpiresAt      time.Time  `json:"expires_at" bun:",type:timestamp,timezone,notnull"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty" bun:",type:timestamp,timezone"`
	IPAddr         *string    `json:"ip_addr,omitempty" bun:",type:varchar(45)"`
	UserAgent      *string    `json:"user_agent,omitempty" bun:",type:text"`
	SessionToken   string     `json:"-" bun:",type:varchar(64),notnull,unique"`
	LastActivityAt *time.Time `json:"last_activity_at,omitempty" bun:",type:timestamp,timezone"`
}

// TableName returns the database table name
func (DatabaseConsoleSession) TableName() string {
	return "database_console_sessions"
}

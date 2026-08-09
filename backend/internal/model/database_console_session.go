package model

import (
	"time"
)

// DatabaseConsoleSession represents a database console session for security auditing
type DatabaseConsoleSession struct {
	ID             string     `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	AccountID      string     `json:"account_id" bun:",type:uuid,notnull,unique:index:sessions"`
	DatabaseID     string     `json:"database_id" bun:",type:uuid,notnull"`
	CreatedAt      time.Time  `json:"created_at" bun:",type:timestamp,timezone,notnull,default:now()"`
	ExpiresAt      time.Time  `json:"expires_at" bun:",type:timestamp,timezone,notnull"`
	IPAddr         *string    `json:"ip_addr,omitempty" bun:",type:varchar(45)"`
	UserAgent      *string    `json:"user_agent,omitempty" bun:",type:text"`
	SessionToken   string     `json:"-" bun:",type:varchar(64),notnull,unique"`
	LastActivityAt *time.Time `json:"last_activity_at,omitempty" bun:",type:timestamp,timezone"`
}

// TableName returns the database table name
func (DatabaseConsoleSession) TableName() string {
	return "database_console_sessions"
}

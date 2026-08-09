package model

import "time"

// ConsoleQuery represents a logged SQL query from console session.
type ConsoleQueryAudit struct {
	ID           string    `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	AccountID    string    `bun:"account_id,notnull,type:text" json:"-"`
	DatabaseID   string    `bun:"database_id,notnull,type:text" json:"-"`
	SessionID    string    `bun:"session_id,notnull,type:text" json:"-"`
	ActorID      string    `bun:"actor_id,notnull,type:text" json:"-"`
	QueryHash    string    `bun:"query_hash,notnull" json:"-"` // SHA-256 hash only, not plaintext
	StatementType string   `bun:"statement_type,notnull" json:"-"` // SELECT, INSERT, UPDATE, DELETE, etc.
	DurationMs   int64     `bun:"duration_ms" json:"-"`
	AffectedRows int64     `bun:"affected_rows" json:"-"`
	Status       string    `bun:"status,notnull" json:"-"` // success, error, timeout
	CreatedAt    time.Time `bun:"created_at,notnull,default:now()" json:"-"`
}

const (
	// QueryStatusSuccess indicates successful completion
	QueryStatusSuccess = "success"
	// QueryStatusError indicates query failed
	QueryStatusError = "error"
	// QueryStatusTimeout indicates query exceeded timeout
	QueryStatusTimeout = "timeout"
)

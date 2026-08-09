package model

import (
	"time"
)

// ConsoleQueryAudit statement types.
const (
	StatementSelect   = "select"
	StatementExplain  = "explain"
	StatementShow     = "show"
	StatementDescribe = "describe"
	StatementUnknown  = "unknown"
)

// ConsoleQueryAudit status values.
const (
	AuditStatusSuccess = "success"
	AuditStatusError   = "error"
	AuditStatusBlocked = "blocked"
)

// ConsoleQueryAudit stores safe metadata about SQL queries executed in the
// database console. The query body is never stored; only a hash is retained.
type ConsoleQueryAudit struct {
	ID            string    `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	AccountID     string    `json:"account_id" bun:",type:uuid,notnull"`
	ActorID       string    `json:"actor_id" bun:",type:text"`
	SessionID     string    `json:"session_id" bun:",type:uuid,notnull"`
	DatabaseID    string    `json:"database_id" bun:",type:uuid,notnull"`
	Engine        string    `json:"engine" bun:",type:varchar(20),notnull"`
	StatementType string    `json:"statement_type" bun:",type:varchar(20),notnull"`
	QueryHash     string    `json:"query_hash" bun:",type:varchar(64),notnull"`
	QueryLength   int       `json:"query_length" bun:",type:int,notnull"`
	Status        string    `json:"status" bun:",type:varchar(20),notnull"` // success, error, blocked
	ErrorMsg      *string   `json:"error_msg,omitempty" bun:",type:text"`
	RowsAffected  *int64    `json:"rows_affected,omitempty" bun:",type:int8"`
	ExecutionTime *float64  `json:"execution_time,omitempty" bun:",type:double precision"`
	CreatedAt     time.Time `json:"created_at" bun:",type:timestamp,timezone,notnull,default:now()"`
}

// TableName returns the database table name
func (ConsoleQueryAudit) TableName() string {
	return "console_query_audit"
}

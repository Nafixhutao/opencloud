package model

import (
	"time"
)

// ConsoleQueryAudit stores audit logs of SQL queries executed in database console
type ConsoleQueryAudit struct {
	ID            string     `json:"id" bun:",pk,type:uuid,default:gen_random_uuid()"`
	AccountID     string     `json:"account_id" bun:",type:uuid,notnull"`
	SessionID     string     `json:"session_id" bun:",type:uuid,notnull,unique:index:audit"`
	DatabaseID    string     `json:"database_id" bun:",type:uuid,notnull"`
	QueryHash     string     `json:"query_hash" bun:",type:varchar(64),notnull,index:index:audit"`
	QueryLength   int        `json:"query_length" bun:",type:int,notnull"`
	Status        string     `json:"status" bun:",type:varchar(20),notnull"` // success, error, blocked
	ErrorMsg      *string    `json:"error_msg,omitempty" bun:",type:text"`
	RowsAffected  *int64     `json:"rows_affected,omitempty" bun:",type:int8"`
	ExecutionTime *float64   `json:"execution_time,omitempty" bun:",type:double precision"`
	CreatedAt     time.Time  `json:"created_at" bun:",type:timestamp,timezone,notnull,default:now()"`
}

// TableName returns the database table name
func (ConsoleQueryAudit) TableName() string {
	return "console_query_audit"
}

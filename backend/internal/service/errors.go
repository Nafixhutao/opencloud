package service

import (
	"errors"
)

var (
	ErrQueryExecutionFailed    = errors.New("query execution failed")
	ErrQueryTimeout            = errors.New("query timed out")
	ErrQueryTooLarge           = errors.New("query too large")
	ErrMultiStatementDisabled  = errors.New("multi-statement queries disabled")
)

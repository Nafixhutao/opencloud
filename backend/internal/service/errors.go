package service

import (
	"errors"
	"time"
)

// ErrInvalidSession indicates an invalid or expired session
var ErrInvalidSession = &ServiceError{Code: "INVALID_SESSION", Message: "Invalid or expired session"}

// ErrSessionExpired indicates session has expired
var ErrSessionExpired = &ServiceError{Code: "SESSION_EXPIRED", Message: "Session has expired"}

// ErrOperationNotAllowed indicates operation is not allowed
var ErrOperationNotAllowed = &ServiceError{Code: "OPERATION_NOT_ALLOWED", Message: "This SQL operation is not allowed in the console"}

// ErrQueryTooLong indicates query exceeds maximum length
var ErrQueryTooLong = &ServiceError{Code: "QUERY_TOO_LONG", Message: "Query exceeds maximum allowed length"}

// ErrMultipleStatementsNotAllowed indicates multiple statements detected
var ErrMultipleStatementsNotAllowed = &ServiceError{Code: "MULTIPLE_STATEMENTS", Message: "Multiple SQL statements are not allowed"}

// ServiceError represents a service-level error
type ServiceError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ServiceError) Error() string {
	return e.Message
}

func timeNow() time.Time {
	return time.Now().UTC()
}

func wrapErr(err error, msg string) error {
	if err == nil {
		return nil
	}
	return errors.New(msg + ": " + err.Error())
}

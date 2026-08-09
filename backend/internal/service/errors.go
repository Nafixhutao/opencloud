package service

import (
	"errors"
)

// Error represents a service-level error with a machine-readable code.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	return e.Message
}

// ErrInvalidSession indicates an invalid or expired session
var ErrInvalidSession = &Error{Code: "INVALID_SESSION", Message: "Invalid or expired session"}

// ErrSessionExpired indicates session has expired
var ErrSessionExpired = &Error{Code: "SESSION_EXPIRED", Message: "Session has expired"}

// ErrOperationNotAllowed indicates operation is not allowed
var ErrOperationNotAllowed = &Error{Code: "OPERATION_NOT_ALLOWED", Message: "This SQL operation is not allowed in the console"}

// ErrQueryTooLong indicates query exceeds maximum length
var ErrQueryTooLong = &Error{Code: "QUERY_TOO_LONG", Message: "Query exceeds maximum allowed length"}

// ErrMultipleStatementsNotAllowed indicates multiple statements detected
var ErrMultipleStatementsNotAllowed = &Error{Code: "MULTIPLE_STATEMENTS", Message: "Multiple SQL statements are not allowed"}

func wrapErr(err error, msg string) error {
	if err == nil {
		return nil
	}
	return errors.New(msg + ": " + err.Error())
}

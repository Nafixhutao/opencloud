// Package apperr defines typed application errors mapped to HTTP envelopes
// (API.md §5, BACKEND.md §10).
package apperr

import (
	"errors"
	"fmt"
	"net/http"
)

// Error is a stable, machine-readable application error.
type Error struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Status  int            `json:"-"`
	Details []FieldIssue   `json:"details,omitempty"`
	cause   error
}

// FieldIssue describes a validation problem for one field.
type FieldIssue struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.cause)
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// New builds an Error with the given status/code/message.
func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// Wrap annotates an Error with a cause without changing the public message.
func (e *Error) Wrap(err error) *Error {
	if e == nil {
		return nil
	}
	clone := *e
	clone.cause = err
	return &clone
}

// WithDetails attaches field-level validation details.
func (e *Error) WithDetails(details ...FieldIssue) *Error {
	if e == nil {
		return nil
	}
	clone := *e
	clone.Details = append([]FieldIssue(nil), details...)
	return &clone
}

// Common constructors.
func Validation(msg string, details ...FieldIssue) *Error {
	return New(http.StatusUnprocessableEntity, "VALIDATION_FAILED", msg).WithDetails(details...)
}

func Unauthenticated(msg string) *Error {
	if msg == "" {
		msg = "authentication required"
	}
	return New(http.StatusUnauthorized, "UNAUTHENTICATED", msg)
}

func Forbidden(msg string) *Error {
	if msg == "" {
		msg = "insufficient permissions"
	}
	return New(http.StatusForbidden, "FORBIDDEN", msg)
}

func NotFound(msg string) *Error {
	if msg == "" {
		msg = "resource not found"
	}
	return New(http.StatusNotFound, "NOT_FOUND", msg)
}

func Conflict(msg string) *Error {
	if msg == "" {
		msg = "conflict"
	}
	return New(http.StatusConflict, "CONFLICT", msg)
}

func RateLimited(msg string) *Error {
	if msg == "" {
		msg = "too many requests"
	}
	return New(http.StatusTooManyRequests, "RATE_LIMITED", msg)
}

func Internal(msg string) *Error {
	if msg == "" {
		msg = "internal server error"
	}
	return New(http.StatusInternalServerError, "INTERNAL", msg)
}

// As extracts *Error from err, or nil.
func As(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}

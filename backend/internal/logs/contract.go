// Package logs defines the tenant-scoped customer log storage boundary.
// Implementations store log lines outside the control-plane PostgreSQL database.
package logs

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// SourceBuild identifies output emitted while an image is built.
	SourceBuild Source = "build"
	// SourceRuntime identifies application stdout and stderr.
	SourceRuntime Source = "runtime"
	// SourceRequest identifies safe ingress request records.
	SourceRequest Source = "request"
	// SourcePlatform identifies OpenCloud lifecycle activity.
	SourcePlatform Source = "platform"
)

const (
	// LevelDebug identifies diagnostic output.
	LevelDebug Level = "debug"
	// LevelInfo identifies normal application output.
	LevelInfo Level = "info"
	// LevelWarn identifies degraded or unexpected behavior.
	LevelWarn Level = "warn"
	// LevelError identifies failed work.
	LevelError Level = "error"
)

var (
	// ErrUnavailable indicates that the external log store is disabled or unhealthy.
	ErrUnavailable = errors.New("log store unavailable")
	// ErrInvalidFilter prevents an unsafe or unbounded storage query.
	ErrInvalidFilter = errors.New("invalid log filter")
)

// Source is one customer-visible log origin.
type Source string

// Level is a normalized customer-visible severity.
type Level string

// RequestMetadata contains the safe subset of an ingress request record.
type RequestMetadata struct {
	RequestID    string  `json:"request_id,omitempty"`
	Method       string  `json:"method,omitempty"`
	Path         string  `json:"path,omitempty"`
	Status       int     `json:"status,omitempty"`
	DurationMS   float64 `json:"duration_ms,omitempty"`
	ResponseSize int64   `json:"response_size,omitempty"`
}

// Entry is one customer-visible log line. AccountID is deliberately absent;
// the API never echoes its server-side tenant selector back to the browser.
type Entry struct {
	Timestamp    time.Time        `json:"timestamp"`
	Source       Source           `json:"source"`
	Level        Level            `json:"level,omitempty"`
	Message      string           `json:"message"`
	ServiceID    *uuid.UUID       `json:"service_id,omitempty"`
	DeploymentID *uuid.UUID       `json:"deployment_id,omitempty"`
	Environment  string           `json:"environment,omitempty"`
	Request      *RequestMetadata `json:"request,omitempty"`
}

// Filter is the complete trusted query sent to a log store. AccountID and
// ProjectID must originate from authenticated server-side ownership checks.
type Filter struct {
	AccountID    uuid.UUID
	ProjectID    uuid.UUID
	ServiceID    *uuid.UUID
	DeploymentID *uuid.UUID
	Sources      []Source
	Levels       []Level
	Environment  string
	Search       string
	Start        time.Time
	End          time.Time
	Limit        int
}

// Validate ensures every provider request remains tenant-scoped and bounded.
func (f Filter) Validate() error {
	if f.AccountID == uuid.Nil || f.ProjectID == uuid.Nil {
		return ErrInvalidFilter
	}
	if f.ServiceID != nil && *f.ServiceID == uuid.Nil {
		return ErrInvalidFilter
	}
	if f.DeploymentID != nil && (f.ServiceID == nil || *f.DeploymentID == uuid.Nil) {
		return ErrInvalidFilter
	}
	if f.Start.IsZero() || f.End.IsZero() || f.End.Before(f.Start) || f.Limit < 1 || f.Limit > 1000 {
		return ErrInvalidFilter
	}
	if len(f.Search) > 256 || len(f.Environment) > 32 {
		return ErrInvalidFilter
	}
	for _, source := range f.Sources {
		if !source.Valid() {
			return ErrInvalidFilter
		}
	}
	for _, level := range f.Levels {
		if !level.Valid() {
			return ErrInvalidFilter
		}
	}
	return nil
}

// Valid reports whether a source is customer-visible.
func (s Source) Valid() bool {
	return s == SourceBuild || s == SourceRuntime || s == SourceRequest || s == SourcePlatform
}

// Valid reports whether a level is normalized and customer-visible.
func (l Level) Valid() bool {
	return l == LevelDebug || l == LevelInfo || l == LevelWarn || l == LevelError
}

// Subscription is a live log feed. Close is idempotent and detaches the
// provider even if the caller disconnects before its context is cancelled.
type Subscription struct {
	Entries <-chan Entry
	Errors  <-chan error
	Close   func()
}

// Store persists and tails customer logs outside PostgreSQL.
type Store interface {
	Query(context.Context, Filter) ([]Entry, error)
	Tail(context.Context, Filter) (Subscription, error)
}

// UnavailableStore fails closed when the optional external store is disabled.
type UnavailableStore struct{}

// Query reports that no external log store is configured.
func (UnavailableStore) Query(context.Context, Filter) ([]Entry, error) {
	return nil, ErrUnavailable
}

// Tail reports that no external log store is configured.
func (UnavailableStore) Tail(context.Context, Filter) (Subscription, error) {
	return Subscription{}, ErrUnavailable
}

// Sanitizer redacts common credential forms immediately before customer
// delivery. Collection pipelines must also redact; this is the fail-safe API
// boundary and does not make arbitrary secret logging acceptable.
type Sanitizer struct {
	authorization *regexp.Regexp
	credential    *regexp.Regexp
}

// NewSanitizer builds the fixed redaction rules used by list and live APIs.
func NewSanitizer() *Sanitizer {
	return &Sanitizer{
		authorization: regexp.MustCompile(`(?i)(authorization\s*[:=]\s*(?:bearer|basic)\s+)[^\s,;]+`),
		credential:    regexp.MustCompile(`(?i)((?:password|passwd|secret|api[_-]?key|access[_-]?token|refresh[_-]?token)\s*[:=]\s*)[^\s,;]+`),
	}
}

// Entry returns a copy safe for customer delivery.
func (s *Sanitizer) Entry(entry Entry) Entry {
	entry.Message = s.Text(entry.Message)
	if entry.Request != nil {
		request := *entry.Request
		request.Path = safeRequestPath(request.Path)
		entry.Request = &request
	}
	return entry
}

// Text redacts fixed credential patterns from untrusted application output.
func (s *Sanitizer) Text(value string) string {
	value = s.authorization.ReplaceAllString(value, `${1}[REDACTED]`)
	return s.credential.ReplaceAllString(value, `${1}[REDACTED]`)
}

func safeRequestPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.ParseRequestURI(value)
	if err == nil && parsed.Path != "" {
		return parsed.EscapedPath()
	}
	if index := strings.IndexAny(value, "?#"); index >= 0 {
		value = value[:index]
	}
	if !strings.HasPrefix(value, "/") {
		return ""
	}
	return value
}

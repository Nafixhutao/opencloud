// Package provisioner defines the hosting capabilities consumed by services.
// Concrete backends must be idempotent because jobs are retried after ambiguous
// failures and worker restarts.
package provisioner

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Backend identifies a hosting implementation without leaking its details into
// services or handlers.
type Backend string

const (
	// BackendDocker provisions isolated containers and Caddy routes.
	BackendDocker Backend = "docker"
	// BackendHestia provisions resources through a Hestia node.
	BackendHestia Backend = "hestia"
	// BackendFake is restricted to development and tests.
	BackendFake Backend = "fake"
)

// ParseBackend normalizes and validates a configured backend. Empty values use
// Docker, the accepted MVP default from ADR 0008.
func ParseBackend(value string) (Backend, error) {
	backend := Backend(strings.ToLower(strings.TrimSpace(value)))
	if backend == "" {
		return BackendDocker, nil
	}

	switch backend {
	case BackendDocker, BackendHestia, BackendFake:
		return backend, nil
	default:
		return "", fmt.Errorf("unsupported PROVISIONER_BACKEND %q", value)
	}
}

// SiteSpec is the provider-neutral input for creating a site. Runtime-specific
// policy is resolved before the request reaches a backend implementation.
type SiteSpec struct {
	SiteID       uuid.UUID
	AccountID    uuid.UUID
	NodeID       uuid.UUID
	Domain       string
	Image        string
	InternalPort uint16
	MemoryBytes  int64
	NanoCPUs     int64
}

// SiteRef identifies a provisioned site without exposing a Docker container ID
// or Hestia username to service code.
type SiteRef struct {
	SiteID    uuid.UUID
	AccountID uuid.UUID
	NodeID    uuid.UUID
}

// SiteState is the backend-observed lifecycle state used for reconciliation.
type SiteState string

const (
	// SiteStateMissing means no backend resource exists for the site.
	SiteStateMissing SiteState = "missing"
	// SiteStateRunning means the site is provisioned and serving traffic.
	SiteStateRunning SiteState = "running"
	// SiteStateSuspended means the site exists but may not serve traffic.
	SiteStateSuspended SiteState = "suspended"
)

// SiteProvisioner is implemented by Docker/Caddy now and may be implemented by
// Hestia later without changing services or public API contracts.
type SiteProvisioner interface {
	CreateSite(ctx context.Context, spec SiteSpec) error
	DeleteSite(ctx context.Context, ref SiteRef) error
	SuspendSite(ctx context.Context, ref SiteRef) error
	ResumeSite(ctx context.Context, ref SiteRef) error
	SiteStatus(ctx context.Context, ref SiteRef) (SiteState, error)
}

// ResourceName returns the deterministic backend resource prefix for a site.
// Deterministic names are part of the idempotency contract.
func ResourceName(siteID uuid.UUID) string {
	return "opencloud-site-" + siteID.String()
}

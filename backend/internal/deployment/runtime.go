// Package deployment defines the runtime deployment capability used after an
// OCI image is safely stored in the private registry. It is intentionally
// separate from the public API and the isolated builder.
package deployment

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/nazxf/opencloud/backend/internal/registry"
)

// Revision is the deterministic runtime identity of a deployment. Providers
// derive their private container/service names from DeploymentID and never
// expose native runtime handles through customer-facing APIs.
type Revision struct {
	AccountID    uuid.UUID
	ProjectID    uuid.UUID
	ServiceID    uuid.UUID
	DeploymentID uuid.UUID
	Artifact     registry.Artifact
}

// Validate ensures a runtime provider can operate only on a fully scoped,
// immutable deployment identity.
func (r Revision) Validate() error {
	if r.AccountID == uuid.Nil || r.ProjectID == uuid.Nil || r.ServiceID == uuid.Nil || r.DeploymentID == uuid.Nil {
		return errors.New("runtime deployment identity is incomplete")
	}
	if err := r.Artifact.Validate(); err != nil {
		return err
	}
	if r.Artifact.Repository.AccountID != r.AccountID ||
		r.Artifact.Repository.ProjectID != r.ProjectID ||
		r.Artifact.Repository.ServiceID != r.ServiceID {
		return errors.New("runtime artifact does not belong to deployment identity")
	}
	return nil
}

// TrafficSwitch identifies a candidate revision and optional currently-active
// revision. A Caddy-capable implementation must make this switch atomic or
// return an error that leaves the prior route intact.
type TrafficSwitch struct {
	Target   Revision
	Previous *Revision
}

// Validate prevents cross-service traffic switches.
func (s TrafficSwitch) Validate() error {
	if err := s.Target.Validate(); err != nil {
		return err
	}
	if s.Previous == nil {
		return nil
	}
	if err := s.Previous.Validate(); err != nil {
		return err
	}
	if s.Previous.AccountID != s.Target.AccountID ||
		s.Previous.ProjectID != s.Target.ProjectID ||
		s.Previous.ServiceID != s.Target.ServiceID {
		return errors.New("traffic switch revisions belong to different services")
	}
	if s.Previous.DeploymentID == s.Target.DeploymentID {
		return errors.New("traffic switch target is already active")
	}
	return nil
}

// RuntimeProvider is implemented only in the restricted runtime deployment
// worker. Start, health, Caddy traffic switch, and retirement must all be
// idempotent because queue retries can follow ambiguous process failures.
type RuntimeProvider interface {
	Start(context.Context, Revision) error
	CheckHealth(context.Context, Revision) error
	SwitchCaddyTraffic(context.Context, TrafficSwitch) error
	Retire(context.Context, Revision) error
}

// ValidateProviderRevision is a small shared guard for concrete runtime
// adapters. It keeps unsafe IDs or mutable tags out of provider calls.
func ValidateProviderRevision(revision Revision) error {
	if err := revision.Validate(); err != nil {
		return fmt.Errorf("invalid runtime deployment: %w", err)
	}
	return nil
}

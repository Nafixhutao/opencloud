// Package registry defines the tenant-safe, provider-neutral boundary for
// private OCI artifact storage. It does not open a registry connection itself;
// concrete transports are injected only into the isolated builder/deployer.
package registry

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var (
	// ErrNotFound means a referenced immutable artifact is absent from the
	// configured private registry.
	ErrNotFound = errors.New("registry artifact not found")
	// ErrDigestMismatch prevents an expected immutable revision from being
	// silently replaced by different registry content.
	ErrDigestMismatch = errors.New("registry artifact digest mismatch")
)

var (
	digestPattern       = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	registryHostPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*(?::[0-9]{1,5})?$`)
)

// Repository is the deterministic tenant-scoped location for one service's
// images. The OCI repository has no tag component: deployment records always
// refer to a digest, never to latest or another mutable tag.
type Repository struct {
	Host      string
	AccountID uuid.UUID
	ProjectID uuid.UUID
	ServiceID uuid.UUID
}

// NewRepository validates the private registry host and resource ownership
// tuple before a provider sees it.
func NewRepository(host string, accountID, projectID, serviceID uuid.UUID) (Repository, error) {
	repository := Repository{
		Host:      strings.ToLower(strings.TrimSpace(host)),
		AccountID: accountID,
		ProjectID: projectID,
		ServiceID: serviceID,
	}
	if err := repository.Validate(); err != nil {
		return Repository{}, err
	}
	return repository, nil
}

// ParseRepository accepts only the canonical OpenCloud private repository
// format emitted by Repository.Name. It rejects tags and arbitrary repository
// paths so stored deployments cannot escape their tenant/service namespace.
func ParseRepository(value string) (Repository, error) {
	parts := strings.Split(strings.TrimSpace(value), "/")
	if len(parts) != 5 || parts[1] != "opencloud" {
		return Repository{}, fmt.Errorf("invalid OpenCloud OCI repository %q", value)
	}
	accountID, err := uuid.Parse(parts[2])
	if err != nil {
		return Repository{}, fmt.Errorf("invalid OpenCloud OCI account id: %w", err)
	}
	projectID, err := uuid.Parse(parts[3])
	if err != nil {
		return Repository{}, fmt.Errorf("invalid OpenCloud OCI project id: %w", err)
	}
	serviceID, err := uuid.Parse(parts[4])
	if err != nil {
		return Repository{}, fmt.Errorf("invalid OpenCloud OCI service id: %w", err)
	}
	repository, err := NewRepository(parts[0], accountID, projectID, serviceID)
	if err != nil {
		return Repository{}, err
	}
	if repository.Name() != value {
		return Repository{}, fmt.Errorf("OpenCloud OCI repository is not canonical %q", value)
	}
	return repository, nil
}

// Validate checks both the private registry host and the tenant resource IDs.
func (r Repository) Validate() error {
	if !registryHostPattern.MatchString(r.Host) || strings.Contains(r.Host, "..") {
		return fmt.Errorf("invalid private registry host %q", r.Host)
	}
	if r.AccountID == uuid.Nil || r.ProjectID == uuid.Nil || r.ServiceID == uuid.Nil {
		return errors.New("registry account, project, and service ids are required")
	}
	return nil
}

// Name returns the canonical repository path. The account/project/service
// identifiers are opaque UUIDs so one tenant cannot choose another's path.
func (r Repository) Name() string {
	return r.Host + "/opencloud/" + r.AccountID.String() + "/" + r.ProjectID.String() + "/" + r.ServiceID.String()
}

// Artifact is a resolved immutable OCI image. Reference is a bare repository
// and Digest is always a sha256 identity; CanonicalReference combines them.
type Artifact struct {
	Repository Repository
	Digest     string
	SizeBytes  int64
}

// Validate confirms that an artifact can safely become a deployment identity.
func (a Artifact) Validate() error {
	if err := a.Repository.Validate(); err != nil {
		return err
	}
	if !digestPattern.MatchString(a.Digest) {
		return fmt.Errorf("invalid OCI digest %q", a.Digest)
	}
	if a.SizeBytes < 0 {
		return errors.New("OCI image size cannot be negative")
	}
	return nil
}

// CanonicalReference returns the immutable pull reference accepted by a
// runtime provider. Callers must not use Repository.Name() without this digest.
func (a Artifact) CanonicalReference() string {
	return a.Repository.Name() + "@" + a.Digest
}

// PushRequest describes an already-built image exported by the isolated
// builder. It intentionally contains a digest and bounded size only — never a
// source path, arbitrary Dockerfile, registry credential, or tag such as latest.
type PushRequest struct {
	Repository   Repository
	SourceDigest string
	SourceBytes  int64
}

// Validate checks that a push input is immutable and bounded.
func (r PushRequest) Validate() error {
	if err := r.Repository.Validate(); err != nil {
		return err
	}
	if !digestPattern.MatchString(r.SourceDigest) {
		return fmt.Errorf("invalid source OCI digest %q", r.SourceDigest)
	}
	if r.SourceBytes < 0 {
		return errors.New("source OCI image size cannot be negative")
	}
	return nil
}

// Provider can publish and verify only immutable OCI artifacts. A provider
// implementation must authenticate privately and must not be reachable from
// the public API or dashboard.
type Provider interface {
	Push(context.Context, PushRequest) (Artifact, error)
	Delete(context.Context, Artifact) error
	Exists(context.Context, Artifact) (bool, error)
	ResolveDigest(context.Context, Repository, string) (Artifact, error)
}

// ValidateDigest is available to callers that receive an OCI digest from a
// trusted lower-level builder or registry transport.
func ValidateDigest(value string) error {
	if !digestPattern.MatchString(value) {
		return fmt.Errorf("invalid OCI digest %q", value)
	}
	return nil
}

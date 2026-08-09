package registry

import (
	"context"
	"errors"
	"fmt"
)

// DistributionClient is the narrow contract an OCI Distribution-compatible
// client must implement. A concrete CNCF Distribution client can be added in
// the isolated deployment environment after its dependency and credentials are
// separately reviewed; the control-plane binary never shells out to a registry.
type DistributionClient interface {
	Push(context.Context, string, string, int64) (digest string, sizeBytes int64, err error)
	Delete(context.Context, string, string) error
	Exists(context.Context, string, string) (bool, error)
	ResolveDigest(context.Context, string, string) (digest string, sizeBytes int64, err error)
}

// DistributionProvider adapts an OCI Distribution-compatible client without
// coupling services to a particular registry product. Harbor and other OCI
// registries can use the same contract.
type DistributionProvider struct {
	client DistributionClient
}

// NewDistributionProvider returns a fail-closed provider.
func NewDistributionProvider(client DistributionClient) (*DistributionProvider, error) {
	if client == nil {
		return nil, errors.New("OCI Distribution client is required")
	}
	return &DistributionProvider{client: client}, nil
}

// Push validates and publishes an immutable artifact through the injected client.
func (p *DistributionProvider) Push(ctx context.Context, request PushRequest) (Artifact, error) {
	if p == nil || p.client == nil {
		return Artifact{}, errors.New("OCI Distribution client is unavailable")
	}
	if err := request.Validate(); err != nil {
		return Artifact{}, err
	}
	digest, size, err := p.client.Push(ctx, request.Repository.Name(), request.SourceDigest, request.SourceBytes)
	if err != nil {
		return Artifact{}, fmt.Errorf("push immutable OCI artifact: %w", err)
	}
	artifact := Artifact{Repository: request.Repository, Digest: digest, SizeBytes: size}
	if err := artifact.Validate(); err != nil {
		return Artifact{}, fmt.Errorf("registry returned invalid artifact: %w", err)
	}
	return artifact, nil
}

// Delete removes a validated immutable artifact through the injected client.
func (p *DistributionProvider) Delete(ctx context.Context, artifact Artifact) error {
	if p == nil || p.client == nil {
		return errors.New("OCI Distribution client is unavailable")
	}
	if err := artifact.Validate(); err != nil {
		return err
	}
	if err := p.client.Delete(ctx, artifact.Repository.Name(), artifact.Digest); err != nil {
		return fmt.Errorf("delete immutable OCI artifact: %w", err)
	}
	return nil
}

// Exists reports whether a validated immutable artifact is present.
func (p *DistributionProvider) Exists(ctx context.Context, artifact Artifact) (bool, error) {
	if p == nil || p.client == nil {
		return false, errors.New("OCI Distribution client is unavailable")
	}
	if err := artifact.Validate(); err != nil {
		return false, err
	}
	exists, err := p.client.Exists(ctx, artifact.Repository.Name(), artifact.Digest)
	if err != nil {
		return false, fmt.Errorf("check immutable OCI artifact: %w", err)
	}
	return exists, nil
}

// ResolveDigest verifies that the registry resolves exactly the requested digest.
func (p *DistributionProvider) ResolveDigest(ctx context.Context, repository Repository, digest string) (Artifact, error) {
	if p == nil || p.client == nil {
		return Artifact{}, errors.New("OCI Distribution client is unavailable")
	}
	if err := repository.Validate(); err != nil {
		return Artifact{}, err
	}
	if err := ValidateDigest(digest); err != nil {
		return Artifact{}, err
	}
	resolved, size, err := p.client.ResolveDigest(ctx, repository.Name(), digest)
	if err != nil {
		return Artifact{}, fmt.Errorf("resolve immutable OCI digest: %w", err)
	}
	if resolved != digest {
		return Artifact{}, ErrDigestMismatch
	}
	artifact := Artifact{Repository: repository, Digest: resolved, SizeBytes: size}
	if err := artifact.Validate(); err != nil {
		return Artifact{}, fmt.Errorf("registry returned invalid artifact: %w", err)
	}
	return artifact, nil
}

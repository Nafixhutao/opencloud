package registry

import (
	"context"
	"sync"
)

// FakeProvider is a deterministic in-memory registry for tests. It stores no
// image bytes and cannot be used to pull or execute customer code.
type FakeProvider struct {
	mu        sync.Mutex
	artifacts map[string]Artifact
}

// NewFakeProvider constructs an empty in-memory fake registry.
func NewFakeProvider() *FakeProvider {
	return &FakeProvider{artifacts: make(map[string]Artifact)}
}

// Push stores a validated immutable artifact in memory.
func (p *FakeProvider) Push(_ context.Context, request PushRequest) (Artifact, error) {
	if err := request.Validate(); err != nil {
		return Artifact{}, err
	}
	artifact := Artifact{Repository: request.Repository, Digest: request.SourceDigest, SizeBytes: request.SourceBytes}
	p.mu.Lock()
	p.artifacts[artifact.CanonicalReference()] = artifact
	p.mu.Unlock()
	return artifact, nil
}

// Delete removes a validated immutable artifact from memory.
func (p *FakeProvider) Delete(_ context.Context, artifact Artifact) error {
	if err := artifact.Validate(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.artifacts[artifact.CanonicalReference()]; !exists {
		return ErrNotFound
	}
	delete(p.artifacts, artifact.CanonicalReference())
	return nil
}

// Exists reports whether a validated immutable artifact is stored in memory.
func (p *FakeProvider) Exists(_ context.Context, artifact Artifact) (bool, error) {
	if err := artifact.Validate(); err != nil {
		return false, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_, exists := p.artifacts[artifact.CanonicalReference()]
	return exists, nil
}

// ResolveDigest returns the exact immutable artifact stored for a repository and digest.
func (p *FakeProvider) ResolveDigest(_ context.Context, repository Repository, digest string) (Artifact, error) {
	if err := repository.Validate(); err != nil {
		return Artifact{}, err
	}
	if err := ValidateDigest(digest); err != nil {
		return Artifact{}, err
	}
	key := Artifact{Repository: repository, Digest: digest}.CanonicalReference()
	p.mu.Lock()
	defer p.mu.Unlock()
	artifact, exists := p.artifacts[key]
	if !exists {
		return Artifact{}, ErrNotFound
	}
	return artifact, nil
}

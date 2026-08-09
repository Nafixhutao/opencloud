package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

const testDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func testRepository(t *testing.T) Repository {
	t.Helper()
	repository, err := NewRepository("registry.internal:5000", uuid.New(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	return repository
}

func TestRepositoryUsesTenantScopedDigestOnlyIdentity(t *testing.T) {
	repository := testRepository(t)
	if want := "registry.internal:5000/opencloud/"; len(repository.Name()) < len(want) || repository.Name()[:len(want)] != want {
		t.Fatalf("repository name = %q, want private OpenCloud namespace", repository.Name())
	}
	artifact := Artifact{Repository: repository, Digest: testDigest, SizeBytes: 10}
	if err := artifact.Validate(); err != nil {
		t.Fatalf("artifact.Validate: %v", err)
	}
	if artifact.CanonicalReference() != repository.Name()+"@"+testDigest {
		t.Fatalf("canonical reference = %q", artifact.CanonicalReference())
	}
	parsed, err := ParseRepository(repository.Name())
	if err != nil || parsed != repository {
		t.Fatalf("ParseRepository = %#v, %v", parsed, err)
	}
}

func TestRegistryRejectsMutableOrMalformedDigest(t *testing.T) {
	if err := ValidateDigest("latest"); err == nil {
		t.Fatal("latest was accepted as an immutable digest")
	}
	if err := ValidateDigest("sha256:ABC"); err == nil {
		t.Fatal("malformed digest was accepted")
	}
}

func TestFakeProviderPushResolveDelete(t *testing.T) {
	provider := NewFakeProvider()
	repository := testRepository(t)
	artifact, err := provider.Push(context.Background(), PushRequest{Repository: repository, SourceDigest: testDigest, SourceBytes: 123})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	exists, err := provider.Exists(context.Background(), artifact)
	if err != nil || !exists {
		t.Fatalf("Exists = %v, %v", exists, err)
	}
	resolved, err := provider.ResolveDigest(context.Background(), repository, testDigest)
	if err != nil || resolved != artifact {
		t.Fatalf("ResolveDigest = %#v, %v", resolved, err)
	}
	if err := provider.Delete(context.Background(), artifact); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = provider.ResolveDigest(context.Background(), repository, testDigest)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("ResolveDigest after delete = %v, want not found", err)
	}
}

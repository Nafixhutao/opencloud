package provisioner

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestFakeLifecycleIsIdempotentAndOwnershipGuarded(t *testing.T) {
	fake := NewFake()
	spec := SiteSpec{
		SiteID:    uuid.New(),
		AccountID: uuid.New(),
		NodeID:    uuid.New(),
	}
	ref := SiteRef{SiteID: spec.SiteID, AccountID: spec.AccountID, NodeID: spec.NodeID}

	if err := fake.CreateSite(context.Background(), spec); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if err := fake.CreateSite(context.Background(), spec); err != nil {
		t.Fatalf("idempotent create: %v", err)
	}
	if state, err := fake.SiteStatus(context.Background(), ref); err != nil || state != SiteStateRunning {
		t.Fatalf("status = %q, %v", state, err)
	}
	if err := fake.SuspendSite(context.Background(), ref); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if err := fake.SuspendSite(context.Background(), ref); err != nil {
		t.Fatalf("idempotent suspend: %v", err)
	}
	if err := fake.ResumeSite(context.Background(), ref); err != nil {
		t.Fatalf("resume: %v", err)
	}

	wrong := ref
	wrong.AccountID = uuid.New()
	if err := fake.DeleteSite(context.Background(), wrong); err == nil {
		t.Fatal("delete accepted mismatched ownership")
	}
	if err := fake.DeleteSite(context.Background(), ref); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := fake.DeleteSite(context.Background(), ref); err != nil {
		t.Fatalf("idempotent delete: %v", err)
	}
}

func TestFakeFailureIsInjectedOnce(t *testing.T) {
	fake := NewFake()
	spec := SiteSpec{SiteID: uuid.New(), AccountID: uuid.New(), NodeID: uuid.New()}
	fake.FailNext(errors.New("ambiguous backend response"))

	if err := fake.CreateSite(context.Background(), spec); err == nil {
		t.Fatal("first operation unexpectedly succeeded")
	}
	if err := fake.CreateSite(context.Background(), spec); err != nil {
		t.Fatalf("retry did not converge: %v", err)
	}
}

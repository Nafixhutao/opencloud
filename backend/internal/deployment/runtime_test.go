package deployment

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/nazxf/opencloud/backend/internal/registry"
)

const testDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func testRevision(t *testing.T) Revision {
	t.Helper()
	repository, err := registry.NewRepository("registry.internal", uuid.New(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	return Revision{
		AccountID: repository.AccountID, ProjectID: repository.ProjectID, ServiceID: repository.ServiceID,
		DeploymentID: uuid.New(), Artifact: registry.Artifact{Repository: repository, Digest: testDigest, SizeBytes: 1},
	}
}

func TestFakeRuntimeRecordsCaddySwitchSequence(t *testing.T) {
	runtime := &FakeRuntime{}
	previous := testRevision(t)
	target := testRevision(t)
	target.AccountID, target.ProjectID, target.ServiceID = previous.AccountID, previous.ProjectID, previous.ServiceID
	target.Artifact.Repository.AccountID = previous.AccountID
	target.Artifact.Repository.ProjectID = previous.ProjectID
	target.Artifact.Repository.ServiceID = previous.ServiceID

	if err := runtime.Start(context.Background(), target); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := runtime.CheckHealth(context.Background(), target); err != nil {
		t.Fatalf("CheckHealth: %v", err)
	}
	if err := runtime.SwitchCaddyTraffic(context.Background(), TrafficSwitch{Target: target, Previous: &previous}); err != nil {
		t.Fatalf("SwitchCaddyTraffic: %v", err)
	}
	if err := runtime.Retire(context.Background(), previous); err != nil {
		t.Fatalf("Retire: %v", err)
	}
	actions := runtime.Actions()
	want := []string{ActionStart, ActionCheckHealth, ActionSwitchTraffic, ActionRetire}
	if len(actions) != len(want) {
		t.Fatalf("actions = %#v", actions)
	}
	for index := range want {
		if actions[index].Name != want[index] {
			t.Fatalf("actions[%d] = %q, want %q", index, actions[index].Name, want[index])
		}
	}
}

func TestTrafficSwitchRejectsCrossServiceRevision(t *testing.T) {
	target, previous := testRevision(t), testRevision(t)
	if err := (TrafficSwitch{Target: target, Previous: &previous}).Validate(); err == nil {
		t.Fatal("cross-service switch was accepted")
	}
}

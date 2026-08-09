package builder

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nazxf/opencloud/backend/internal/build"
)

func testRequest() Request {
	return Request{
		ID: "build-1",
		Plan: build.Plan{
			Provider:   "railpack",
			ArtifactID: "artifact-1",
			Kind:       build.KindApplication,
		},
		Source: SourceArtifact{ID: "artifact-1", SizeBytes: 10},
	}
}

func TestLimitsRequireEveryGuard(t *testing.T) {
	limits := DefaultLimits()
	if err := limits.Validate(); err != nil {
		t.Fatalf("default limits: %v", err)
	}
	limits.PIDLimit = 0
	if err := limits.Validate(); err == nil {
		t.Fatal("limits without a PID limit unexpectedly succeeded")
	}
}

func TestMachineRejectsSkippedAndTerminalTransitions(t *testing.T) {
	machine := NewMachine()
	if err := machine.Transition(StateBuilding); err == nil {
		t.Fatal("queued build skipped preparing")
	}
	for _, state := range []State{StatePreparing, StateBuilding, StateExporting, StateSucceeded} {
		if err := machine.Transition(state); err != nil {
			t.Fatalf("transition to %q: %v", state, err)
		}
	}
	if err := machine.Transition(StateFailed); err == nil {
		t.Fatal("terminal build transitioned again")
	}
}

func TestServiceStreamsLifecycleAndCleansUp(t *testing.T) {
	executor := &FakeExecutor{}
	service, err := NewService(executor, DefaultLimits())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	stream := NewStream()
	events, unsubscribe := stream.Subscribe(16)
	defer unsubscribe()

	result, err := service.Build(context.Background(), testRequest(), stream)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if result.State != StateSucceeded || !result.Simulated || result.ArtifactDigest == "" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if executor.CleanupCalls() != 1 {
		t.Fatalf("cleanup calls = %d, want 1", executor.CleanupCalls())
	}

	var states []State
	for event := range events {
		states = append(states, event.State)
	}
	want := []State{StateQueued, StatePreparing, StateBuilding, StateExporting, StateSucceeded}
	if len(states) != len(want) {
		t.Fatalf("states = %v, want %v", states, want)
	}
	for index := range want {
		if states[index] != want[index] {
			t.Fatalf("states[%d] = %q, want %q", index, states[index], want[index])
		}
	}
}

func TestServiceCancellationCleansUp(t *testing.T) {
	started := make(chan struct{}, 1)
	block := make(chan struct{})
	executor := &FakeExecutor{Started: started, Block: block}
	service, err := NewService(executor, DefaultLimits())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := NewStream()
	resultCh := make(chan struct {
		result Result
		err    error
	}, 1)
	go func() {
		result, buildErr := service.Build(ctx, testRequest(), stream)
		resultCh <- struct {
			result Result
			err    error
		}{result, buildErr}
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("build did not start")
	}
	cancel()

	select {
	case outcome := <-resultCh:
		if !errors.Is(outcome.err, context.Canceled) {
			t.Fatalf("Build error = %v, want context cancellation", outcome.err)
		}
		if outcome.result.State != StateCancelled {
			t.Fatalf("result state = %q, want cancelled", outcome.result.State)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled build did not return")
	}
	if executor.CleanupCalls() != 1 {
		t.Fatalf("cleanup calls = %d, want 1", executor.CleanupCalls())
	}
}

func TestServiceRejectsInvalidArtifactAfterCleanup(t *testing.T) {
	executor := &FakeExecutor{}
	service, err := NewService(executor, DefaultLimits())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	// An executor is a narrow interface and may be independently implemented;
	// ensure success still requires a valid immutable artifact identity.
	invalid := executorFunc{execute: func(context.Context, ExecutionRequest) (ExecutionResult, error) {
		return ExecutionResult{ImageBytes: 1}, nil
	}, cleanup: executor.Cleanup}
	service, err = NewService(invalid, DefaultLimits())
	if err != nil {
		t.Fatalf("NewService invalid executor: %v", err)
	}
	result, err := service.Build(context.Background(), testRequest(), NewStream())
	if !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("Build error = %v, want invalid artifact", err)
	}
	if result.State != StateFailed {
		t.Fatalf("result state = %q, want failed", result.State)
	}
	if executor.CleanupCalls() != 1 {
		t.Fatalf("cleanup calls = %d, want 1", executor.CleanupCalls())
	}
}

type executorFunc struct {
	execute func(context.Context, ExecutionRequest) (ExecutionResult, error)
	cleanup func(context.Context, ExecutionRequest) error
}

func (f executorFunc) Execute(ctx context.Context, request ExecutionRequest) (ExecutionResult, error) {
	return f.execute(ctx, request)
}

func (f executorFunc) Cleanup(ctx context.Context, request ExecutionRequest) error {
	return f.cleanup(ctx, request)
}

type recordingBuildKitClient struct {
	request    BuildKitSolveRequest
	cleanupID  string
	solveCalls int
}

func (c *recordingBuildKitClient) Solve(_ context.Context, request BuildKitSolveRequest) (BuildKitSolveResult, error) {
	c.request = request
	c.solveCalls++
	return BuildKitSolveResult{ArtifactDigest: "sha256:test", ImageBytes: 12}, nil
}

func (c *recordingBuildKitClient) Cleanup(_ context.Context, buildID string) error {
	c.cleanupID = buildID
	return nil
}

func TestBuildKitExecutorForcesSandboxContract(t *testing.T) {
	client := &recordingBuildKitClient{}
	executor, err := NewBuildKitExecutor(client)
	if err != nil {
		t.Fatalf("NewBuildKitExecutor: %v", err)
	}
	request := testRequest()
	result, err := executor.Execute(context.Background(), ExecutionRequest{
		BuildID:  request.ID,
		Plan:     request.Plan,
		Source:   request.Source,
		Limits:   DefaultLimits(),
		Rootless: true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.ImageBytes != 12 || client.solveCalls != 1 {
		t.Fatalf("unexpected BuildKit result=%#v calls=%d", result, client.solveCalls)
	}
	if !client.request.RootlessRequired || client.request.Network != NetworkDisabled || client.request.HostMounts {
		t.Fatalf("unsafe BuildKit request: %#v", client.request)
	}
	if err := executor.Cleanup(context.Background(), ExecutionRequest{BuildID: request.ID}); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if client.cleanupID != request.ID {
		t.Fatalf("cleanup id = %q, want %q", client.cleanupID, request.ID)
	}
}

func TestBuildKitExecutorRejectsMissingClient(t *testing.T) {
	if _, err := NewBuildKitExecutor(nil); !errors.Is(err, ErrBuildKitUnavailable) {
		t.Fatalf("NewBuildKitExecutor(nil) error = %v", err)
	}
}

func TestBuildKitExecutorValidatesLimitsBeforeCallingClient(t *testing.T) {
	client := &recordingBuildKitClient{}
	executor, err := NewBuildKitExecutor(client)
	if err != nil {
		t.Fatalf("NewBuildKitExecutor: %v", err)
	}
	request := testRequest()
	limits := DefaultLimits()
	limits.MemoryBytes = 0
	_, err = executor.Execute(context.Background(), ExecutionRequest{
		BuildID:  request.ID,
		Plan:     request.Plan,
		Source:   request.Source,
		Limits:   limits,
		Rootless: true,
	})
	if err == nil {
		t.Fatal("BuildKit executor accepted missing memory limit")
	}
	if client.solveCalls != 0 {
		t.Fatalf("BuildKit solve calls = %d, want 0", client.solveCalls)
	}
}

package builder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// FakeExecutor is a deterministic test adapter. It never reads source content,
// starts a process, opens a network connection, or creates an image.
type FakeExecutor struct {
	// Started is notified when Execute begins. It is useful for cancellation
	// tests; callers may leave it nil.
	Started chan<- struct{}
	// Block, when non-nil, keeps Execute running until it is closed or the
	// context is cancelled.
	Block <-chan struct{}
	// ExecuteErr and CleanupErr simulate isolated-builder failures.
	ExecuteErr error
	CleanupErr error

	mu           sync.Mutex
	cleanupCalls int
}

func (f *FakeExecutor) Execute(ctx context.Context, request ExecutionRequest) (ExecutionResult, error) {
	if f.Started != nil {
		select {
		case f.Started <- struct{}{}:
		case <-ctx.Done():
			return ExecutionResult{}, ctx.Err()
		}
	}
	if f.Block != nil {
		select {
		case <-f.Block:
		case <-ctx.Done():
			return ExecutionResult{}, ctx.Err()
		}
	}
	if f.ExecuteErr != nil {
		return ExecutionResult{}, f.ExecuteErr
	}
	digest := sha256.Sum256([]byte(request.BuildID + ":" + request.Source.ID + ":" + request.Plan.Provider))
	return ExecutionResult{
		ArtifactDigest: "sha256:" + hex.EncodeToString(digest[:]),
		ImageBytes:     1,
		Simulated:      true,
	}, nil
}

func (f *FakeExecutor) Cleanup(context.Context, ExecutionRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleanupCalls++
	return f.CleanupErr
}

// CleanupCalls reports how often cleanup was attempted.
func (f *FakeExecutor) CleanupCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cleanupCalls
}

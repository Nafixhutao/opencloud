// Package builder owns the isolated execution boundary for application builds.
// It accepts only an immutable source-artifact reference and declarative plan;
// it never receives a local source path, customer credentials, or a Docker
// socket from the control plane.
package builder

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nazxf/opencloud/backend/internal/build"
)

var (
	// ErrBuildKitUnavailable means no isolated BuildKit adapter was supplied.
	// It is intentionally distinct from an execution failure so callers never
	// retry it as if customer source had run.
	ErrBuildKitUnavailable = errors.New("isolated BuildKit executor is unavailable")
	// ErrImageTooLarge rejects an artifact before it can be published by a later
	// registry integration.
	ErrImageTooLarge = errors.New("built image exceeds the configured size limit")
	// ErrInvalidArtifact prevents a failed executor from being recorded as a
	// successful build with an absent or impossible artifact identity.
	ErrInvalidArtifact = errors.New("builder returned an invalid artifact result")
)

const (
	defaultCPUMilli       int64 = 2_000
	defaultMemoryBytes    int64 = 4 << 30
	defaultPIDLimit       int64 = 256
	defaultSourceBytes    int64 = 1 << 30
	defaultImageBytes     int64 = 4 << 30
	defaultBuildTimeout         = 15 * time.Minute
	defaultCleanupTimeout       = 30 * time.Second
)

// Limits are mandatory per-build sandbox bounds. A future BuildKit transport
// must enforce every field; accepting an unset limit is unsafe.
type Limits struct {
	CPUMilli       int64
	MemoryBytes    int64
	PIDLimit       int64
	MaxSourceBytes int64
	MaxImageBytes  int64
	Timeout        time.Duration
	CleanupTimeout time.Duration
}

// DefaultLimits returns the conservative limits used until per-plan quotas are
// introduced. They are values, not optional hints.
func DefaultLimits() Limits {
	return Limits{
		CPUMilli:       defaultCPUMilli,
		MemoryBytes:    defaultMemoryBytes,
		PIDLimit:       defaultPIDLimit,
		MaxSourceBytes: defaultSourceBytes,
		MaxImageBytes:  defaultImageBytes,
		Timeout:        defaultBuildTimeout,
		CleanupTimeout: defaultCleanupTimeout,
	}
}

// Validate verifies that every isolation and capacity guard is active.
func (l Limits) Validate() error {
	for _, check := range []struct {
		name  string
		value int64
	}{
		{"CPU milli", l.CPUMilli},
		{"memory bytes", l.MemoryBytes},
		{"PID limit", l.PIDLimit},
		{"maximum source bytes", l.MaxSourceBytes},
		{"maximum image bytes", l.MaxImageBytes},
	} {
		if check.value <= 0 {
			return fmt.Errorf("builder %s must be positive", check.name)
		}
	}
	if l.Timeout <= 0 {
		return errors.New("builder timeout must be positive")
	}
	if l.CleanupTimeout <= 0 {
		return errors.New("builder cleanup timeout must be positive")
	}
	return nil
}

// SourceArtifact is a trusted, immutable reference produced by the future
// source-acquisition layer. It deliberately has no filesystem path, archive
// URL, source content, or credential-bearing repository URL.
type SourceArtifact struct {
	ID        string
	SizeBytes int64
}

// Request is the complete input admitted to an isolated build. It is small
// enough for a queue message and contains no secrets or customer source.
type Request struct {
	ID     string
	Plan   build.Plan
	Source SourceArtifact
}

func (r Request) validate(limits Limits) error {
	if strings.TrimSpace(r.ID) == "" {
		return errors.New("build id is required")
	}
	if strings.TrimSpace(r.Plan.Provider) == "" || strings.TrimSpace(r.Plan.ArtifactID) == "" {
		return errors.New("build plan provider and artifact id are required")
	}
	if r.Plan.Kind != build.KindStatic && r.Plan.Kind != build.KindApplication {
		return fmt.Errorf("build plan kind %q is invalid", r.Plan.Kind)
	}
	if r.Source.ID != r.Plan.ArtifactID {
		return errors.New("source artifact does not match build plan")
	}
	if r.Source.SizeBytes <= 0 {
		return errors.New("source artifact size must be positive")
	}
	if r.Source.SizeBytes > limits.MaxSourceBytes {
		return errors.New("source artifact exceeds the configured size limit")
	}
	return nil
}

// State is a build lifecycle state persisted by a later durable job adapter.
// The service enforces transitions now so that adapter cannot invent a second
// state machine when source acquisition and registry publication land.
type State string

const (
	// StateQueued is the initial state before an isolated builder begins preparation.
	StateQueued State = "queued"
	// StatePreparing validates the build request and isolated execution environment.
	StatePreparing State = "preparing"
	// StateBuilding represents an active isolated build.
	StateBuilding State = "building"
	// StateExporting represents exporting the completed immutable artifact.
	StateExporting State = "exporting"
	// StateSucceeded represents a completed build with a valid immutable artifact.
	StateSucceeded State = "succeeded"
	// StateFailed represents a terminal failed build.
	StateFailed State = "failed"
	// StateCancelled represents a terminal cancelled build.
	StateCancelled State = "cancelled"
)

// IsTerminal reports whether a build may not transition further.
func (s State) IsTerminal() bool {
	return s == StateSucceeded || s == StateFailed || s == StateCancelled
}

// CanTransition reports whether the target is an allowed lifecycle step.
func (s State) CanTransition(target State) bool {
	if s.IsTerminal() {
		return false
	}
	if target == StateFailed || target == StateCancelled {
		return true
	}
	switch s {
	case StateQueued:
		return target == StatePreparing
	case StatePreparing:
		return target == StateBuilding
	case StateBuilding:
		return target == StateExporting
	case StateExporting:
		return target == StateSucceeded
	default:
		return false
	}
}

// Machine validates lifecycle changes for one build.
type Machine struct {
	state State
}

// NewMachine starts every admitted request in the queued state.
func NewMachine() *Machine { return &Machine{state: StateQueued} }

// State returns the machine's current lifecycle state.
func (m *Machine) State() State { return m.state }

// Transition moves to the next valid state.
func (m *Machine) Transition(target State) error {
	if !m.state.CanTransition(target) {
		return fmt.Errorf("invalid build transition %q -> %q", m.state, target)
	}
	m.state = target
	return nil
}

// ExecutionRequest is what an executor receives. Sandbox restrictions are
// represented as data and checked by the BuildKit adapter, not delegated to
// application plans or API handlers.
type ExecutionRequest struct {
	BuildID  string
	Plan     build.Plan
	Source   SourceArtifact
	Limits   Limits
	Rootless bool
}

// ExecutionResult is a private pre-registry build result. The digest is only
// eligible for publication after the registry slice verifies and stores it.
type ExecutionResult struct {
	ArtifactDigest string
	ImageBytes     int64
	Simulated      bool
}

// Executor is the dedicated, capability-restricted execution boundary. It has
// no way to request database access, host mounts, repository credentials, or
// arbitrary command text.
type Executor interface {
	Execute(context.Context, ExecutionRequest) (ExecutionResult, error)
	Cleanup(context.Context, ExecutionRequest) error
}

// Result is returned only after the executor has cleaned its ephemeral work
// area. It intentionally lacks a registry URL because registry publication is
// a separate later capability.
type Result struct {
	State          State
	ArtifactDigest string
	ImageBytes     int64
	Simulated      bool
}

// Service owns one build execution lifecycle. A queue adapter will call Build;
// HTTP handlers and the normal runtime worker must not call it directly.
type Service struct {
	executor Executor
	limits   Limits
	now      func() time.Time
}

// NewService creates the isolated-builder service with mandatory bounds.
func NewService(executor Executor, limits Limits) (*Service, error) {
	if executor == nil {
		return nil, errors.New("builder executor is required")
	}
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	return &Service{executor: executor, limits: limits, now: time.Now}, nil
}

// Build executes an admitted request, streams lifecycle records, and always
// attempts cleanup before returning a terminal result. Only fixed operational
// messages are streamed here: raw customer build output needs a dedicated
// redacting, durable log store in a later slice.
func (s *Service) Build(ctx context.Context, request Request, stream *Stream) (result Result, err error) {
	if stream == nil {
		return Result{}, errors.New("build event stream is required")
	}
	if err := request.validate(s.limits); err != nil {
		return Result{}, err
	}

	machine := NewMachine()
	emit := func(message string) {
		stream.Publish(Event{At: s.now().UTC(), State: machine.State(), Message: message})
	}
	defer stream.Close()

	emit("Build queued")
	if err := machine.Transition(StatePreparing); err != nil {
		return Result{}, err
	}
	emit("Preparing isolated build environment")
	if err := machine.Transition(StateBuilding); err != nil {
		return Result{}, err
	}
	emit("Building source artifact")

	executionCtx, cancel := context.WithTimeout(ctx, s.limits.Timeout)
	defer cancel()
	execution := ExecutionRequest{
		BuildID:  request.ID,
		Plan:     request.Plan,
		Source:   request.Source,
		Limits:   s.limits,
		Rootless: true,
	}

	executionResult, executionErr := s.executor.Execute(executionCtx, execution)
	if executionErr == nil && executionCtx.Err() != nil {
		executionErr = executionCtx.Err()
	}
	if executionErr == nil {
		switch {
		case strings.TrimSpace(executionResult.ArtifactDigest) == "":
			executionErr = ErrInvalidArtifact
		case executionResult.ImageBytes < 0:
			executionErr = ErrInvalidArtifact
		case executionResult.ImageBytes > s.limits.MaxImageBytes:
			executionErr = ErrImageTooLarge
		}
	}

	if executionErr == nil {
		if err := machine.Transition(StateExporting); err != nil {
			return Result{}, err
		}
		emit("Finalizing build artifact")
	}

	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), s.limits.CleanupTimeout)
	cleanupErr := s.executor.Cleanup(cleanupCtx, execution)
	cleanupCancel()
	if cleanupErr != nil && executionErr == nil {
		executionErr = fmt.Errorf("clean up isolated build: %w", cleanupErr)
	}

	if executionErr != nil {
		terminal := StateFailed
		message := "Build failed"
		if errors.Is(executionErr, context.Canceled) || errors.Is(executionErr, context.DeadlineExceeded) {
			terminal = StateCancelled
			message = "Build cancelled"
		}
		if transitionErr := machine.Transition(terminal); transitionErr != nil {
			return Result{}, transitionErr
		}
		emit(message)
		return Result{State: terminal}, executionErr
	}

	if err := machine.Transition(StateSucceeded); err != nil {
		return Result{}, err
	}
	emit("Build completed")
	return Result{
		State:          StateSucceeded,
		ArtifactDigest: executionResult.ArtifactDigest,
		ImageBytes:     executionResult.ImageBytes,
		Simulated:      executionResult.Simulated,
	}, nil
}

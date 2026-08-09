package builder

import (
	"context"
	"errors"
	"fmt"
)

// NetworkPolicy is deliberately closed by default. Fetching a private source
// later will use a narrowly scoped source-acquisition capability, not general
// network access from arbitrary customer build steps.
type NetworkPolicy string

const NetworkDisabled NetworkPolicy = "disabled"

// BuildKitSolveRequest is the only request shape an eventual BuildKit client
// may receive. It does not contain a Dockerfile path, arbitrary build command,
// bind mount, registry credential, or Docker daemon socket.
type BuildKitSolveRequest struct {
	BuildID          string
	SourceArtifactID string
	Provider         string
	Kind             string
	Limits           Limits
	RootlessRequired bool
	Network          NetworkPolicy
	HostMounts       bool
}

// BuildKitSolveResult comes from an adapter that runs inside the isolated
// builder environment. Registry publication is intentionally out of scope.
type BuildKitSolveResult struct {
	ArtifactDigest string
	ImageBytes     int64
}

// BuildKitClient abstracts the versioned BuildKit client library or private
// transport. Keeping it behind this interface avoids putting that dependency,
// a socket, or a process launcher in the control-plane module.
type BuildKitClient interface {
	Solve(context.Context, BuildKitSolveRequest) (BuildKitSolveResult, error)
	Cleanup(context.Context, string) error
}

// BuildKitExecutor adapts the isolated BuildKit transport to the builder
// service. Constructing it without a transport fails closed.
type BuildKitExecutor struct {
	client BuildKitClient
}

// NewBuildKitExecutor returns a capability-restricted BuildKit executor.
func NewBuildKitExecutor(client BuildKitClient) (*BuildKitExecutor, error) {
	if client == nil {
		return nil, ErrBuildKitUnavailable
	}
	return &BuildKitExecutor{client: client}, nil
}

// Execute requests a rootless, network-disabled solve with no host mounts.
func (e *BuildKitExecutor) Execute(ctx context.Context, request ExecutionRequest) (ExecutionResult, error) {
	if e == nil || e.client == nil {
		return ExecutionResult{}, ErrBuildKitUnavailable
	}
	if !request.Rootless {
		return ExecutionResult{}, errors.New("builder requires rootless execution")
	}
	if err := request.Limits.Validate(); err != nil {
		return ExecutionResult{}, fmt.Errorf("invalid BuildKit limits: %w", err)
	}
	if err := (Request{ID: request.BuildID, Plan: request.Plan, Source: request.Source}).validate(request.Limits); err != nil {
		return ExecutionResult{}, fmt.Errorf("invalid BuildKit request: %w", err)
	}
	result, err := e.client.Solve(ctx, BuildKitSolveRequest{
		BuildID:          request.BuildID,
		SourceArtifactID: request.Source.ID,
		Provider:         request.Plan.Provider,
		Kind:             request.Plan.Kind,
		Limits:           request.Limits,
		RootlessRequired: true,
		Network:          NetworkDisabled,
		HostMounts:       false,
	})
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("BuildKit solve: %w", err)
	}
	if result.ImageBytes < 0 {
		return ExecutionResult{}, errors.New("BuildKit returned a negative image size")
	}
	return ExecutionResult{ArtifactDigest: result.ArtifactDigest, ImageBytes: result.ImageBytes}, nil
}

// Cleanup removes the ephemeral BuildKit state for exactly this build.
func (e *BuildKitExecutor) Cleanup(ctx context.Context, request ExecutionRequest) error {
	if e == nil || e.client == nil {
		return ErrBuildKitUnavailable
	}
	return e.client.Cleanup(ctx, request.BuildID)
}

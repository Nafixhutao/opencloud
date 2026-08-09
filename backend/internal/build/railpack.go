package build

import "context"

// RailpackProvider is the default generic application planner. It deliberately
// makes no language decision: Railpack performs its own runtime detection only
// after an isolated build worker receives the source artifact.
type RailpackProvider struct{}

// Name returns the Railpack provider's stable identifier.
func (RailpackProvider) Name() string { return "railpack" }

// Detect recognizes any non-empty source manifest for later isolated detection.
func (RailpackProvider) Detect(_ context.Context, source SourceManifest) (Detection, error) {
	if len(source.Files) == 0 {
		return Detection{}, ErrNotDetected
	}
	return Detection{Provider: "railpack", Kind: KindApplication, Evidence: []string{"source-manifest"}}, nil
}

// Plan produces a declarative Railpack plan without executing customer source.
func (RailpackProvider) Plan(_ context.Context, source SourceManifest, detection Detection) (Plan, error) {
	if detection.Provider != "railpack" || detection.Kind != KindApplication {
		return Plan{}, ErrNotDetected
	}
	return Plan{Provider: "railpack", ArtifactID: source.ArtifactID, Kind: KindApplication, Evidence: detection.Evidence}, nil
}

// Build fails closed until isolated execution is enabled.
func (RailpackProvider) Build(_ context.Context, _ Plan) (Result, error) {
	return Result{}, ErrExecutionDisabled
}

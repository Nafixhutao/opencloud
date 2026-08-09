package build

import "context"

// RailpackProvider is the default generic application planner. It deliberately
// makes no language decision: Railpack performs its own runtime detection only
// after an isolated build worker receives the source artifact.
type RailpackProvider struct{}

func (RailpackProvider) Name() string { return "railpack" }

func (RailpackProvider) Detect(_ context.Context, source SourceManifest) (Detection, error) {
	if len(source.Files) == 0 {
		return Detection{}, ErrNotDetected
	}
	return Detection{Provider: "railpack", Kind: KindApplication, Evidence: []string{"source-manifest"}}, nil
}

func (RailpackProvider) Plan(_ context.Context, source SourceManifest, detection Detection) (Plan, error) {
	if detection.Provider != "railpack" || detection.Kind != KindApplication {
		return Plan{}, ErrNotDetected
	}
	return Plan{Provider: "railpack", ArtifactID: source.ArtifactID, Kind: KindApplication, Evidence: detection.Evidence}, nil
}

func (RailpackProvider) Build(_ context.Context, _ Plan) (Result, error) {
	return Result{}, ErrExecutionDisabled
}

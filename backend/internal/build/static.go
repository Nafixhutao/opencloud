package build

import "context"

// StaticProvider recognizes a root index.html and creates a lightweight static
// serving plan. Packaging waits for the isolated builder/registry slices.
type StaticProvider struct{}

func (StaticProvider) Name() string { return "static" }

func (StaticProvider) Detect(_ context.Context, source SourceManifest) (Detection, error) {
	if !source.HasRootFile("index.html") {
		return Detection{}, ErrNotDetected
	}
	return Detection{Provider: "static", Kind: KindStatic, Evidence: []string{"index.html"}}, nil
}

func (StaticProvider) Plan(_ context.Context, source SourceManifest, detection Detection) (Plan, error) {
	if detection.Provider != "static" || detection.Kind != KindStatic {
		return Plan{}, ErrNotDetected
	}
	return Plan{Provider: "static", ArtifactID: source.ArtifactID, Kind: KindStatic, Evidence: detection.Evidence}, nil
}

func (StaticProvider) Build(_ context.Context, _ Plan) (Result, error) {
	return Result{}, ErrExecutionDisabled
}

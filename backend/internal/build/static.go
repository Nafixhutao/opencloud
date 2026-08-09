package build

import "context"

// StaticProvider recognizes a root index.html and creates a lightweight static
// serving plan. Packaging waits for the isolated builder/registry slices.
type StaticProvider struct{}

// Name returns the static provider's stable identifier.
func (StaticProvider) Name() string { return "static" }

// Detect recognizes manifests containing a root index.html file.
func (StaticProvider) Detect(_ context.Context, source SourceManifest) (Detection, error) {
	if !source.HasRootFile("index.html") {
		return Detection{}, ErrNotDetected
	}
	return Detection{Provider: "static", Kind: KindStatic, Evidence: []string{"index.html"}}, nil
}

// Plan produces a declarative static-serving plan without packaging files.
func (StaticProvider) Plan(_ context.Context, source SourceManifest, detection Detection) (Plan, error) {
	if detection.Provider != "static" || detection.Kind != KindStatic {
		return Plan{}, ErrNotDetected
	}
	return Plan{Provider: "static", ArtifactID: source.ArtifactID, Kind: KindStatic, Evidence: detection.Evidence}, nil
}

// Build fails closed until the isolated builder and registry are enabled.
func (StaticProvider) Build(_ context.Context, _ Plan) (Result, error) {
	return Result{}, ErrExecutionDisabled
}

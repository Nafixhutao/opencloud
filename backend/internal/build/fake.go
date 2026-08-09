package build

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
)

// FakeProvider is a deterministic test adapter. It never executes source or
// produces a real artifact.
type FakeProvider struct {
	Kind string
}

func (f FakeProvider) Name() string { return "fake" }

func (f FakeProvider) Detect(_ context.Context, _ SourceManifest) (Detection, error) {
	kind := f.Kind
	if kind == "" {
		kind = KindApplication
	}
	return Detection{Provider: f.Name(), Kind: kind, Evidence: []string{"fake"}}, nil
}

func (f FakeProvider) Plan(_ context.Context, source SourceManifest, detection Detection) (Plan, error) {
	return Plan{Provider: f.Name(), ArtifactID: source.ArtifactID, Kind: detection.Kind, Evidence: detection.Evidence}, nil
}

func (f FakeProvider) Build(_ context.Context, plan Plan) (Result, error) {
	digest := sha256.Sum256([]byte(plan.Provider + ":" + plan.ArtifactID + ":" + plan.Kind))
	return Result{Provider: f.Name(), ArtifactDigest: "sha256:" + hex.EncodeToString(digest[:]), Simulated: true}, nil
}

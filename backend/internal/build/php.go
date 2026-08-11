package build

import (
	"context"
	"errors"
	"strings"
)

// PHPProvider detects PHP projects via composer.json.
type PHPProvider struct{}

// Name returns the provider identifier.
func (p *PHPProvider) Name() string { return "php" }

// Detect checks for composer.json presence.
func (p *PHPProvider) Detect(_ context.Context, source SourceManifest) (Detection, error) {
	if !source.HasRootFile("composer.json") {
		return Detection{}, ErrNotDetected
	}
	return Detection{
		Provider: p.Name(),
		Kind:     KindApplication,
		Evidence: []string{"composer.json"},
	}, nil
}

// Plan creates a PHP build plan.
func (p *PHPProvider) Plan(_ context.Context, source SourceManifest, detection Detection) (Plan, error) {
	if detection.Provider != p.Name() {
		return Plan{}, errors.New("PHP provider called with wrong detection")
	}

	var evidence []string
	if source.HasRootFile("composer.lock") {
		evidence = append(evidence, "composer.lock")
	}

	// Detect web root
	webRoot := ""
	for _, dir := range []string{"public", "web", "www"} {
		for _, f := range source.Files {
			if strings.HasPrefix(f.Path, dir+"/") {
				webRoot = dir
				break
			}
		}
		if webRoot != "" {
			break
		}
	}

	result := append(detection.Evidence, evidence...)
	if webRoot != "" {
		result = append(result, "web_root:"+webRoot)
	}

	return Plan{
		Provider:   p.Name(),
		ArtifactID: source.ArtifactID,
		Kind:       detection.Kind,
		Evidence:   result,
	}, nil
}

// Build is not called — real execution is gated.
func (p *PHPProvider) Build(_ context.Context, _ Plan) (Result, error) {
	return Result{}, ErrExecutionDisabled
}

var _ Provider = (*PHPProvider)(nil)

// Package build defines the safe, provider-neutral planning boundary for
// application builds. It intentionally does not execute customer source.
package build

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
)

var (
	// ErrNotDetected means a provider does not support the supplied source.
	ErrNotDetected = errors.New("build provider did not detect a compatible source")
	// ErrExecutionDisabled prevents source execution until the trusted source
	// transport and private registry are connected to the isolated builder.
	ErrExecutionDisabled = errors.New("build execution is disabled until trusted source and registry boundaries are available")
)

const (
	KindStatic      = "static"
	KindApplication = "application"
)

// SourceFile is metadata emitted by a future trusted source-acquisition step.
// It contains no content, archive path, credentials, or command.
type SourceFile struct {
	Path string
	Size int64
}

// SourceManifest is the bounded, validated source index available to detection.
// Providers must not read customer source directly or execute it during planning.
type SourceManifest struct {
	ArtifactID string
	Files      []SourceFile
}

// Validate rejects invalid source metadata before a provider sees it.
func (s SourceManifest) Validate() error {
	if strings.TrimSpace(s.ArtifactID) == "" {
		return errors.New("source artifact id is required")
	}
	if len(s.Files) == 0 {
		return errors.New("source manifest is empty")
	}
	seen := make(map[string]struct{}, len(s.Files))
	for _, file := range s.Files {
		if err := validateSourcePath(file.Path); err != nil {
			return err
		}
		if file.Size < 0 {
			return fmt.Errorf("source file %q has a negative size", file.Path)
		}
		if _, exists := seen[file.Path]; exists {
			return fmt.Errorf("source manifest contains duplicate path %q", file.Path)
		}
		seen[file.Path] = struct{}{}
	}
	return nil
}

func validateSourcePath(value string) error {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return fmt.Errorf("invalid source path %q", value)
	}
	clean := path.Clean(value)
	if clean == "." || clean != value || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("invalid source path %q", value)
	}
	return nil
}

// HasRootFile reports whether a manifest contains an exact root-level file.
func (s SourceManifest) HasRootFile(name string) bool {
	for _, file := range s.Files {
		if file.Path == name {
			return true
		}
	}
	return false
}

// Detection is a provider's safe classification of a source artifact.
type Detection struct {
	Provider string
	Kind     string
	Evidence []string
}

// Plan is an immutable build plan. Its fields are declarative; neither user
// source nor shell commands are accepted at this boundary.
type Plan struct {
	Provider   string
	ArtifactID string
	Kind       string
	Evidence   []string
}

// Result is the eventual build result. Only FakeProvider returns one in this
// slice; real providers fail closed until Slice 3 supplies isolated execution.
type Result struct {
	Provider       string
	ArtifactDigest string
	Simulated      bool
}

// Provider lets build engines detect, plan, and eventually build source behind
// one small interface. Build must never be called by an HTTP handler.
type Provider interface {
	Name() string
	Detect(ctx context.Context, source SourceManifest) (Detection, error)
	Plan(ctx context.Context, source SourceManifest, detection Detection) (Plan, error)
	Build(ctx context.Context, plan Plan) (Result, error)
}

// Planner selects the first provider that recognizes a validated manifest.
type Planner struct {
	providers []Provider
}

// NewPlanner constructs a planner with ordered provider preference.
func NewPlanner(providers ...Provider) (*Planner, error) {
	if len(providers) == 0 {
		return nil, errors.New("at least one build provider is required")
	}
	seen := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		if provider == nil || strings.TrimSpace(provider.Name()) == "" {
			return nil, errors.New("build provider is invalid")
		}
		if _, exists := seen[provider.Name()]; exists {
			return nil, fmt.Errorf("duplicate build provider %q", provider.Name())
		}
		seen[provider.Name()] = struct{}{}
	}
	return &Planner{providers: append([]Provider(nil), providers...)}, nil
}

// DetectAndPlan validates source metadata, selects a provider, and returns its
// declarative plan. It never invokes Build.
func (p *Planner) DetectAndPlan(ctx context.Context, source SourceManifest) (Plan, error) {
	if err := source.Validate(); err != nil {
		return Plan{}, err
	}
	for _, provider := range p.providers {
		detection, err := provider.Detect(ctx, source)
		if errors.Is(err, ErrNotDetected) {
			continue
		}
		if err != nil {
			return Plan{}, fmt.Errorf("detect with %s: %w", provider.Name(), err)
		}
		plan, err := provider.Plan(ctx, source, detection)
		if err != nil {
			return Plan{}, fmt.Errorf("plan with %s: %w", provider.Name(), err)
		}
		return normalizePlan(plan, source, provider.Name())
	}
	return Plan{}, errors.New("no build provider supports this source")
}

func normalizePlan(plan Plan, source SourceManifest, provider string) (Plan, error) {
	if plan.Provider != provider || plan.ArtifactID != source.ArtifactID {
		return Plan{}, errors.New("build provider returned a plan for another provider or artifact")
	}
	if plan.Kind != KindStatic && plan.Kind != KindApplication {
		return Plan{}, fmt.Errorf("build provider returned invalid kind %q", plan.Kind)
	}
	plan.Evidence = append([]string(nil), plan.Evidence...)
	sort.Strings(plan.Evidence)
	return plan, nil
}

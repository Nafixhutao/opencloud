package build

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPlannerPrefersStaticBeforeGenericRailpack(t *testing.T) {
	planner, err := NewPlanner(StaticProvider{}, RailpackProvider{})
	require.NoError(t, err)

	plan, err := planner.DetectAndPlan(context.Background(), SourceManifest{ArtifactID: "artifact-static", Files: []SourceFile{{Path: "index.html", Size: 32}, {Path: "assets/app.js", Size: 64}}})
	require.NoError(t, err)
	require.Equal(t, "static", plan.Provider)
	require.Equal(t, KindStatic, plan.Kind)
	require.Equal(t, []string{"index.html"}, plan.Evidence)
}

func TestPlannerUsesRailpackWithoutLanguageBranches(t *testing.T) {
	planner, err := NewPlanner(StaticProvider{}, RailpackProvider{})
	require.NoError(t, err)

	plan, err := planner.DetectAndPlan(context.Background(), SourceManifest{ArtifactID: "artifact-app", Files: []SourceFile{{Path: "src/main", Size: 64}}})
	require.NoError(t, err)
	require.Equal(t, "railpack", plan.Provider)
	require.Equal(t, KindApplication, plan.Kind)
}

func TestManifestRejectsTraversalAndDuplicatePaths(t *testing.T) {
	_, err := NewPlanner(RailpackProvider{})
	require.NoError(t, err)
	for _, manifest := range []SourceManifest{
		{ArtifactID: "bad", Files: []SourceFile{{Path: "../secret", Size: 1}}},
		{ArtifactID: "bad", Files: []SourceFile{{Path: "a", Size: 1}, {Path: "a", Size: 2}}},
		{ArtifactID: "bad", Files: []SourceFile{{Path: "a", Size: -1}}},
	} {
		require.Error(t, manifest.Validate())
	}
}

func TestRealProvidersFailClosedUntilIsolatedExecutionExists(t *testing.T) {
	result, err := StaticProvider{}.Build(context.Background(), Plan{})
	require.ErrorIs(t, err, ErrExecutionDisabled)
	require.Empty(t, result)

	result, err = RailpackProvider{}.Build(context.Background(), Plan{})
	require.ErrorIs(t, err, ErrExecutionDisabled)
	require.Empty(t, result)
}

func TestFakeProviderReturnsOnlySimulatedDigest(t *testing.T) {
	fake := FakeProvider{}
	result, err := fake.Build(context.Background(), Plan{Provider: fake.Name(), ArtifactID: "artifact", Kind: KindApplication})
	require.NoError(t, err)
	require.True(t, result.Simulated)
	require.Regexp(t, `^sha256:[0-9a-f]{64}$`, result.ArtifactDigest)
	require.False(t, errors.Is(err, ErrExecutionDisabled))
}

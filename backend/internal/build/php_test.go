package build_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nazxf/opencloud/backend/internal/build"
)

func TestPHPProviderDetection(t *testing.T) {
	ctx := context.Background()
	p := &build.PHPProvider{}

	t.Run("detects composer-json", func(t *testing.T) {
		source := build.SourceManifest{
			ArtifactID: "test",
			Files: []build.SourceFile{
				{Path: "composer.json", Size: 100},
				{Path: "index.php", Size: 50},
			},
		}
		detection, err := p.Detect(ctx, source)
		require.NoError(t, err)
		require.Equal(t, "php", detection.Provider)
		require.Equal(t, build.KindApplication, detection.Kind)
	})

	t.Run("rejects-missing-composer", func(t *testing.T) {
		source := build.SourceManifest{
			ArtifactID: "test",
			Files: []build.SourceFile{
				{Path: "index.html", Size: 50},
			},
		}
		_, err := p.Detect(ctx, source)
		require.ErrorIs(t, err, build.ErrNotDetected)
	})

	t.Run("plans-php-app", func(t *testing.T) {
		source := build.SourceManifest{
			ArtifactID: "test",
			Files: []build.SourceFile{
				{Path: "composer.json", Size: 100},
				{Path: "composer.lock", Size: 200},
				{Path: "public/index.php", Size: 50},
			},
		}
		detection, _ := p.Detect(ctx, source)
		plan, err := p.Plan(ctx, source, detection)
		require.NoError(t, err)
		require.Equal(t, "php", plan.Provider)
		require.Contains(t, plan.Evidence, "composer.lock")
		require.Contains(t, plan.Evidence, "web_root:public")
	})

	t.Run("build-is-gated", func(t *testing.T) {
		_, err := p.Build(ctx, build.Plan{})
		require.ErrorIs(t, err, build.ErrExecutionDisabled)
	})
}

func TestPlannerIntegration(t *testing.T) {
	ctx := context.Background()

	t.Run("planner-selects-php", func(t *testing.T) {
		planner, err := build.NewPlanner(&build.PHPProvider{}, &build.StaticProvider{})
		require.NoError(t, err)

		source := build.SourceManifest{
			ArtifactID: "test",
			Files: []build.SourceFile{
				{Path: "composer.json", Size: 100},
				{Path: "index.php", Size: 50},
			},
		}
		plan, err := planner.DetectAndPlan(ctx, source)
		require.NoError(t, err)
		require.Equal(t, "php", plan.Provider)
	})

	t.Run("planner-falls-back-when-no-match", func(t *testing.T) {
		planner, err := build.NewPlanner(&build.PHPProvider{})
		require.NoError(t, err)

		source := build.SourceManifest{
			ArtifactID: "test",
			Files: []build.SourceFile{
				{Path: "app.py", Size: 50},
			},
		}
		_, err = planner.DetectAndPlan(ctx, source)
		require.Error(t, err)
	})

	t.Run("planner-with-all-providers", func(t *testing.T) {
		planner, err := build.NewPlanner(&build.StaticProvider{}, &build.PHPProvider{}, &build.RailpackProvider{})
		require.NoError(t, err)

		// StaticProvider matches index.html
		source := build.SourceManifest{
			ArtifactID: "test",
			Files: []build.SourceFile{
				{Path: "index.html", Size: 100},
			},
		}
		plan, err := planner.DetectAndPlan(ctx, source)
		require.NoError(t, err)
		require.Equal(t, "static", plan.Provider)
	})
}

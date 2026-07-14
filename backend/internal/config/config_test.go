package config

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// TestViper_MissingConfigFileErrorType records exactly what ReadInConfig returns
// when the SetConfigFile target is absent. This is the evidence for whether the
// errors.As(ConfigFileNotFoundError) arm in Load() is reachable, or whether a
// single errors.Is(fs.ErrNotExist) check is enough (review finding #3).
func TestViper_MissingConfigFileErrorType(t *testing.T) {
	v := viper.New()
	v.SetConfigFile(filepath.Join(t.TempDir(), ".env")) // guaranteed missing
	v.SetConfigType("env")

	err := v.ReadInConfig()
	if err == nil {
		t.Fatal("expected an error reading a missing config file")
	}

	asNotFound := errors.As(err, new(viper.ConfigFileNotFoundError))
	isNotExist := errors.Is(err, fs.ErrNotExist)

	t.Logf("concrete type: %T", err)
	t.Logf("errors.As(ConfigFileNotFoundError) = %v", asNotFound)
	t.Logf("errors.Is(fs.ErrNotExist)          = %v", isNotExist)

	// The simplification (drop the ConfigFileNotFoundError arm) is safe ONLY if
	// the missing-file error is caught by errors.Is(fs.ErrNotExist).
	if !isNotExist {
		t.Errorf("SIMPLIFICATION UNSAFE: missing .env is NOT an fs.ErrNotExist; "+
			"removing the ConfigFileNotFoundError arm would fail boot (as=%v)", asNotFound)
	}
}

// TestLoad_NoEnvFile_SucceedsFromEnvVars proves the real boot path: with no
// .env on disk (as in prod / the distroless image) but required env vars set,
// Load() must succeed. Guards against a regression that would break api/worker
// startup wherever config comes purely from the environment.
func TestLoad_NoEnvFile_SucceedsFromEnvVars(t *testing.T) {
	t.Chdir(t.TempDir()) // no .env here; SetConfigFile(".env") will miss
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db?sslmode=disable")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed with no .env but env vars set: %v", err)
	}
	if cfg.DatabaseURL == "" || cfg.RedisURL == "" {
		t.Fatalf("env vars not picked up: db=%q redis=%q", cfg.DatabaseURL, cfg.RedisURL)
	}
}

func TestLoadForMigration_DoesNotRequireRedis(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db?sslmode=disable")
	t.Setenv("REDIS_URL", "")

	cfg, err := LoadForMigration()
	if err != nil {
		t.Fatalf("LoadForMigration() unexpectedly required Redis: %v", err)
	}
	if cfg.DatabaseURL == "" {
		t.Fatal("DATABASE_URL was not loaded")
	}
}

func TestLoad_RequiresRedis(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db?sslmode=disable")
	t.Setenv("REDIS_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() succeeded without REDIS_URL")
	}
}

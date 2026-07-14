// Package config loads all runtime configuration once at startup into a typed
// Config struct, sourced from environment variables (see docs/INFRASTRUCTURE.md §3).
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/spf13/viper"
)

// Config is the typed application configuration. It is loaded once at boot and
// passed down via dependency injection — never read viper deep in the code.
type Config struct {
	Env         string `mapstructure:"ENV"`
	HTTPAddr    string `mapstructure:"HTTP_ADDR"`
	MetricsAddr string `mapstructure:"METRICS_ADDR"`
	LogLevel    string `mapstructure:"LOG_LEVEL"`
	DatabaseURL string `mapstructure:"DATABASE_URL"`
	RedisURL    string `mapstructure:"REDIS_URL"`

	// Wired in later phases; loaded now so config never churns when they land.
	AuthJWKSURL  string       `mapstructure:"AUTH_JWKS_URL"` // better-auth JWKS (ADR 0006)
	AuthIssuer   string       `mapstructure:"AUTH_ISSUER"`   // expected JWT iss (empty = skip check)
	AuthAudience string       `mapstructure:"AUTH_AUDIENCE"` // expected JWT aud (empty = skip check)
	CORSOrigins  string       `mapstructure:"CORS_ORIGINS"`
	RateLimitRPS int          `mapstructure:"RATE_LIMIT_RPS"`
	Hestia       HestiaConfig `mapstructure:",squash"`
}

// HestiaConfig holds the provisioner's node credentials (used from Phase 2).
type HestiaConfig struct {
	APIURL string `mapstructure:"HESTIA_API_URL"`
	APIKey string `mapstructure:"HESTIA_API_KEY"`
}

// Load reads API/worker configuration, including both datastores.
func Load() (*Config, error) {
	return load(true)
}

// LoadForMigration reads the subset needed by the migration command.
func LoadForMigration() (*Config, error) {
	return load(false)
}

func load(requireRedis bool) (*Config, error) {
	v := viper.New()

	v.SetDefault("ENV", "development")
	v.SetDefault("HTTP_ADDR", ":8080")
	v.SetDefault("METRICS_ADDR", ":9090")
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("RATE_LIMIT_RPS", 10)

	// Explicitly bind every key: viper's Unmarshal only sees keys it already
	// knows, and AutomaticEnv alone doesn't register them — so without this,
	// config from real env vars (prod, where .env is absent) would be dropped.
	for _, key := range []string{
		"ENV", "HTTP_ADDR", "METRICS_ADDR", "LOG_LEVEL", "DATABASE_URL", "REDIS_URL",
		"AUTH_JWKS_URL", "AUTH_ISSUER", "AUTH_AUDIENCE", "CORS_ORIGINS", "RATE_LIMIT_RPS",
		"HESTIA_API_URL", "HESTIA_API_KEY",
	} {
		_ = v.BindEnv(key)
	}

	// .env is optional (present in dev, absent in prod where env is injected).
	// A missing file surfaces as *fs.PathError (errors.Is fs.ErrNotExist), never
	// viper.ConfigFileNotFoundError — proven in config_test.go — so one check
	// suffices; any other error (e.g. malformed .env) is real.
	v.SetConfigFile(".env")
	v.SetConfigType("env")
	if err := v.ReadInConfig(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read .env: %w", err)
	}

	v.AutomaticEnv() // real env vars override .env

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := cfg.validate(requireRedis); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) validate(requireRedis bool) error {
	var missing []string
	if c.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if requireRedis && c.RedisURL == "" {
		missing = append(missing, "REDIS_URL")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}
	return nil
}

// IsProduction reports whether the app runs with production semantics.
func (c *Config) IsProduction() bool { return c.Env == "production" }

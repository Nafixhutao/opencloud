// Package config loads all runtime configuration once at startup into a typed
// Config struct, sourced from environment variables (see docs/INFRASTRUCTURE.md §3).
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/nazxf/opencloud/backend/internal/provisioner"
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
	AuthJWKSURL  string            `mapstructure:"AUTH_JWKS_URL"` // better-auth JWKS (ADR 0006)
	AuthIssuer   string            `mapstructure:"AUTH_ISSUER"`   // expected JWT iss (empty = skip check)
	AuthAudience string            `mapstructure:"AUTH_AUDIENCE"` // expected JWT aud (empty = skip check)
	CORSOrigins  string            `mapstructure:"CORS_ORIGINS"`
	RateLimitRPS int               `mapstructure:"RATE_LIMIT_RPS"`
	Provisioner  ProvisionerConfig `mapstructure:",squash"`
}

// ProvisionerConfig selects the hosting backend and carries only the connection
// details the worker needs. Hestia stays available as a documented fallback.
type ProvisionerConfig struct {
	Backend       provisioner.Backend `mapstructure:"PROVISIONER_BACKEND"`
	DockerSocket  string              `mapstructure:"DOCKER_SOCKET"`
	CaddyAPIURL   string              `mapstructure:"CADDY_API_URL"`
	CaddyServerID string              `mapstructure:"CADDY_SERVER_ID"`
	SiteImage     string              `mapstructure:"SITE_DEFAULT_IMAGE"`
	Hestia        HestiaConfig        `mapstructure:",squash"`
}

// HestiaConfig holds fallback Hestia credentials. Access/secret keys are the
// preferred scoped authentication mechanism; APIKey exists for legacy nodes.
type HestiaConfig struct {
	APIURL    string `mapstructure:"HESTIA_API_URL"`
	AccessKey string `mapstructure:"HESTIA_ACCESS_KEY"`
	SecretKey string `mapstructure:"HESTIA_SECRET_KEY"`
	APIKey    string `mapstructure:"HESTIA_API_KEY"`
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
	v.SetDefault("PROVISIONER_BACKEND", string(provisioner.BackendFake))
	v.SetDefault("DOCKER_SOCKET", "/var/run/docker.sock")
	v.SetDefault("CADDY_API_URL", "http://127.0.0.1:2019")
	v.SetDefault("CADDY_SERVER_ID", "srv0")
	v.SetDefault("SITE_DEFAULT_IMAGE", "opencloud/site-static:phase2")

	// Explicitly bind every key: viper's Unmarshal only sees keys it already
	// knows, and AutomaticEnv alone doesn't register them — so without this,
	// config from real env vars (prod, where .env is absent) would be dropped.
	for _, key := range []string{
		"ENV", "HTTP_ADDR", "METRICS_ADDR", "LOG_LEVEL", "DATABASE_URL", "REDIS_URL",
		"AUTH_JWKS_URL", "AUTH_ISSUER", "AUTH_AUDIENCE", "CORS_ORIGINS", "RATE_LIMIT_RPS",
		"PROVISIONER_BACKEND", "DOCKER_SOCKET", "CADDY_API_URL", "CADDY_SERVER_ID",
		"SITE_DEFAULT_IMAGE",
		"HESTIA_API_URL", "HESTIA_ACCESS_KEY", "HESTIA_SECRET_KEY", "HESTIA_API_KEY",
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

// ValidateAPI enforces settings used only by the HTTP API.
func (c *Config) ValidateAPI() error {
	if !c.IsProduction() {
		return nil
	}

	var missing []string
	// In production, iss/aud validation must not be a silent no-op: an empty
	// value makes Auth skip that check, so a token merely signed by the trusted
	// JWKS would pass regardless of who it was issued for. Fail fast instead
	// (better-auth always emits iss/aud — default the BFF base URL, ADR 0006).
	if c.AuthIssuer == "" {
		missing = append(missing, "AUTH_ISSUER")
	}
	if c.AuthAudience == "" {
		missing = append(missing, "AUTH_AUDIENCE")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required API config: %s", strings.Join(missing, ", "))
	}
	return nil
}

// ValidateProvisioner enforces worker-only hosting backend configuration.
func (c *Config) ValidateProvisioner() error {
	backend, err := provisioner.ParseBackend(string(c.Provisioner.Backend))
	if err != nil {
		return err
	}

	switch backend {
	case provisioner.BackendDocker:
		return requireConfig(map[string]string{
			"CADDY_API_URL":      c.Provisioner.CaddyAPIURL,
			"CADDY_SERVER_ID":    c.Provisioner.CaddyServerID,
			"DOCKER_SOCKET":      c.Provisioner.DockerSocket,
			"SITE_DEFAULT_IMAGE": c.Provisioner.SiteImage,
		})
	case provisioner.BackendHestia:
		if err := requireConfig(map[string]string{"HESTIA_API_URL": c.Provisioner.Hestia.APIURL}); err != nil {
			return err
		}
		hasAccessPair := c.Provisioner.Hestia.AccessKey != "" && c.Provisioner.Hestia.SecretKey != ""
		if !hasAccessPair && c.Provisioner.Hestia.APIKey == "" {
			return errors.New("missing required Hestia credentials: set HESTIA_ACCESS_KEY and HESTIA_SECRET_KEY")
		}
		if (c.Provisioner.Hestia.AccessKey == "") != (c.Provisioner.Hestia.SecretKey == "") {
			return errors.New("HESTIA_ACCESS_KEY and HESTIA_SECRET_KEY must be set together")
		}
		return nil
	case provisioner.BackendFake:
		if c.IsProduction() {
			return errors.New("PROVISIONER_BACKEND=fake is not allowed in production")
		}
		return nil
	default:
		return fmt.Errorf("unsupported provisioner backend %q", backend)
	}
}

func requireConfig(values map[string]string) error {
	var missing []string
	for name, value := range values {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("missing required provisioner config: %s", strings.Join(missing, ", "))
	}
	return nil
}

// IsProduction reports whether the app runs with production semantics.
func (c *Config) IsProduction() bool { return c.Env == "production" }

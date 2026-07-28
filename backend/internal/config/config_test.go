package config

import (
	"crypto/rand"
	"encoding/base64"
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

// TestValidateAPI_ProductionRequiresIssuerAudience guards the security fix:
// only the production API requires iss/aud, while development stays lenient.
func TestValidateAPI_ProductionRequiresIssuerAudience(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		issuer   string
		audience string
		wantErr  bool
	}{
		{name: "production without iss or aud fails", env: "production", wantErr: true},
		{name: "production without iss fails", env: "production", audience: "https://auth.example.com", wantErr: true},
		{name: "production without aud fails", env: "production", issuer: "https://auth.example.com", wantErr: true},
		{name: "production with iss and aud succeeds", env: "production", issuer: "https://auth.example.com", audience: "https://auth.example.com"},
		{name: "development without iss or aud succeeds", env: "development"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("ENV", tt.env)
			t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db?sslmode=disable")
			t.Setenv("REDIS_URL", "redis://localhost:6379/0")
			t.Setenv("AUTH_ISSUER", tt.issuer)
			t.Setenv("AUTH_AUDIENCE", tt.audience)

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() unexpectedly failed: %v", err)
			}
			err = cfg.ValidateAPI()
			if tt.wantErr && err == nil {
				t.Fatal("ValidateAPI() succeeded with incomplete production auth config")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateAPI() unexpectedly failed: %v", err)
			}
		})
	}
}

func TestLoad_ProductionWorkerDoesNotRequireAPIAuth(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db?sslmode=disable")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("AUTH_ISSUER", "")
	t.Setenv("AUTH_AUDIENCE", "")

	if _, err := Load(); err != nil {
		t.Fatalf("worker config unexpectedly required API auth: %v", err)
	}
}

func TestLoadForMigration_ProductionRequiresOnlyDatabase(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db?sslmode=disable")
	t.Setenv("REDIS_URL", "")
	t.Setenv("AUTH_ISSUER", "")
	t.Setenv("AUTH_AUDIENCE", "")

	if _, err := LoadForMigration(); err != nil {
		t.Fatalf("migration config required an unrelated service setting: %v", err)
	}
}

func TestValidateProvisioner(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "Docker defaults are complete",
			cfg: Config{Provisioner: ProvisionerConfig{
				Backend:       "docker",
				DockerSocket:  "/var/run/docker.sock",
				CaddyAPIURL:   "http://127.0.0.1:2019",
				CaddyServerID: "srv0",
				SiteImage:     "opencloud/site-static:phase2",
			}},
		},
		{
			name: "Hestia accepts scoped access pair",
			cfg: Config{Provisioner: ProvisionerConfig{
				Backend: "hestia",
				Hestia: HestiaConfig{
					APIURL:    "https://node.example.com:8083",
					AccessKey: "access",
					SecretKey: "secret",
				},
			}},
		},
		{
			name: "Hestia rejects half an access pair",
			cfg: Config{Provisioner: ProvisionerConfig{
				Backend: "hestia",
				Hestia: HestiaConfig{
					APIURL:    "https://node.example.com:8083",
					AccessKey: "access",
				},
			}},
			wantErr: true,
		},
		{
			name: "fake is forbidden in production",
			cfg: Config{
				Env:         "production",
				Provisioner: ProvisionerConfig{Backend: "fake"},
			},
			wantErr: true,
		},
		{
			name:    "unknown backend is rejected",
			cfg:     Config{Provisioner: ProvisionerConfig{Backend: "unknown"}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.ValidateProvisioner()
			if tt.wantErr && err == nil {
				t.Fatal("ValidateProvisioner() succeeded, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateProvisioner() error = %v", err)
			}
		})
	}
}

func TestCustomerDatabaseConfigurationFailsClosed(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		t.Fatal(err)
	}
	encodedKey := base64.StdEncoding.EncodeToString(key)

	tests := []struct {
		name        string
		cfg         Config
		validateAPI bool
		wantErr     bool
	}{
		{
			name:        "disabled capability needs no secrets",
			cfg:         Config{Provisioner: ProvisionerConfig{Backend: "fake"}},
			validateAPI: true,
		},
		{
			name: "enabled API rejects missing credential key",
			cfg: Config{CustomerDatabases: CustomerDatabaseConfig{
				Enabled:      true,
				PostgresHost: "postgres.example.test",
				PostgresPort: 5432,
				MariaDBHost:  "mariadb.example.test",
				MariaDBPort:  3306,
				TLSRequired:  true,
			}},
			validateAPI: true,
			wantErr:     true,
		},
		{
			name: "enabled worker requires separate admin targets",
			cfg: Config{
				Provisioner: ProvisionerConfig{Backend: "fake"},
				CustomerDatabases: CustomerDatabaseConfig{
					Enabled:       true,
					CredentialKey: encodedKey,
					PostgresHost:  "postgres.example.test",
					PostgresPort:  5432,
					MariaDBHost:   "mariadb.example.test",
					MariaDBPort:   3306,
					TLSRequired:   true,
				},
			},
			wantErr: true,
		},
		{
			name: "complete API configuration succeeds",
			cfg: Config{CustomerDatabases: CustomerDatabaseConfig{
				Enabled:       true,
				CredentialKey: encodedKey,
				PostgresHost:  "postgres.example.test",
				PostgresPort:  5432,
				MariaDBHost:   "mariadb.example.test",
				MariaDBPort:   3306,
				TLSRequired:   true,
			}},
			validateAPI: true,
		},
		{
			name: "complete worker configuration succeeds",
			cfg: Config{
				DatabaseURL: "postgres://control:secret@control.internal:5432/opencloud?sslmode=disable",
				Provisioner: ProvisionerConfig{Backend: "fake"},
				CustomerDatabases: CustomerDatabaseConfig{
					Enabled:          true,
					CredentialKey:    encodedKey,
					PostgresAdminURL: "postgres://admin:secret@postgres.internal/postgres",
					PostgresHost:     "postgres.example.test",
					PostgresPort:     5432,
					MariaDBAdminDSN:  "admin:secret@tcp(mariadb.internal:3306)/",
					MariaDBHost:      "mariadb.example.test",
					MariaDBPort:      3306,
					TLSRequired:      true,
				},
			},
		},
		{
			name: "worker rejects customer PostgreSQL on control-plane target",
			cfg: Config{
				DatabaseURL: "postgres://control:secret@postgres.internal:5432/opencloud?sslmode=disable",
				Provisioner: ProvisionerConfig{Backend: "fake"},
				CustomerDatabases: CustomerDatabaseConfig{
					Enabled:          true,
					CredentialKey:    encodedKey,
					PostgresAdminURL: "postgres://admin:secret@postgres.internal:5432/postgres?sslmode=disable",
					PostgresHost:     "postgres.example.test",
					PostgresPort:     5432,
					MariaDBAdminDSN:  "admin:secret@tcp(mariadb.internal:3306)/",
					MariaDBHost:      "mariadb.example.test",
					MariaDBPort:      3306,
					TLSRequired:      true,
				},
			},
			wantErr: true,
		},
		{
			name: "production worker requires TLS for both admin targets",
			cfg: Config{
				Env:         "production",
				DatabaseURL: "postgres://control:secret@control.internal:5432/opencloud?sslmode=verify-full",
				Provisioner: ProvisionerConfig{
					Backend:       "docker",
					DockerSocket:  "/var/run/docker.sock",
					CaddyAPIURL:   "https://caddy.internal:2019",
					CaddyServerID: "srv0",
					SiteImage:     "opencloud/site-static:phase2",
				},
				CustomerDatabases: CustomerDatabaseConfig{
					Enabled:          true,
					CredentialKey:    encodedKey,
					PostgresAdminURL: "postgres://admin:secret@postgres.internal:5432/postgres?sslmode=disable",
					PostgresHost:     "postgres.example.test",
					PostgresPort:     5432,
					MariaDBAdminDSN:  "admin:secret@tcp(mariadb.internal:3306)/?tls=false",
					MariaDBHost:      "mariadb.example.test",
					MariaDBPort:      3306,
					TLSRequired:      true,
				},
			},
			wantErr: true,
		},
		{
			name: "production worker rejects unverified MariaDB TLS",
			cfg: Config{
				Env:         "production",
				DatabaseURL: "postgres://control:secret@control.internal:5432/opencloud?sslmode=verify-full",
				Provisioner: ProvisionerConfig{
					Backend:       "docker",
					DockerSocket:  "/var/run/docker.sock",
					CaddyAPIURL:   "https://caddy.internal:2019",
					CaddyServerID: "srv0",
					SiteImage:     "opencloud/site-static:phase2",
				},
				CustomerDatabases: CustomerDatabaseConfig{
					Enabled:          true,
					CredentialKey:    encodedKey,
					PostgresAdminURL: "postgres://admin:secret@postgres.internal:5432/postgres?sslmode=verify-full",
					PostgresHost:     "postgres.example.test",
					PostgresPort:     5432,
					MariaDBAdminDSN:  "admin:secret@tcp(mariadb.internal:3306)/?tls=skip-verify",
					MariaDBHost:      "mariadb.example.test",
					MariaDBPort:      3306,
					TLSRequired:      true,
				},
			},
			wantErr: true,
		},
		{
			name: "production worker accepts verified TLS admin targets",
			cfg: Config{
				Env:         "production",
				DatabaseURL: "postgres://control:secret@control.internal:5432/opencloud?sslmode=verify-full",
				Provisioner: ProvisionerConfig{
					Backend:       "docker",
					DockerSocket:  "/var/run/docker.sock",
					CaddyAPIURL:   "https://caddy.internal:2019",
					CaddyServerID: "srv0",
					SiteImage:     "opencloud/site-static:phase2",
				},
				CustomerDatabases: CustomerDatabaseConfig{
					Enabled:          true,
					CredentialKey:    encodedKey,
					PostgresAdminURL: "postgres://admin:secret@postgres.internal:5432/postgres?sslmode=verify-full",
					PostgresHost:     "postgres.example.test",
					PostgresPort:     5432,
					MariaDBAdminDSN:  "admin:secret@tcp(mariadb.internal:3306)/?tls=true",
					MariaDBHost:      "mariadb.example.test",
					MariaDBPort:      3306,
					TLSRequired:      true,
				},
			},
		},
		{
			name: "production rejects plaintext customer endpoints",
			cfg: Config{
				Env:          "production",
				AuthIssuer:   "https://auth.example.test",
				AuthAudience: "https://auth.example.test",
				CustomerDatabases: CustomerDatabaseConfig{
					Enabled:       true,
					CredentialKey: encodedKey,
					PostgresHost:  "postgres.example.test",
					PostgresPort:  5432,
					MariaDBHost:   "mariadb.example.test",
					MariaDBPort:   3306,
					TLSRequired:   false,
				},
			},
			validateAPI: true,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.validateAPI {
				err = tt.cfg.ValidateAPI()
			} else {
				err = tt.cfg.ValidateProvisioner()
			}
			if tt.wantErr && err == nil {
				t.Fatal("validation succeeded, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validation failed: %v", err)
			}
		})
	}
}

func TestProductionCustomerPostgresRequiresCertificateAndHostnameVerification(
	t *testing.T,
) {
	encodedKey := base64.StdEncoding.EncodeToString(make([]byte, 32))
	tests := []struct {
		sslMode string
		wantErr bool
	}{
		{sslMode: "disable", wantErr: true},
		{sslMode: "allow", wantErr: true},
		{sslMode: "prefer", wantErr: true},
		{sslMode: "require", wantErr: true},
		{sslMode: "verify-ca", wantErr: true},
		{sslMode: "verify-full"},
	}

	for _, tt := range tests {
		t.Run(tt.sslMode, func(t *testing.T) {
			cfg := Config{
				Env:         "production",
				DatabaseURL: "postgres://control:secret@control.internal:5432/opencloud?sslmode=verify-full",
				Provisioner: ProvisionerConfig{
					Backend:       "docker",
					DockerSocket:  "/var/run/docker.sock",
					CaddyAPIURL:   "https://caddy.internal:2019",
					CaddyServerID: "srv0",
					SiteImage:     "opencloud/site-static:phase2",
				},
				CustomerDatabases: CustomerDatabaseConfig{
					Enabled:          true,
					CredentialKey:    encodedKey,
					PostgresAdminURL: "postgres://admin:secret@postgres.internal:5432/postgres?sslmode=" + tt.sslMode,
					PostgresHost:     "postgres.example.test",
					PostgresPort:     5432,
					MariaDBAdminDSN:  "admin:secret@tcp(mariadb.internal:3306)/?tls=true",
					MariaDBHost:      "mariadb.example.test",
					MariaDBPort:      3306,
					TLSRequired:      true,
				},
			}

			err := cfg.ValidateProvisioner()
			if tt.wantErr && err == nil {
				t.Fatalf("sslmode=%s unexpectedly passed production validation", tt.sslMode)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("sslmode=%s unexpectedly failed production validation: %v", tt.sslMode, err)
			}
		})
	}
}

// Package app holds the shared startup wiring used by every entrypoint
// (cmd/api, cmd/worker, cmd/migrate). Keeping it in one place means the three
// binaries load config, logging and datastores identically (BACKEND.md §3).
package app

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/uptrace/bun"
	"go.uber.org/zap"

	"github.com/nazxf/opencloud/backend/internal/cache"
	"github.com/nazxf/opencloud/backend/internal/config"
	"github.com/nazxf/opencloud/backend/internal/database"
	"github.com/nazxf/opencloud/backend/internal/logging"
)

// Deps is the set of long-lived dependencies shared by the binaries.
type Deps struct {
	Cfg *config.Config
	Log *zap.Logger
	DB  *bun.DB
	RDB *redis.Client
}

// Close releases datastore handles. Safe to call on a partially-built Deps.
func (d *Deps) Close() {
	if d.DB != nil {
		_ = d.DB.Close()
	}
	if d.RDB != nil {
		_ = d.RDB.Close()
	}
	if d.Log != nil {
		_ = d.Log.Sync()
	}
}

// Bootstrap loads config and logging and, when withStores is true, connects
// both PostgreSQL and Redis (used by cmd/api and cmd/worker). cmd/migrate needs
// only the DB, so it passes withStores=false and calls database.Connect itself.
// On any error the caller gets a partially-built Deps it should Close.
func Bootstrap(ctx context.Context, withStores bool) (*Deps, error) {
	var cfg *config.Config
	var err error
	if withStores {
		cfg, err = config.Load()
	} else {
		cfg, err = config.LoadForMigration()
	}
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	log, err := logging.New(cfg.Env, cfg.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("init logging: %w", err)
	}

	d := &Deps{Cfg: cfg, Log: log}
	if !withStores {
		return d, nil
	}

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		d.Close()
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	d.DB = db

	rdb, err := cache.Connect(ctx, cfg.RedisURL)
	if err != nil {
		d.Close()
		return nil, fmt.Errorf("connect redis: %w", err)
	}
	d.RDB = rdb

	return d, nil
}

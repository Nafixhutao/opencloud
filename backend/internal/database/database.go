// Package database wires the PostgreSQL connection: a pgx-backed database/sql
// pool with Bun (pgdialect) on top. Bun is the ORM used by every repository.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// Pool sizing. Deliberate, not per-request connections (INFRASTRUCTURE.md §9).
const (
	maxOpenConns    = 25
	maxIdleConns    = 25
	connMaxLifetime = 30 * time.Minute
)

// Connect opens a pooled connection to PostgreSQL via the pgx driver and wraps
// it with Bun. It pings once so a misconfigured DSN fails fast at boot.
func Connect(ctx context.Context, dsn string) (*bun.DB, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database dsn: %w", err)
	}

	sqldb := stdlib.OpenDB(*cfg)
	sqldb.SetMaxOpenConns(maxOpenConns)
	sqldb.SetMaxIdleConns(maxIdleConns)
	sqldb.SetConnMaxLifetime(connMaxLifetime)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sqldb.PingContext(pingCtx); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return bun.NewDB(sqldb, pgdialect.New()), nil
}

// Ping checks database reachability with a short deadline. Used by /readyz.
func Ping(ctx context.Context, db *bun.DB) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return db.PingContext(ctx)
}

// InTx runs fn inside a single transaction, committing on success and rolling
// back on error or panic. Services use this so multi-write operations never
// leave partial state (BACKEND.md §6).
func InTx(ctx context.Context, db *bun.DB, fn func(ctx context.Context, tx bun.Tx) error) error {
	return db.RunInTx(ctx, &sql.TxOptions{}, fn)
}

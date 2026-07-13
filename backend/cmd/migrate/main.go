// Command migrate runs Bun database migrations: `up`, `down`, `status`
// (BACKEND.md §3). Migrations are registered in the migrations package; until
// the first schema migration lands the registry is empty and `up` is a no-op.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/uptrace/bun/migrate"
	"go.uber.org/zap"

	"github.com/nazxf/opencloud/backend/internal/app"
	"github.com/nazxf/opencloud/backend/internal/database"
	"github.com/nazxf/opencloud/backend/migrations"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: migrate <up|down|status>")
		os.Exit(2)
	}
	cmd := os.Args[1]

	ctx := context.Background()

	// migrate needs config/logging but not Redis; connect the DB directly.
	deps, err := app.Bootstrap(ctx, false)
	if err != nil {
		panic(err)
	}
	defer deps.Close()

	db, err := database.Connect(ctx, deps.Cfg.DatabaseURL)
	if err != nil {
		deps.Log.Fatal("connect postgres", zap.Error(err))
	}
	defer db.Close()

	migrator := migrate.NewMigrator(
		db,
		migrations.Migrations,
		migrate.WithMarkAppliedOnSuccess(true),
	)
	if err := migrator.Init(ctx); err != nil {
		deps.Log.Fatal("migrator init", zap.Error(err))
	}

	if err := run(ctx, deps.Log, migrator, cmd); err != nil {
		deps.Log.Fatal("migrate", zap.String("cmd", cmd), zap.Error(err))
	}
}

func run(ctx context.Context, log *zap.Logger, m *migrate.Migrator, cmd string) error {
	switch cmd {
	case "up":
		group, err := m.Migrate(ctx)
		if err != nil {
			return err
		}
		if group.IsZero() {
			log.Info("no new migrations to run")
			return nil
		}
		log.Info("migrations applied", zap.String("group", group.String()))
		return nil

	case "down":
		group, err := m.Rollback(ctx)
		if err != nil {
			return err
		}
		if group.IsZero() {
			log.Info("no migration group to roll back")
			return nil
		}
		log.Info("migrations rolled back", zap.String("group", group.String()))
		return nil

	case "status":
		ms, err := m.MigrationsWithStatus(ctx)
		if err != nil {
			return err
		}
		log.Info("migration status",
			zap.String("applied", ms.Applied().String()),
			zap.String("unapplied", ms.Unapplied().String()),
			zap.Int64("last_group", ms.LastGroupID()),
		)
		return nil

	default:
		return fmt.Errorf("unknown command %q (want up|down|status)", cmd)
	}
}

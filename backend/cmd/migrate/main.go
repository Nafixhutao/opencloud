// Command migrate runs Bun database migrations: `up`, `down`, and `status`
// (BACKEND.md §3). Migrations are registered in the migrations package.
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
	defer func() {
		if err := db.Close(); err != nil {
			deps.Log.Error("close postgres", zap.Error(err))
		}
	}()

	migrator := migrate.NewMigrator(
		db,
		migrations.Migrations,
		migrate.WithMarkAppliedOnSuccess(true),
		migrate.WithUpsert(true),
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
		ms, err := m.MigrationsWithStatus(ctx)
		if err != nil {
			return err
		}
		unapplied := ms.Unapplied()
		if len(unapplied) == 0 {
			log.Info("no new migrations to run")
			return nil
		}
		names := make([]string, 0, len(unapplied))
		// Bun's Migrate applies every pending file as one rollback group. That
		// makes a fresh install's first `down` remove the entire schema. Apply
		// each pending migration as its own group so rollback is always limited
		// to the newest migration and shipped history remains immutable.
		for i := range unapplied {
			if err := m.RunMigration(ctx, unapplied[i].Name); err != nil {
				return err
			}
			names = append(names, unapplied[i].Name)
		}
		log.Info("migrations applied", zap.Strings("migrations", names))
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

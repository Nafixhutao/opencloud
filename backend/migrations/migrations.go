// Package migrations holds the Bun migration registry. SQL migration files in
// this directory (<timestamp>_<name>.up.sql / .down.sql) are embedded and
// discovered at init. The auth.* tables are owned by better-auth's own
// migrations, not Bun (ADR 0006) — migrations here touch only public.*.
package migrations

import (
	"embed"

	"github.com/uptrace/bun/migrate"
)

//go:embed *.sql
var sqlMigrations embed.FS

// Migrations is the registry the migrate command runs against.
var Migrations = migrate.NewMigrations()

func init() {
	if err := Migrations.Discover(sqlMigrations); err != nil {
		panic(err)
	}
}

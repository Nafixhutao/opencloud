// Package migrations holds the Bun migration registry. SQL migration files in
// this directory (<timestamp>_<name>.up.sql / .down.sql) are embedded and
// discovered at init. Bun bootstraps the auth schema, but its tables remain
// owned by better-auth's own migrations (ADR 0006).
package migrations

import (
	"embed"

	"github.com/uptrace/bun/migrate"
)

//go:embed *.sql checksums.sha256
var sqlMigrations embed.FS

// Migrations is the registry the migrate command runs against.
var Migrations = migrate.NewMigrations()

func init() {
	if err := Migrations.Discover(sqlMigrations); err != nil {
		panic(err)
	}
}

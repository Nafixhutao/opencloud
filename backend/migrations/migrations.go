// Package migrations holds the Bun migration registry. The initial domain
// schema (public.accounts, …) lands in a later Phase 0 item; until then the
// registry is empty and `migrate up` is a safe no-op. The auth.* tables are
// owned by better-auth's own migrations, not Bun (ADR 0006).
//
// When the first migration is added, register it here — either via
// Migrations.Discover(embed.FS) over embedded *.sql files, or MustRegister with
// Go funcs (BACKEND.md §13: Bun's own migrate tool, no external migrator).
package migrations

import "github.com/uptrace/bun/migrate"

// Migrations is the registry the migrate command runs against.
var Migrations = migrate.NewMigrations()

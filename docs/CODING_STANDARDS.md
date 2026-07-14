# Coding Standards

Conventions for all OpenCloud code. These keep the codebase legible enough that
any engineer — or Claude — can extend it safely from the docs alone. Language
deep-dives: [`BACKEND.md`](BACKEND.md), [`FRONTEND.md`](FRONTEND.md).

---

## 1. Universal principles

- **Code is read far more than written.** Optimize for the next reader.
- **Smallest change that solves the problem.** No speculative abstraction (YAGNI).
- **One responsibility** per function/file. **Delete** dead code, don't comment it out.
- **No magic values** — name constants and config.
- **Comments explain *why*, not *what*.** The code already says what.
- **Boring over clever.** Clever is what someone decodes at 3am.
- **DRY**, but don't over-abstract — duplication is cheaper than the wrong abstraction.

## 2. Naming conventions

| Thing | Convention | Example |
|---|---|---|
| Go package | short, lowercase, no underscores | `provisioner` |
| Go exported | `PascalCase` | `CreateSite` |
| Go unexported | `camelCase` | `hestiaClient` |
| Go interface | capability + `-er` | `SiteProvisioner` |
| Go file | `snake_case.go` | `site_service.go` |
| Go test | `*_test.go`, `TestXxx` | `site_service_test.go` |
| SQL table | `snake_case`, plural | `hosting_accounts` |
| SQL column | `snake_case` | `created_at` |
| JSON / API field | `snake_case` | `account_id` |
| TS component | `PascalCase.tsx` | `SiteCard.tsx` |
| TS hook | `useCamelCase.ts` | `useSites.ts` |
| TS util/module | `kebab-case.ts` | `api-client.ts` |
| TS var/function | `camelCase` | `fetchSites` |
| TS type/interface | `PascalCase` | `HostingAccount` |
| Env var | `UPPER_SNAKE_CASE` | `DATABASE_URL` |
| Route segment | `kebab-case` | `/payment-methods` |

Booleans read as predicates: `isActive`, `hasSSL`, `canSuspend`.

## 3. Go standards

- **Format with `gofmt`; lint with `golangci-lint`.** CI fails on either.
- **Return errors, don't panic.** Panic only for unrecoverable startup faults; a
  recovery middleware catches the rest ([`BACKEND.md`](BACKEND.md#10-error-handling)).
- **Wrap errors with context:** `fmt.Errorf("provision site %q: %w", domain, err)`.
- **`context.Context` is the first parameter** on anything doing I/O; set timeouts.
- **Accept interfaces, return concrete types**; define interfaces in the consumer.
- **No global mutable state** except the config and logger singletons.
- Keep functions short; prefer early returns over deep nesting.
- Table-driven tests; mock at interface boundaries.

```go
// good: wrapped error, context first, scoped query in the repo it belongs to
func (s *SiteService) Get(ctx context.Context, acct, id uuid.UUID) (*model.Site, error) {
    site, err := s.repo.Get(ctx, acct, id)
    if err != nil {
        return nil, fmt.Errorf("get site %s: %w", id, err)
    }
    return site, nil
}
```

## 4. TypeScript / React standards

- **`strict: true`. No `any`** — use `unknown` + narrowing or a real type.
- **Server Components by default**; `"use client"` only for interactivity, at the leaves.
- **No business logic in components** — push it into `lib/` or the backend.
- Prefer `type`/`interface` for reused shapes; don't redeclare API types inline.
- Dashboard/admin use Tailwind + `cn()`; marketing may use Astryx/StyleX. Never
  mix component systems within a route group (ADR 0007).
- One Lucide icon set, imported per-icon.
- Lint with **oxlint**; type-check with `tsc --noEmit`.

## 5. Comments & documentation

- Document the **why** and any non-obvious constraint, invariant, or gotcha.
- Exported Go identifiers get doc comments starting with the name.
- Mark deliberate shortcuts with their ceiling and upgrade path, e.g.
  `// naive O(n) scan; index node lookups if the list grows`.
- Keep docs in sync with code — stale docs are worse than none.

## 6. Error handling (cross-cutting)

- **Backend:** errors flow up wrapped; the handler layer is the only place that
  maps an error to an HTTP status + envelope. Never leak internals to clients.
- **Frontend:** normalize backend error envelopes into a typed `ApiError`; use
  `error.tsx` boundaries; show actionable messages.
- Never swallow an error to silence it. If you intentionally ignore one, say why.

## 7. Logging

- Structured (Zap) on the backend; carry `request_id`. Never log secrets.
- No `console.log` left in committed frontend code; no `fmt.Println` in Go.
- Levels used as defined in [`BACKEND.md`](BACKEND.md#11-logging-zap).

## 8. Formatting & tooling

| Language | Format | Lint | Type-check |
|---|---|---|---|
| Go | `gofmt` / `goimports` | `golangci-lint` | compiler |
| TypeScript | editor/Prettier-compatible | `oxlint` | `tsc --noEmit` |
| SQL | consistent, lowercase keywords optional but uniform | — | migration review |

Run formatters/linters before committing; CI enforces them.

## 9. Commits & branches

- **Conventional Commits:** `feat:`, `fix:`, `chore:`, `refactor:`, `docs:`,
  `test:`, `perf:`, scoped where useful (`feat(provisioner): …`). Imperative mood.
- Short-lived branches off `main`: `feat/…`, `fix/…`, `chore/…`, `docs/…`.
- Small, focused commits and PRs. Full workflow: [`CONTRIBUTING.md`](CONTRIBUTING.md).

## 10. Things to avoid

The repo-wide list lives in [`../CLAUDE.md`](../CLAUDE.md#6-things-to-never-do).
Highlights: business logic in handlers/components; unscoped customer queries; SQL
string concatenation; `any`/`interface{}` escape hatches; floats for money;
editing shipped migrations; new dependencies for what a few lines solve.

# Contributing

How to work on OpenCloud. This applies to every contributor — human or AI. Read
[`../CLAUDE.md`](../CLAUDE.md) and [`CODING_STANDARDS.md`](CODING_STANDARDS.md)
first; this document is the workflow around them.

---

## 1. Before you start

- Pick up work from [`../ROADMAP.md`](../ROADMAP.md) or an issue.
- For anything non-trivial, agree on the approach first (issue discussion or a
  short design note / ADR) before writing code.
- Make sure you can run the stack locally — see [`../README.md`](../README.md#quick-start).

## 2. Local setup

```bash
cp .env.example .env          # fill in dev values
docker compose up --build     # full stack
# or run pieces directly:
(cd backend && go run ./cmd/api)
npm install && npm run dev    # frontend (repo root)
```

## 3. Branching

- Branch off `main`; keep branches **short-lived**.
- Naming: `feat/<slug>`, `fix/<slug>`, `chore/<slug>`, `refactor/<slug>`,
  `docs/<slug>`, `test/<slug>`.
- `main` is always deployable. **Never commit directly to `main`.**
- Don't commit or push unless the work is ready and (for AI) the human has asked.

## 4. Commits

- **Conventional Commits**, imperative mood, scoped where useful:
  ```
  feat(provisioner): add idempotent CreateDatabase
  fix(auth): rotate refresh token on reuse
  docs(api): document idempotency-key header
  ```
- Small, focused commits that each leave the tree working.
- **Never commit** secrets, `.env`, `node_modules`, build outputs, or large binaries.

## 5. Making the change

- Follow the layering, naming, and structure in the docs. Match surrounding code.
- **Smallest correct change.** No unrequested abstractions, files, or dependencies
  ([`../CLAUDE.md`](../CLAUDE.md)).
- Validate input at boundaries; scope every customer query by `account_id`.
- Add/adjust tests for non-trivial logic ([`TESTING.md`](TESTING.md)).
- Update docs in the **same PR** when you change architecture, conventions, API, or
  config — and add a [`../CHANGELOG.md`](../CHANGELOG.md) entry under **[Unreleased]**.

## 6. Before opening a PR

Run the checks CI will run:

```bash
# backend (from repo root)
(cd backend && test -z "$(gofmt -l .)")
(cd backend && golangci-lint run)
(cd backend && go test ./...)

# frontend
npm run lint
npx tsc --noEmit
npm run build
```

## 7. Pull requests

- **One concern per PR.** Smaller PRs review faster and break less.
- Description covers **what** changed and **why**, how it was tested, and any
  migration/rollout notes.
- Link the issue/roadmap item. Call out schema changes, breaking API changes, and
  anything touching security explicitly.
- **CI must be green** — no green, no merge.

### PR checklist
- [ ] Follows coding standards and layering
- [ ] Tests added/updated; suite passes
- [ ] Customer queries scoped by `account_id`; input validated at boundaries
- [ ] No secrets, `.env`, or generated artifacts committed
- [ ] Docs + `CHANGELOG.md` updated if behavior/architecture changed
- [ ] Migrations are forward-only and backward-compatible ([`DEPLOYMENT.md`](DEPLOYMENT.md#4-database-migrations))

## 8. Code review

- At least one reviewer approves before merge (AI-authored PRs included).
- Reviewers check correctness, security (especially tenant scoping), tests, and
  whether the change is the **simplest** thing that works.
- Be specific and kind; review the code, not the author. Prefer suggestions over
  demands; block only on real problems.
- **Squash-merge** to keep history linear and each `main` commit meaningful.

## 9. Architecture decisions

- A change that alters architecture, a cross-cutting convention, or the tech stack
  needs an **ADR** in [`adr/`](adr/) (copy `adr/0000-template.md`).
- ADRs are immutable once accepted; supersede rather than edit.

## 10. Reporting bugs & security issues

- **Bugs:** open an issue with repro steps, expected vs actual, and environment.
  Fixes ship with a regression test.
- **Security:** do **not** open a public issue. Report privately to the maintainers
  per [`SECURITY.md`](SECURITY.md); credentials and details stay out of public
  trackers.

## 11. For AI contributors (Claude)

The full ruleset is in [`../CLAUDE.md`](../CLAUDE.md). In short: read before
writing, stay within the layering, make the smallest correct change, never weaken
security or tenant scoping, test non-trivial logic, explain briefly, and surface
conflicts instead of papering over them.

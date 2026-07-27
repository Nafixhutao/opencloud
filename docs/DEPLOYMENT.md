# Deployment

How OpenCloud is built, released, and rolled back. Packaging and environments are
in [`INFRASTRUCTURE.md`](INFRASTRUCTURE.md); this covers the release process.

**Stack:** Docker · Docker Compose · CI (build/lint/test) · Bun + Better Auth migrations.

---

## 1. Artifacts

| Artifact | Built from | Contents |
|---|---|---|
| `opencloud-backend` image | `backend/Dockerfile` (multi-stage) | `api`, `worker`, and `migrate` binaries |
| `opencloud-control-plane-backup` image | `backend/Dockerfile` (`backup` target) | PostgreSQL 18 client tools + static encrypted backup/restore orchestrator |
| `opencloud-frontend` image | root `Dockerfile` (`runner` target) | Next.js standalone server |
| `opencloud-auth-migrate` image | root `Dockerfile` (`auth-migrate` target) | Better Auth migration runtime |

Images are immutable and tagged by version + git SHA (e.g.
`opencloud-backend:1.4.0-ab12cd3`). The same image promotes through staging → prod.
The backend Dockerfile uses BuildKit cache mounts, so build hosts must provide a
working `docker buildx` plugin; on Ubuntu 24.04 the package is `docker-buildx`.

## 2. CI pipeline

Runs on every PR and on merge to `main`. **Merges are blocked unless CI is green.**

```
1. Backend:  gofmt check · golangci-lint · vet · unit/real-Postgres tests · migration round trip · govulncheck · docker build
2. Frontend: Better Auth migration · auth/UI tests · oxlint · tsc --noEmit · npm run build · npm audit
3. Image publishing and automatic deployment are added with the release pipeline.
4. Backup gate: authenticated encryption unit tests + a real disposable
   scheduled `pg_dump`/verify/restore rehearsal against two PostgreSQL instances.
```

See [`TESTING.md`](TESTING.md) for the test layers and [`CONTRIBUTING.md`](CONTRIBUTING.md)
for the PR workflow.

## 3. Environments & promotion

```
feature branch → PR (CI) → merge to main → deploy staging → verify → promote to production
```

- **development** — local Docker Compose; fake provisioner.
- **staging** — mirrors production config with disposable labeled Docker/Caddy
  resources; auto-deployed from `main`.
- **production** — promoted from a verified staging build; manual approval gate.

Config differs only by environment variables ([`INFRASTRUCTURE.md`](INFRASTRUCTURE.md#3-configuration--environment-variables)).

## 4. Database migrations

- Migrations run as an explicit deploy step, **before** the new app version starts:
  ```bash
  (cd backend && go run ./cmd/migrate up)
  npm run auth:migrate
  ```
- Bun runs first to create the `auth` namespace. Better Auth's migration API then manages
  only the identity tables inside that schema (ADR 0006).
- Production is **forward-only**; never edit a shipped migration — add a new one.
  SQL checksums enforce immutable history, and `up` creates one rollback group
  per file. `down` is a disposable-development verification tool, not a
  production rollback procedure.
- Migrations must be **backward-compatible** with the currently-running app version
  so a deploy (or rollback) never breaks live traffic. The pattern for breaking
  schema changes is **expand → migrate → contract**:
  1. *Expand:* add new columns/tables; deploy app that writes both old + new.
  2. *Migrate:* backfill data.
  3. *Contract:* deploy app that uses only new; later migration drops the old.
- A failed migration aborts the deploy; the previous version keeps serving.

## 5. Deploy procedure (Compose)

```bash
git pull                              # or check out the release tag
docker compose build
docker compose run --rm migrate       # explicit, fail-fast deploy gate (Bun)
docker compose run --rm auth-migrate  # Better Auth identity tables (ADR 0006)
docker compose up -d api worker dashboard
docker compose ps                     # verify health
curl -fsS localhost:8080/readyz       # readiness gate
```

- `api` and `worker` are stateless → replaceable without data loss.
- Roll services one at a time behind the proxy for zero-downtime where the
  orchestrator supports it.

## 6. Zero-downtime considerations

- **Stateless services** (API, worker, frontend) scale horizontally and roll
  without draining state — sessions live in Redis, the job queue in PostgreSQL.
- **Readiness gating:** new instances take traffic only after `/readyz` passes.
- **Graceful shutdown:** on `SIGTERM` the API drains in-flight HTTP and the worker
  finishes its current job before exiting ([`BACKEND.md`](BACKEND.md#3-entry-points)).
- **Backward-compatible migrations** (above) keep old and new app versions working
  against the same schema during a rollout.

## 7. Rollback

- **App:** redeploy the previous image tag. Because services are stateless and
  migrations are backward-compatible, this is safe and fast.
- **Schema:** prefer rolling *forward* with a fix. Only run a `down` migration if it
  is known-safe for current data; destructive `down`s are avoided in production.
- **Data:** restore from PostgreSQL backups only as a last resort, following the
  rehearsed restore runbook ([`DATABASE.md`](DATABASE.md#9-backups)).

## 8. Hosting backend rollout

- The Phase 0 Docker/Caddy spike is disposable and run explicitly from
  `deploy/spikes/docker-caddy`; it is not part of every app deployment.
- Phase 2 worker rollout grants hosting access only after ownership-label,
  idempotency, resource-limit, and backup/restore checks pass in staging.
- Docker daemon and Caddy admin access are never added to dashboard/API
  containers. Scale-out or fallback Hestia nodes are drained before maintenance
  ([`HOSTING.md`](HOSTING.md)).
- The base Compose file intentionally defaults the worker to `fake` and does not
  mount `/var/run/docker.sock`. Enabling the Docker adapter requires a separately
  reviewed worker deployment with a restricted daemon boundary and private Caddy
  admin reachability. A raw socket grants host-equivalent control and is only
  used by the disposable integration harness, never by the public API/frontend.

## Phase 2 review-branch validation

The site-provisioning slice is validated with isolated PostgreSQL, Caddy, and
site resources on a disposable target. `deploy/validation/caddy-phase2.json`
binds only validation loopback ports and must not replace the host's active Caddy
configuration. The tagged Docker/Caddy integration test uses fixed deterministic
resource names so cleanup can target exactly those objects. Passing this
validation does not authorize staging promotion or production deployment.

## 9. Secrets & config at deploy time

- Production secrets come from the orchestrator's secret store, injected as env
  vars — never baked into images or committed.
- Changing a secret is a deliberate, documented operation. The JWT signing key
  lives in **better-auth** (BFF), not the Go API (ADR 0006): rotating
  `BETTER_AUTH_SECRET` invalidates active sessions/tokens (clients re-authenticate),
  and the Go API picks up the new signing key automatically from the JWKS at
  `AUTH_JWKS_URL` — no API secret to rotate for auth.

## 10. Post-deploy verification

After every production deploy:
1. `/healthz` and `/readyz` green on all instances.
2. Error rate and latency steady in Grafana ([`INFRASTRUCTURE.md`](INFRASTRUCTURE.md#5-monitoring-prometheus--grafana)).
3. A smoke test of a critical flow (login → list sites).
4. Queue depth draining normally; no failed-job spike.

If any check fails, roll back (§7) and investigate before re-attempting.

## 11. Release tagging & changelog

- Tag releases with SemVer (`vMAJOR.MINOR.PATCH`).
- Move `CHANGELOG.md`'s **[Unreleased]** section under the new version + date on
  release ([`../CHANGELOG.md`](../CHANGELOG.md)).

## 12. Control-plane backup rollout

The backup service is opt-in and must not be enabled merely because its image
exists:

1. Generate a 32-byte key in the deployment secret manager. Never place the
   production value in `.env`, shell history, an image, or the repository.
2. Mount an encrypted off-host destination (or an access-controlled local spool
   with immediate off-host replication) at `/backups`.
3. Start `docker compose --profile backup up -d control-plane-backup`.
4. Alert on container restarts/exits and confirm a fresh `.dump.ocb` plus
   `.sha256` pair arrives on schedule.
5. Run `verify`, copy the artifact to an isolated environment, and perform the
   full restore rehearsal in
   [`runbooks/control-plane-backup-restore.md`](runbooks/control-plane-backup-restore.md).

The named Compose volume is suitable for local/staging rehearsal only. It is
co-located with the control plane and therefore does not satisfy production
disaster-recovery requirements by itself. No production restore is automated.

## Phase 1 deploy notes

After Bun migrations, run Better Auth migration, then rolling-update API/worker/dashboard.
Promote the first admin with `/app/bootstrap-admin --user-id <id>` (not via HTTP).
Production must set `MAIL_PROVIDER=smtp`, `MAIL_FROM`, `SMTP_HOST`,
`SMTP_PORT`, `SMTP_SECURE`, `SMTP_USER`, and `SMTP_PASS`. Port 465 normally uses
`SMTP_SECURE=true`; STARTTLS ports use `false` and are upgraded with certificate
validation. The dashboard fails fast if production configuration is incomplete.
`log`/`memory` are development/test adapters and do not deliver email. Provider
credentials are external secrets; do not deploy or claim production email active
until staging verification/reset delivery succeeds.

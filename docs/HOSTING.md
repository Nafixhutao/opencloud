# Hosting stack — Docker, Caddy, and provider fallbacks

OpenCloud owns the customer experience and hosting control plane. For the MVP,
the data plane runs customer sites as Docker containers and routes domains
through Caddy (ADR 0008). Hestia is preserved as a fallback adapter rather than
installed on the live control-plane host.

## 1. Components

| Component | Responsibility |
|---|---|
| **Docker Engine** | Site containers, private networks, persistent volumes, runtime resource policy |
| **Caddy** | Public ingress, reverse proxy, automatic HTTPS, certificate renewal |
| **OpenCloud worker** | Sole caller of hosting backends; executes retryable jobs |
| **PostgreSQL** | System of record for sites, domains, desired state, and jobs |
| **Cloudflare** | Authoritative DNS and optional tunnel/edge path (ADR 0003) |

The Phase 0 spike is disposable and proves the host can create one constrained
container, route a real hostname through Caddy, repeat the operation without
duplicates, and clean up only OpenCloud-labeled resources. The Phase 2
site-provisioning review slice implements the durable queue, customer APIs,
worker, and Docker/Caddy adapter. A stacked Phase 2 branch adds encrypted
scheduled control-plane PostgreSQL backup plus a disposable restore rehearsal.
Neither branch is a production deployment. A further stacked branch implements
customer PostgreSQL/MariaDB lifecycle; site volume/customer-database backups
remain outstanding.

## 2. Provisioner boundary

Only the provisioner talks to Docker, Caddy, Cloudflare, or a fallback Hestia
node. Handlers and services depend on capabilities, never provider payloads.

```go
type SiteProvisioner interface {
    CreateSite(ctx context.Context, spec SiteSpec) error
    DeleteSite(ctx context.Context, ref SiteRef) error
    SuspendSite(ctx context.Context, ref SiteRef) error
    ResumeSite(ctx context.Context, ref SiteRef) error
    SiteStatus(ctx context.Context, ref SiteRef) (SiteState, error)
}
```

The backend is selected with `PROVISIONER_BACKEND=docker|hestia|fake`:

- `docker` is the MVP default.
- `hestia` is the documented fallback and requires a dedicated clean node.
- `fake` is permitted only in development/tests and is rejected in production.

Every operation is idempotent. An already-correct resource is success; deleting
an absent managed resource is success; a conflicting unmanaged resource is an
error requiring operator review.

## 3. Docker object ownership

OpenCloud creates deterministic objects using the site UUID and labels every
object with at least:

- `opencloud.managed=true`
- `opencloud.account_id=<uuid>`
- `opencloud.site_id=<uuid>`
- `opencloud.node_id=<uuid>`

The provisioner may mutate or remove an object only after all ownership labels
match the requested tenant and site. It never uses global prune commands.

Each site receives a dedicated network and persistent volume. Launch templates
run non-root with a read-only root filesystem where possible, dropped Linux
capabilities, `no-new-privileges`, PID/CPU/memory limits, and no arbitrary host
mounts. Arbitrary user Dockerfiles remain out of launch scope until isolated
build workers, image scanning, and policy enforcement exist.

## 4. Site lifecycle

All customer-facing writes are asynchronous through the PostgreSQL `jobs` table:

```text
create:
  desired row + job -> ensure network/volume -> ensure container -> ensure Caddy route
  -> health check -> active

suspend:
  remove/disable public route -> stop container -> keep volume -> suspended

resume:
  start container -> health check -> restore route -> active

delete:
  remove route -> remove matching container/network -> retain or delete volume by policy
  -> delete database credentials -> deleted
```

If a worker loses the response after a successful operation, its retry inspects
the deterministic object and converges instead of creating a duplicate.

## 5. Domains and HTTPS

Caddy remains the only public listener on ports 80/443. Site containers publish
no public ports; Caddy reaches an internal/loopback upstream.

For customer-controlled domains, Caddy's On-Demand TLS permission endpoint must
query an indexed OpenCloud domain record and return success only for an active,
verified domain. This prevents arbitrary certificate issuance. Caddy config
updates are serialized; each update is validated before reload and must preserve
unrelated platform routes.

Cloudflare remains authoritative DNS under ADR 0003. Customers bring a domain,
complete ownership/DNS verification, and then OpenCloud authorizes Caddy to serve
it.

## 6. Databases and persistent files

The Phase 2 database adapter targets dedicated managed PostgreSQL and MariaDB
services through worker-only admin connections. It creates deterministic
physical databases and least-privileged logins; the control-plane PostgreSQL is
never a customer target. Retried create rotates the login password while
preserving the existing physical database, and repeated delete converges.

Credentials are generated in worker memory, encrypted with a separate external
AES-256-GCM key before control-plane persistence, and shown only through an
explicit audited one-time route. Job payloads, logs, audits, and normal
list/detail responses never carry a password or target admin DSN. Production
activation requires private admin reachability, customer-facing TLS endpoints,
external key custody, and successfully rehearsed lifecycle tests against both
engines.

Site files live in named volumes. Backups must include the volume plus a
consistent database dump; a backup is not considered operational until a restore
has been rehearsed.

## 7. Security boundary

Docker daemon access is root-equivalent. It is never mounted into the dashboard
or public API. The production worker reaches it through a hardened local boundary
(rootless engine or restricted socket proxy/authorization policy chosen during
Phase 2 hardening), and only typed operations are exposed to job handlers.

Customer input never becomes a shell command, container name, label selector,
mount path, or raw Caddy JSON. Domains, image/template IDs, resource limits, and
environment keys are validated before provisioning.

## 8. Hestia fallback

ADR 0001 is retained as historical context. Hestia becomes active only through a
new/superseding decision after its access-key scope, idempotency, backup, and
migration checks pass on a dedicated non-production node. The full triggers and
migration runbook are in [`HESTIA_FALLBACK.md`](HESTIA_FALLBACK.md).

## 9. Failure and reconciliation

- Failed jobs retry with backoff; terminal failures enqueue compensating cleanup.
- A reconciliation job compares PostgreSQL desired state with labeled Docker
  objects and Caddy routes.
- Unknown/unmanaged resources are reported, never adopted or deleted silently.
- Metrics cover provisioning latency, retries, failures, container health, and
  Caddy route/certificate errors without per-user high-cardinality labels.

The Docker adapter uses one deterministic Caddy route ID per site and conditional
`If-Match` writes against Caddy's config API so concurrent changes retry instead
of overwriting another route. It validates the complete existing ownership
label set before stopping or deleting Docker objects. The launch slice permits
only the exact configured static image and binds the random upstream port to
host loopback.

The direct Docker socket remains unsuitable as a production boundary. A
rootless engine or restricted authorization/socket proxy and private Caddy admin
path must be selected and reviewed before enabling this adapter outside isolated
validation.

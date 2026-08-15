# Hosting stack — Docker and Caddy

OpenCloud owns the customer experience and hosting control plane. For the MVP,
the data plane runs customer sites as Docker containers and routes domains
through Caddy (ADR 0008).

## 1. Components

| Component | Responsibility |
|---|---|
| **Docker Engine** | Site containers, private networks, persistent volumes, runtime resource policy |
| **Caddy** | Public ingress, reverse proxy, automatic HTTPS, certificate renewal |
| **OpenCloud worker** | Sole caller of hosting backends; executes retryable jobs |
| **PostgreSQL** | System of record for sites, domains, desired state, and jobs |
| **Customer DNS provider** | Manual TXT ownership proof and A record to direct ingress (ADR 0009) |
| **Cloudflare** | Optional future tenant-authorized DNS/tunnel adapter; unavailable today |

The Phase 0 spike is disposable and proves the host can create one constrained
container, route a real hostname through Caddy, repeat the operation without
duplicates, and clean up only OpenCloud-labeled resources. The Phase 2
site-provisioning review slice implements the durable queue, customer APIs,
worker, and Docker/Caddy adapter. A stacked Phase 2 branch adds encrypted
scheduled control-plane PostgreSQL backup plus a disposable restore rehearsal.
Neither branch is a production deployment. A further stacked branch implements
customer PostgreSQL/MariaDB lifecycle; site volume/customer-database backups
remain outstanding.
The Phase 3 customer-domain lifecycle is code-complete and exercised in
disposable infrastructure, but it is not production-deployed.

## 2. Provisioner boundary

Only the provisioner talks to Docker, Caddy, or a future Cloudflare adapter. Handlers and services depend on capabilities, never provider payloads.

```go
type SiteProvisioner interface {
    CreateSite(ctx context.Context, spec SiteSpec) error
    DeleteSite(ctx context.Context, ref SiteRef) error
    SuspendSite(ctx context.Context, ref SiteRef) error
    ResumeSite(ctx context.Context, ref SiteRef) error
    SiteStatus(ctx context.Context, ref SiteRef) (SiteState, error)
}
```

The backend is selected with `PROVISIONER_BACKEND=docker|fake`:

- `docker` is the MVP default.
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

Customers can use any authoritative DNS provider. OpenCloud returns a TXT record
at `_opencloud-verification.<hostname>` containing an expiring ownership token;
after verification it returns an A record to `DOMAIN_INGRESS_IPV4`. The API
observes both through `DOMAIN_DNS_RESOLVER`. Automated zone writes and Cloudflare
Tunnel are optional future adapters, not the implemented default (ADR 0009).
Primary site hostnames are separate: production accepts only strict children of
the platform-owned `SITE_DOMAIN_SUFFIX`. A customer-owned hostname can never be
created as an unverified primary route; it must pass the custom-domain proof.

Caddy's On-Demand TLS permission endpoint queries an indexed OpenCloud record
and returns empty `200` only for an active primary/custom hostname. Missing or
inactive names return `403`; database errors return `503` with `Retry-After`.
Because every non-`2xx` denies authorization, outages fail new issuance closed.
The ask endpoint is served beside metrics on an internal-only listener and is
never publicly routed. On-demand issuance is also enabled explicitly on the
eligible TLS policy; configuring the ask endpoint alone is not enough.

The worker serializes validated Caddy changes, uses exact-host routes, preserves
primary plus alias hosts, and refuses unowned route IDs. Production Caddy uses
public ACME. [`../deploy/caddy/caddy.json`](../deploy/caddy/caddy.json) assumes a
co-located host where Caddy admin and the API permission listener use loopback;
a separated topology must provide equivalent private authenticated transport.
The disposable proof uses a local CA and does not claim public issuance.

Certificate observation dials the configured ingress IPv4 with the customer
hostname as SNI, so it measures the actual served endpoint rather than trusting
customer DNS alone. The worker records observed/expiry state, schedules renewal
observation, and avoids audit/status churn when nothing changed. Operations must
alert on certificate errors, approaching expiry, permission failures, and ACME
rate limits.

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

## 9. Failure and reconciliation

- Failed jobs retry with backoff; terminal failures enqueue compensating cleanup.
- A reconciliation job compares PostgreSQL desired state with labeled Docker
  objects and Caddy routes.
- Unknown/unmanaged resources are reported, never adopted or deleted silently.
- Metrics cover provisioning latency, retries, failures, container health, and
  Caddy route/certificate errors without per-user high-cardinality labels.
- Domain reconciliation repairs desired exact routes; deprovision and site
  deletion retain global hostname ownership until durable cleanup succeeds.

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

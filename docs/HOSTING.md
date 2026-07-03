# Hosting Stack — Hestia & the Data Plane

OpenCloud orchestrates a proven hosting stack rather than building one. The
**data plane** is one or more Linux nodes running **Hestia Control Panel**, which
manages Nginx, Apache, PHP-FPM, MariaDB, BIND9, and Certbot. The Go control plane
drives it exclusively through the **provisioner**.

See [ADR 0001](adr/0001-hestia-as-provisioning-backend.md) for the "orchestrate,
don't fork" decision, and [`BACKEND.md`](BACKEND.md#8-provisioner) for the
provisioner package.

---

## 1. Components

| Component | Role on a node |
|---|---|
| **Hestia Control Panel** | Manages all of the below via its API/CLI; owns per-customer Linux users, quotas, and isolation. |
| **Nginx** | Front-end reverse proxy + static file serving; terminates HTTP(S). |
| **Apache HTTP Server** | Application server behind Nginx for PHP apps (`.htaccess` compatibility). |
| **PHP-FPM** | Runs PHP per site, with per-customer pools for isolation. |
| **MariaDB** | Customer databases and DB users. |
| **BIND9** | Installed with Hestia; **fallback only** — production DNS is Cloudflare ([ADR 0003](adr/0003-cloudflare-dns-and-ingress.md)). |
| **Certbot** | Let's Encrypt certificate issuance and renewal. |

> Nginx → Apache → PHP-FPM is the standard layered setup: Nginx handles TLS,
> static assets, and proxying; Apache + PHP-FPM run the dynamic PHP application.

## 2. How OpenCloud talks to Hestia

- The **provisioner** is the only component that touches a node. It uses Hestia's
  API (and CLI where the API is thin) over an authenticated, TLS channel. It is
  also the sole caller of the **Cloudflare API** for customer DNS zones/records
  ([ADR 0003](adr/0003-cloudflare-dns-and-ingress.md)).
- Every provisioner operation is **idempotent**: creating a site/DB/zone that
  already exists succeeds rather than errors. This makes job retries and drift
  reconciliation safe.
- No raw shell string interpolation — commands are built from typed, validated
  arguments. Customer-supplied values (domains, DB names) are validated before
  they reach a node. See [`SECURITY.md`](SECURITY.md).
- Credentials for node access come from config (Viper) and are never logged.

```go
type Provisioner interface {
    CreateSite(ctx context.Context, node model.Node, spec SiteSpec) error
    DeleteSite(ctx context.Context, node model.Node, domain string) error
    CreateDatabase(ctx context.Context, node model.Node, spec DBSpec) error
    CreateDNSZone(ctx context.Context, zone string) error // Cloudflare API, not a node (ADR 0003)
    IssueCertificate(ctx context.Context, node model.Node, domain string) error
}
```

## 3. Node topology

```
                 OpenCloud control plane (Go API + worker)
                                │  provisioner (Hestia API/CLI, TLS)
        ┌───────────────────────┼───────────────────────┐
        ▼                       ▼                        ▼
   Hestia node 1           Hestia node 2            Hestia node N
   ─────────────          ─────────────            ─────────────
   Nginx / Apache         Nginx / Apache           Nginx / Apache
   PHP-FPM pools          PHP-FPM pools            PHP-FPM pools
   MariaDB                MariaDB                  MariaDB
   BIND9                  BIND9                    BIND9
   Certbot                Certbot                  Certbot
   Fail2ban + UFW         Fail2ban + UFW           Fail2ban + UFW
```

- Each node is registered in the `nodes` table with capacity/usage.
- New accounts are placed on the least-loaded online node (simple scheduler; see
  [`../ROADMAP.md`](../ROADMAP.md) Phase 2/7).
- Nodes can be set to `draining` to stop new placements before maintenance.

## 4. Provisioning flows

All flows are **asynchronous** (queued via the `jobs` table, run by the worker) and update the
resource's `status`. The control plane reconciles its state with the node's.

### Create a site
```
1. service inserts sites row (status=provisioning) + enqueues provision_site
2. worker → provisioner.CreateSite:
     - create Hestia web domain (Nginx vhost + Apache + PHP-FPM pool)
     - set document root, PHP version
3. success → status=active ; failure → retry → status=failed + cleanup
```

### Add a domain + SSL
```
1. attach domain to site (DNS zone via the Cloudflare API — ADR 0003)
2. enqueue issue_certificate
3. provisioner.IssueCertificate (Certbot) → store cert metadata, status=active
4. renewal handled on the node by Certbot; control plane tracks expiry
```

### Provision a database
```
1. service inserts databases row (status=provisioning)
2. provisioner.CreateDatabase: MariaDB database + scoped DB user
3. credentials returned to the customer once, not stored in plaintext
```

### Suspend / delete
```
suspend: provisioner disables the web domain (503), keeps data → status=suspended
delete:  provisioner removes site/DB/zone, then control plane hard-deletes rows
```

## 5. Isolation on the node

Hestia provides OS-level multi-tenancy that complements application/DB scoping:

- **Separate Linux user per customer**, with home-directory permissions.
- **Per-customer PHP-FPM pools** so one site can't read another's processes/files.
- **Per-customer MariaDB users** scoped to their own databases.
- Resource quotas (disk, optionally CPU/memory) enforced per account.

This is the third isolation layer described in
[`../ARCHITECTURE.md`](../ARCHITECTURE.md#8-multi-tenancy--isolation).

## 6. SSL / certificates

- Issuance and renewal use **Certbot** (Let's Encrypt) on the node.
- The control plane records certificate domain, issuer, and expiry; renewal alerts
  fire before expiry (monitoring — [`INFRASTRUCTURE.md`](INFRASTRUCTURE.md)).
- HTTPS is enforced (HSTS) for customer sites and the dashboard.

## 7. DNS & ingress (Cloudflare — ADR 0003)

- Customer zones live in the platform's Cloudflare account. Customers **bring
  their own domain** and point it at the Cloudflare-assigned nameservers.
- Record changes go through the API (`/dns/zones/{id}/records`) → provisioner →
  **Cloudflare API**. Propagation status is surfaced in the UI.
- Inbound traffic (dashboard, API, customer sites) enters through **Cloudflare
  Tunnel** (`cloudflared`, outbound-only) — no static IP or open inbound ports
  required on our hardware. The tunnel carries HTTP(S) only: customer file
  access starts with the web file manager, not raw FTP/SFTP.
- BIND9 on the nodes is the documented **fallback** (self-hosted Hestia DNS
  cluster, `v-add-remote-dns-host`) if Cloudflare must ever be dropped — see
  [ADR 0003](adr/0003-cloudflare-dns-and-ingress.md).

## 8. Backups

- Hestia handles per-account backups on the node (web files, databases, mail).
- Backup schedules and retention are configured per node and documented in a
  runbook. Restores are rehearsed.
- Control-plane data (PostgreSQL) is backed up separately — see
  [`DATABASE.md`](DATABASE.md#9-backups).

## 9. Node hardening

Every node runs **Fail2ban** and **UFW** with only required ports open, plus the
standard hardening in [`SECURITY.md`](SECURITY.md). Node bootstrap/hardening is
scripted under `deploy/hestia/` so a new node is reproducible.

## 10. Failure & reconciliation

- Because the provisioner is idempotent, failed jobs retry safely.
- A periodic **reconciliation job** compares control-plane state to each node's
  actual state and repairs drift (e.g. a site marked active but missing on the
  node), or flags it for an operator.
- Partial provisioning is cleaned up by compensating actions — no orphaned
  resources on a node or stale `active` rows in the DB.

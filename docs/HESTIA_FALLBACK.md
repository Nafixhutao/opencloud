# Hestia fallback plan

Hestia is no longer the primary MVP backend (ADR 0008), but it remains a
supported architectural fallback. This document preserves the operational
knowledge needed to add a Hestia adapter without coupling services or public APIs
to Docker.

## Why it is deferred

- Hestia requires a fresh supported OS and manages Nginx, Apache, PHP-FPM,
  MariaDB, firewall rules, and ports used by the live OpenCloud host.
- The current MVP needs containerized HTTP applications and custom domains, not
  mail, FTP, or a cPanel-compatible filesystem.
- Docker and Caddy are already deployed, so their lifecycle can be validated
  without reimaging production.

Never install Hestia natively beside the live OpenCloud control plane. A fallback
adoption starts with a separate clean VPS/VM running a supported Debian or Ubuntu
release. See Hestia's official [installation requirements](https://hestiacp.com/docs/introduction/getting-started).

## When to reconsider Hestia

Open a superseding ADR when one or more conditions are true:

- email hosting, webmail, FTP, or traditional shared-hosting workflows become a
  launch requirement;
- curated containers cannot provide the required PHP/user isolation;
- custom backup, quota, and lifecycle operations cost more to operate than a
  dedicated Hestia node;
- a security review requires per-customer Linux users instead of container-level
  isolation.

## Adapter contract

The Hestia implementation must satisfy the same provider-neutral
`SiteProvisioner` interface as Docker. Service and handler code must not contain
Hestia usernames, commands, response payloads, or credentials.

| OpenCloud concept | Docker/Caddy primary | Hestia fallback |
|---|---|---|
| Site identity | deterministic container labels | web domain owned by a Hestia user |
| Account isolation | network, volume, cgroup, runtime policy | Linux user + PHP-FPM pool |
| Files | named volume | `/home/<user>/web/<domain>` |
| Domain/HTTPS | Caddy route + managed certificate | Hestia web domain + Certbot |
| Database | managed DB/container + scoped credentials | MariaDB DB and user |
| Suspend/resume | stop/start container, keep volume | suspend/unsuspend web domain |

The future `nodes` model records a backend driver and provider-specific metadata;
customer-facing resource tables remain provider-neutral.

## Authentication and key scope

- Use Hestia access/secret keys, not password authentication or the deprecated
  unrestricted API key where avoidable.
- Create an allowlisted API profile containing only commands required by the
  adapter. Hestia documents scoped profiles and access keys in its official
  [REST API guide](https://hestiacp.com/docs/server-administration/rest-api).
- Restrict API source IPs, use TLS, rotate credentials, and never log secrets.
- Keep `HESTIA_API_URL`, `HESTIA_ACCESS_KEY`, and `HESTIA_SECRET_KEY` in the
  secret manager. `HESTIA_API_KEY` exists only for a legacy transition.

## Required validation before activation

1. Provision a dedicated non-production Hestia node.
2. Prove an allowed command succeeds and a command outside the profile fails.
3. Create the same user/domain/database twice through the adapter and confirm the
   second call converges without duplication.
4. Retry after an intentionally discarded/ambiguous response.
5. Suspend, resume, and delete twice; confirm missing resources are success.
6. Reconcile control-plane state against the node and record drift behavior.
7. Rehearse backup and restore before moving customer data.

## Migration from Docker/Caddy

Migration is site-by-site and reversible during a maintenance window:

1. Put the source site in maintenance mode and stop writes.
2. Copy the named volume into the Hestia user's document root.
3. Export and import the database; issue new scoped credentials.
4. Create the Hestia web domain and validate it through a temporary hostname.
5. Switch the Caddy/Cloudflare route, verify HTTPS and application health, then
   mark the site backend as Hestia.
6. Retain the stopped source container and volume through the rollback window.

No destructive cleanup occurs until the migrated site passes verification and
its backup is restorable.

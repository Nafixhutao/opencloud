# ADR 0009: Direct Caddy ingress and customer-managed DNS

- **Status:** Accepted
- **Date:** 2026-07-30
- **Deciders:** Core team
- **Supersedes:** ADR 0003's default ingress and DNS decision

## Context

Phase 3 needs a complete bring-your-own-domain path that works without placing
every customer zone in one platform-owned DNS account. A platform-wide
Cloudflare credential is not a safe substitute for per-tenant authorization,
and Tunnel is not required on a host that can expose Caddy on public ports
80/443 with a stable public address.

The control plane already stores desired site state, while the worker is the
only component permitted to change Docker or Caddy. Caddy can obtain public
certificates on demand, but its permission decision must be backed by an
indexed, verified OpenCloud domain record so arbitrary hostnames cannot consume
certificate issuance capacity.

## Decision

OpenCloud's default customer-domain path is **direct Caddy ingress with
customer-managed DNS**:

- every customer receives manual DNS instructions: a TXT ownership challenge at
  `_opencloud-verification.<hostname>` and, after verification, an A record to
  the configured public `DOMAIN_INGRESS_IPV4`;
- the API verifies ownership through a configurable public recursive resolver;
- the worker alone adds or removes exact-host Caddy routes after durable jobs
  establish the requested lifecycle state;
- Caddy uses On-Demand TLS only with an internal permission endpoint. A `2xx`
  response authorizes the hostname; missing, unknown, inactive, or database-error
  decisions deny issuance. The endpoint has no customer-auth path and is never
  exposed publicly;
- production Caddy uses public ACME and listens on public ports 80/443. Its admin
  API remains loopback/private and worker-only;
- Cloudflare Tunnel and DNS automation remain optional future adapters. The
  Cloudflare API feature flag fails closed until real per-tenant authorization
  is implemented.

ADR 0003 remains immutable historical context. This ADR supersedes only its
choice of Cloudflare as the default DNS and ingress path; its discussion of the
trade-offs of Cloudflare and CGNAT remains useful for an optional deployment.

## Consequences

**Easier:** customers can use any authoritative DNS provider, OpenCloud holds no
registrar or zone-wide credential for the manual path, and Caddy provides direct
certificate issuance and renewal behind an explicit database allowlist.

**Harder / accepted cost:** production requires a stable public IPv4 address,
inbound ports 80/443, working public DNS, public ACME reachability, rate-limit
monitoring, and secure private connectivity between Caddy, the worker, and the
API permission listener. CGNAT deployments need an explicitly selected tunnel
or other edge adapter. DNS changes are customer-driven until a tenant-authorized
provider integration is built.

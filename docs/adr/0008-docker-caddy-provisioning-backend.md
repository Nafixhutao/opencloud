# ADR 0008: Docker and Caddy as the primary provisioning backend

- **Status:** Accepted
- **Date:** 2026-07-21
- **Deciders:** Core team
- **Supersedes:** ADR 0001 for the primary MVP backend

## Context

OpenCloud's first production host already runs the control plane with Docker and
Caddy. This requires a fresh operating system and owns web-server, firewall,
database, and mail configuration, so installing it beside the live control plane
would create unsupported conflicts.

The product first needs containerized HTTP applications, customer domains,
automatic HTTPS, resource limits, and retry-safe lifecycle operations. Mail,
traditional FTP, and a cPanel-compatible host filesystem are not launch goals.

## Decision

OpenCloud will use **Docker Engine for isolated site workloads** and **Caddy for
ingress and certificate automation** as its primary MVP provisioning backend.
Services depend only on the provider-neutral `SiteProvisioner` capability; the
worker is the sole component allowed to reach the hosting backend.

## Consequences

**Easier:** the MVP reuses the deployed host, supports static and containerized
HTTP applications, and can add domains without installing another control panel.
Deterministic Docker labels/names and serialized Caddy changes make retries and
reconciliation implementable.

**Harder / accepted cost:** OpenCloud now owns workload policy, image trust,
resource quotas, backup/restore, and container lifecycle security. Docker daemon
access is root-equivalent and must never reach the dashboard or API; the Phase 2
worker uses a hardened local boundary. Containers are not VM isolation, so the
launch catalog uses curated non-privileged images before arbitrary Dockerfiles.

Email, webmail, and traditional FTP are deferred. If they become core product
requirements, or container isolation and operational cost prove inadequate,
the platform can reconsider control panel integration through a new ADR.

# ADR 0003: Cloudflare for DNS and ingress (Tunnel)

- **Status:** Accepted
- **Date:** 2026-07-02
- **Deciders:** Core team

## Context

Customer domains need authoritative DNS, and registrars require at least two
nameservers on distinct IPs. The original design made each node serve its
own zones — if a node dies, its customers' DNS dies with it; migrating
a customer changes their nameservers; and a single node can never satisfy the
two-nameserver rule.

The project is **self-hosted first**, and the initial deployment target is our
own hardware behind a residential connection: no static public IP (CGNAT), no
reverse-DNS control, and no second network for a redundant nameserver. A pure
self-hosted answer needs two small VPSes in separate
networks — paid external infrastructure either way.

## Decision

**Cloudflare (free tier) is the authoritative DNS and the ingress path.**

- Customer zones live in the platform's Cloudflare account, created and managed
  by the **provisioner via the Cloudflare API**. The provisioner remains the only
  component that talks to external infrastructure — nodes and Cloudflare.
- Customers **bring their own domain** and point it at the Cloudflare-assigned
  nameservers. Vanity `ns1/ns2.<platform>` needs a paid tier — skipped; this is
  branding, not function.
- Inbound traffic (dashboard, API, customer sites) enters through **Cloudflare
  Tunnel** (`cloudflared`): an outbound-only connection from our hardware, so no
  static IP, port forwarding, or exposed origin is required.
- BIND9 is **not** in the production DNS path.
  The documented fallback, if Cloudflare must ever be dropped, is a self-hosted
  cluster: two dedicated DNS nodes in separate networks synced with
  `v-add-remote-dns-host` — adopted by superseding this ADR.

## Consequences

**Easier:** zero-cost DNS and ingress; works behind CGNAT; DDoS protection, WAF,
and edge TLS for free; a node failure or customer migration never touches
nameservers; the origin IP stays hidden.

**Harder / accepted cost:** a deliberate exception to self-hosted-first — DNS and
all ingress depend on one third party (a Cloudflare outage or account suspension
takes every site offline). The Tunnel carries HTTP(S) only: no customer FTP/SFTP
through it (web file manager first) and no SMTP (email deferred — see
[ADR 0004](0004-external-services-at-launch.md)).

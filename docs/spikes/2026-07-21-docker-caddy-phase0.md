# Docker + Caddy Phase 0 spike result

- **Date:** 2026-07-21
- **Target:** existing OpenCloud Ubuntu 24.04 VPS
- **Result:** passed
- **Decision:** Docker + Caddy is viable as the MVP provisioning backend.

## Scope and safety boundary

The test used one disposable Go HTTP container, one dedicated bridge network,
one named volume, one loopback-only published port, and one temporary Caddy
route. Every Docker object carried both `opencloud.managed=true` and
`opencloud.spike=phase0-docker-caddy`. The cleanup path selected exact names and
labels; it did not use Docker prune or modify unrelated OpenCloud containers.

The site container ran as UID 65532 with a read-only root filesystem, all Linux
capabilities dropped, `no-new-privileges`, and CPU, memory, and PID limits.
Caddy was the only public ingress and obtained a valid certificate for a
temporary `sslip.io` hostname.

## Host preflight

| Check | Observed |
|---|---|
| Operating system | Ubuntu 24.04.4 LTS, KVM VPS |
| Capacity | 2 vCPU, 3.6 GiB RAM, 1.9 GiB swap, about 40 GiB free disk |
| Docker | 29.1.3 |
| Caddy | 2.11.4, active and enabled |
| Existing OpenCloud stack | dashboard, API, worker, PostgreSQL, and Redis healthy |

This capacity is enough for the spike and the current control plane. It is not
approval for an arbitrary number of customer workloads; Phase 2 must add
capacity accounting and admission limits.

## Evidence

1. The first experiment used a Docker `--internal` network. Docker accepted the
   loopback port binding but did not expose it to host-level Caddy, so the local
   health check failed. The script removed every object from that attempt.
2. The final configuration used a dedicated normal bridge network while keeping
   the published site port bound to `127.0.0.1`. The public boundary therefore
   remained Caddy.
3. The first `apply` passed with exactly one container, network, and volume;
   both the local HTTP check and public HTTPS health check returned 200.
4. A second `apply` returned the same resource counts and 200 responses, proving
   idempotent creation without duplicates.
5. After the site container was stopped manually, another `apply` restarted the
   same container and restored both health checks.
6. An external workstation request to the temporary HTTPS endpoint returned
   `{"service":"opencloud-phase0-spike","status":"ok"}`.
7. `destroy` succeeded twice. The second run was a no-op success, and the audit
   found no labeled container, network, volume, image, or temporary Caddy route.
8. The original Caddyfile was restored, validated, and reloaded. Caddy remained
   active, the production login route returned HTTP 200, and the existing
   Compose services remained healthy.

## Conclusion

The spike satisfies the Phase 0 requirement for repeatable create, retry,
recovery, HTTPS routing, delete, and safe cleanup on the intended VPS. It does
not grant the application worker unrestricted Docker access or constitute the
Phase 2 production adapter. That work still requires serialized Caddy updates,
least-privilege Docker access, quotas, reconciliation, audit events, and restore
tests as described in [`../HOSTING.md`](../HOSTING.md).

If those controls cannot meet the required isolation or shared-hosting product
scope, consider alternative control panel integration through a new ADR. Do not install uncontrolled panels over the live control-plane host.

## Reproduction

The disposable implementation and operator instructions live in
[`../../deploy/spikes/docker-caddy/`](../../deploy/spikes/docker-caddy/).

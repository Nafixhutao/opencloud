# Docker + Caddy Phase 0 spike

This disposable spike proves that the current OpenCloud host can create one
isolated site container, route a real hostname through Caddy with HTTPS, repeat
creation without duplicates, and remove every managed resource safely.

The script only manages objects labeled `opencloud.spike=phase0-docker-caddy`
and the dedicated Caddy snippet `opencloud-phase0-spike.caddy`. It never prunes
Docker or rewrites unrelated routes.

```bash
export SPIKE_SITE_HOST=phase0-spike.203-0-113-10.sslip.io
./deploy/spikes/docker-caddy/run.sh apply
./deploy/spikes/docker-caddy/run.sh apply   # idempotency check
./deploy/spikes/docker-caddy/run.sh verify
./deploy/spikes/docker-caddy/run.sh destroy # also removes the disposable image
```

Use a DNS name that resolves to the host. The script binds the test site only to
`127.0.0.1:18080`; Caddy remains the sole public entry point. It creates a
read-only, non-root container with dropped capabilities, no-new-privileges,
CPU/memory/PID limits, a dedicated bridge network, and a named volume. The
bridge is required because host-level Caddy cannot reach a port published from a
Docker `--internal` network; the port remains loopback-only.

This is evidence for the architecture decision, not the Phase 2 production
provisioner. The production implementation must use the provider-neutral Go
interface, serialize Caddy config changes, and give Docker access only to the
worker through a hardened local boundary.

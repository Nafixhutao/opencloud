#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
validation_id="${OPENCLOUD_VALIDATION_ID:-019f8d57}"
site_id="00000000-0000-4000-8000-00000000d257"
site_name="opencloud-site-${site_id}"
site_network="${site_name}-net"
site_volume="${site_name}-data"
caddy_name="opencloud-phase2-caddy-${validation_id}"
site_image="opencloud/site-static:phase2-validation-${validation_id}"
caddy_admin_url="http://127.0.0.1:22019"
caddy_public_url="http://127.0.0.1:22443"

case "$validation_id" in
  *[!a-zA-Z0-9_-]* | "") echo "invalid validation id" >&2; exit 2 ;;
esac

for container_name in "$site_name" "$caddy_name"; do
  if docker container inspect "$container_name" >/dev/null 2>&1; then
    echo "refusing to reuse existing container: $container_name" >&2
    exit 1
  fi
done
if docker network inspect "$site_network" >/dev/null 2>&1; then
  echo "refusing to reuse existing network: $site_network" >&2
  exit 1
fi
if docker volume inspect "$site_volume" >/dev/null 2>&1; then
  echo "refusing to reuse existing volume: $site_volume" >&2
  exit 1
fi
if docker image inspect "$site_image" >/dev/null 2>&1; then
  echo "refusing to reuse existing image: $site_image" >&2
  exit 1
fi
if ss -ltnH | awk '{print $4}' | grep -Eq '(^|:)(22019|22443)$'; then
  echo "validation port 22019 or 22443 is already in use" >&2
  exit 1
fi

remove_owned_site_resources() {
  if docker container inspect "$site_name" >/dev/null 2>&1; then
    if [ "$(docker inspect -f '{{index .Config.Labels "opencloud.managed"}}' "$site_name")" != "true" ] ||
      [ "$(docker inspect -f '{{index .Config.Labels "opencloud.site_id"}}' "$site_name")" != "$site_id" ]; then
      echo "refusing to remove container with mismatched ownership: $site_name" >&2
      return 1
    fi
    docker container rm -f "$site_name" >/dev/null
  fi
  if docker network inspect "$site_network" >/dev/null 2>&1; then
    if [ "$(docker network inspect -f '{{index .Labels "opencloud.managed"}}' "$site_network")" != "true" ] ||
      [ "$(docker network inspect -f '{{index .Labels "opencloud.site_id"}}' "$site_network")" != "$site_id" ]; then
      echo "refusing to remove network with mismatched ownership: $site_network" >&2
      return 1
    fi
    docker network rm "$site_network" >/dev/null
  fi
  if docker volume inspect "$site_volume" >/dev/null 2>&1; then
    if [ "$(docker volume inspect -f '{{index .Labels "opencloud.managed"}}' "$site_volume")" != "true" ] ||
      [ "$(docker volume inspect -f '{{index .Labels "opencloud.site_id"}}' "$site_volume")" != "$site_id" ]; then
      echo "refusing to remove volume with mismatched ownership: $site_volume" >&2
      return 1
    fi
    docker volume rm "$site_volume" >/dev/null
  fi
}

cleanup() {
  remove_owned_site_resources || true
  if docker container inspect "$caddy_name" >/dev/null 2>&1; then
    if [ "$(docker inspect -f '{{index .Config.Labels "opencloud.validation"}}' "$caddy_name")" = "$validation_id" ]; then
      docker container rm -f "$caddy_name" >/dev/null 2>&1 || true
    fi
  fi
  if docker image inspect "$site_image" >/dev/null 2>&1; then
    if [ "$(docker image inspect -f '{{index .Config.Labels "opencloud.managed"}}' "$site_image")" = "true" ]; then
      docker image rm "$site_image" >/dev/null 2>&1 || true
    fi
  fi
}
trap cleanup EXIT

docker build -t "$site_image" "${repo_root}/deploy/site-runtime"
docker run -d \
  --name "$caddy_name" \
  --network host \
  --label "opencloud.validation=${validation_id}" \
  -v "${repo_root}/deploy/validation/caddy-phase2.json:/etc/caddy/caddy.json:ro" \
  caddy:2.10.2-alpine \
  caddy run --config /etc/caddy/caddy.json >/dev/null

for _ in $(seq 1 30); do
  if docker exec "$caddy_name" wget -qO- "${caddy_admin_url}/config/" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
docker exec "$caddy_name" wget -qO- "${caddy_admin_url}/config/" >/dev/null

docker run --rm \
  --network host \
  -e DOCKER_INTEGRATION=1 \
  -e "DOCKER_SITE_IMAGE=${site_image}" \
  -e "CADDY_INTEGRATION_URL=${caddy_admin_url}" \
  -e "CADDY_INTEGRATION_PUBLIC_URL=${caddy_public_url}" \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "${repo_root}:/src" \
  -v "${repo_root}/.gocache:/root/.cache/go-build" \
  -v "${repo_root}/.gomodcache:/go/pkg/mod" \
  -w /src/backend \
  golang:1.26-bookworm \
  sh -c "go test -tags=integration ./internal/provisioner -run TestDockerCaddyLifecycleAgainstDisposableBackend -count=1 -v"

remove_owned_site_resources
if docker container inspect "$site_name" >/dev/null 2>&1; then
  echo "validation site container remains after cleanup" >&2
  exit 1
fi
if docker network inspect "$site_network" >/dev/null 2>&1; then
  echo "validation site network remains after cleanup" >&2
  exit 1
fi
if docker volume inspect "$site_volume" >/dev/null 2>&1; then
  echo "validation site volume remains after cleanup" >&2
  exit 1
fi

echo "Phase 2 disposable Docker/Caddy lifecycle validation passed."

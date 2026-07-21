#!/usr/bin/env bash
set -Eeuo pipefail

readonly mode="${1:-verify}"
readonly spike_host="${SPIKE_SITE_HOST:-}"
readonly host_port="${SPIKE_HOST_PORT:-18080}"
readonly image="${SPIKE_IMAGE:-opencloud/phase0-spike:local}"
readonly container="${SPIKE_CONTAINER:-opencloud-phase0-spike}"
readonly network="${SPIKE_NETWORK:-opencloud-phase0-spike}"
readonly volume="${SPIKE_VOLUME:-opencloud-phase0-spike-data}"
readonly caddyfile="${CADDYFILE_PATH:-/etc/caddy/Caddyfile}"
readonly snippet_dir="${CADDY_SNIPPET_DIR:-/etc/caddy/conf.d}"
readonly snippet="${snippet_dir}/opencloud-phase0-spike.caddy"
readonly managed_label="opencloud.managed=true"
readonly spike_label="opencloud.spike=phase0-docker-caddy"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
site_dir="${script_dir}/site"

if [[ "$(id -u)" -eq 0 ]]; then
  sudo_cmd=()
else
  sudo_cmd=(sudo)
fi

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "required command not found: $1" >&2
    exit 1
  fi
}

validate_input() {
  if [[ -z "${spike_host}" ]]; then
    echo "SPIKE_SITE_HOST is required" >&2
    exit 1
  fi
  if [[ ! "${spike_host}" =~ ^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$ ]]; then
    echo "SPIKE_SITE_HOST must be a lowercase DNS hostname" >&2
    exit 1
  fi
  if [[ ! "${host_port}" =~ ^[0-9]+$ ]] || ((host_port < 1024 || host_port > 65535)); then
    echo "SPIKE_HOST_PORT must be between 1024 and 65535" >&2
    exit 1
  fi
}

ensure_network() {
  if ! docker network inspect "${network}" >/dev/null 2>&1; then
    # Host-level Caddy reaches the container through a loopback-published port.
    # Docker suppresses published ports on an --internal network, so use a
    # dedicated bridge and keep the public boundary at 127.0.0.1 instead.
    docker network create --label "${managed_label}" --label "${spike_label}" "${network}" >/dev/null
  fi
}

ensure_volume() {
  if ! docker volume inspect "${volume}" >/dev/null 2>&1; then
    docker volume create --label "${managed_label}" --label "${spike_label}" "${volume}" >/dev/null
  fi
}

ensure_container() {
  if docker container inspect "${container}" >/dev/null 2>&1; then
    if [[ "$(docker inspect --format '{{ index .Config.Labels "opencloud.spike" }}' "${container}")" != "phase0-docker-caddy" ]]; then
      echo "refusing to reuse unmanaged container ${container}" >&2
      exit 1
    fi
  else
    docker run --detach \
      --name "${container}" \
      --network "${network}" \
      --mount "type=volume,source=${volume},target=/data" \
      --publish "127.0.0.1:${host_port}:8080" \
      --label "${managed_label}" \
      --label "${spike_label}" \
      --restart unless-stopped \
      --read-only \
      --cap-drop ALL \
      --security-opt no-new-privileges \
      --pids-limit 64 \
      --memory 128m \
      --cpus 0.25 \
      "${image}" >/dev/null
  fi

  if [[ "$(docker inspect --format '{{.State.Running}}' "${container}")" != "true" ]]; then
    docker start "${container}" >/dev/null
  fi
}

ensure_caddy_route() {
  local import_line="import ${snippet_dir}/*.caddy"
  local temp_snippet
  temp_snippet="$(mktemp)"
  trap 'rm -f "${temp_snippet:-}"' RETURN

  "${sudo_cmd[@]}" install -d -m 0755 "${snippet_dir}"
  if ! "${sudo_cmd[@]}" grep -Fqx "${import_line}" "${caddyfile}"; then
    "${sudo_cmd[@]}" cp --preserve=mode,timestamps "${caddyfile}" "${caddyfile}.opencloud-phase0.bak"
    printf '\n%s\n' "${import_line}" | "${sudo_cmd[@]}" tee -a "${caddyfile}" >/dev/null
  fi

  cat >"${temp_snippet}" <<EOF
${spike_host} {
    encode zstd gzip
    reverse_proxy 127.0.0.1:${host_port}
}
EOF
  "${sudo_cmd[@]}" install -m 0644 "${temp_snippet}" "${snippet}"
  "${sudo_cmd[@]}" caddy validate --config "${caddyfile}" --adapter caddyfile >/dev/null
  "${sudo_cmd[@]}" systemctl reload caddy
}

apply_spike() {
  if ! docker image inspect "${image}" >/dev/null 2>&1; then
    docker build --tag "${image}" "${site_dir}" >/dev/null
  fi
  ensure_network
  ensure_volume
  ensure_container
  ensure_caddy_route
  verify_spike
}

verify_spike() {
  local count
  count="$(docker ps --all --filter "label=${spike_label}" --format '{{.Names}}' | awk -v expected="${container}" '$0 == expected { count++ } END { print count + 0 }')"
  if [[ "${count}" != "1" ]]; then
    echo "expected exactly one managed spike container, found ${count}" >&2
    exit 1
  fi

  docker network inspect "${network}" >/dev/null
  docker volume inspect "${volume}" >/dev/null
  curl --fail --silent --show-error --retry 10 --retry-connrefused --retry-delay 1 \
    "http://127.0.0.1:${host_port}/healthz" | grep -Fq '"status":"ok"'
  curl --fail --silent --show-error --retry 12 --retry-all-errors --retry-delay 5 \
    --resolve "${spike_host}:443:127.0.0.1" "https://${spike_host}/healthz" | grep -Fq '"status":"ok"'
  "${sudo_cmd[@]}" systemctl is-active --quiet caddy
  echo "PASS container=1 network=1 volume=1 local_http=200 caddy_https=200 host=${spike_host}"
}

destroy_spike() {
  "${sudo_cmd[@]}" rm -f "${snippet}"
  "${sudo_cmd[@]}" caddy validate --config "${caddyfile}" --adapter caddyfile >/dev/null
  "${sudo_cmd[@]}" systemctl reload caddy

  if docker container inspect "${container}" >/dev/null 2>&1; then
    docker rm --force "${container}" >/dev/null
  fi
  if docker network inspect "${network}" >/dev/null 2>&1; then
    docker network rm "${network}" >/dev/null
  fi
  if docker volume inspect "${volume}" >/dev/null 2>&1; then
    docker volume rm "${volume}" >/dev/null
  fi
  if docker image inspect "${image}" >/dev/null 2>&1; then
    docker image rm "${image}" >/dev/null
  fi
  echo "PASS removed container network volume image and Caddy route"
}

for command in docker caddy curl grep awk systemctl install; do
  require_command "${command}"
done
validate_input

case "${mode}" in
  apply)
    apply_spike
    ;;
  verify)
    verify_spike
    ;;
  destroy)
    destroy_spike
    ;;
  *)
    echo "usage: SPIKE_SITE_HOST=host.example.com $0 {apply|verify|destroy}" >&2
    exit 1
    ;;
esac

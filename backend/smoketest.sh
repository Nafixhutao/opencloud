#!/usr/bin/env bash
# Local smoke test for the Compose stack. It uses its own project name so its
# containers and volumes never overlap the developer's normal `opencloud` stack.
set -Eeuo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

project=${SMOKETEST_PROJECT_NAME:-opencloud-smoketest}
if [[ $project == opencloud ]]; then
  echo "refusing to use the development Compose project" >&2
  exit 2
fi
compose=(docker compose --project-name "$project")

# Compose requires BETTER_AUTH_SECRET (dashboard/auth-migrate); supply a
# throwaway value so the smoke test never depends on a developer's .env.
export BETTER_AUTH_SECRET=${BETTER_AUTH_SECRET:-smoketest-only-secret-at-least-32-chars}

cleanup() {
  "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "=== rebuild ==="
"${compose[@]}" build migrate api worker auth-migrate dashboard

echo "=== up ==="
"${compose[@]}" up -d

echo "=== wait for /readyz (up to 40s) ==="
ready=false
for i in $(seq 1 40); do
  if curl -fsS http://localhost:8080/readyz >/dev/null; then
    echo "ready after ${i}s"
    ready=true
    break
  fi
  sleep 1
done
if [[ $ready != true ]]; then
  "${compose[@]}" logs api migrate >&2
  echo "stack did not become ready" >&2
  exit 1
fi

echo "=== probes ==="
curl -fsS http://localhost:8080/healthz
echo
curl -fsS http://localhost:8080/readyz
echo
curl -fsS http://localhost:9090/metrics >/dev/null

echo "=== wait for dashboard (up to 40s) ==="
dashboard_ready=false
for i in $(seq 1 40); do
  if curl -fsS http://localhost:3000 >/dev/null; then
    echo "dashboard ready after ${i}s"
    dashboard_ready=true
    break
  fi
  sleep 1
done
if [[ $dashboard_ready != true ]]; then
  "${compose[@]}" logs dashboard auth-migrate >&2
  echo "dashboard did not become ready" >&2
  exit 1
fi

echo "=== auth tables exist ==="
auth_tables=$("${compose[@]}" exec -T postgres \
  psql -U opencloud -d opencloud -Atc \
  "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'auth'")
if (( auth_tables < 4 )); then
  echo "expected >=4 better-auth tables in the auth schema, found ${auth_tables}" >&2
  exit 1
fi

echo "=== worker command ==="
worker_command=$("${compose[@]}" ps worker --format '{{.Command}}')
grep -F '/app/worker' <<<"$worker_command"

echo "=== worker started ==="
worker_logs=$("${compose[@]}" logs worker 2>&1)
grep -F 'worker started' <<<"$worker_logs"

echo "smoke test passed"

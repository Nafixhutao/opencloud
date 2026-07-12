#!/usr/bin/env bash
# Local smoke test for the compose stack (run inside one WSL session so dockerd
# does not get torn down between steps). Not committed — dev convenience only.
set -e
cd /mnt/d/opencloud

echo "=== rebuild ==="
docker compose build api worker >/dev/null 2>&1 && echo "built"

echo "=== up ==="
docker compose up -d >/dev/null 2>&1

echo "=== wait for /readyz (up to 40s) ==="
ready=no
for i in $(seq 1 40); do
  code=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/readyz || true)
  if [ "$code" = "200" ]; then echo "ready after ${i}s"; ready=yes; break; fi
  sleep 1
done

echo "=== /healthz ==="; curl -s -w " [%{http_code}]\n" http://localhost:8080/healthz || true
echo "=== /readyz  ==="; curl -s -w " [%{http_code}]\n" http://localhost:8080/readyz || true
echo "=== /metrics (first line) ==="; curl -s http://localhost:8080/metrics | head -1 || true

echo "=== COMMAND column (worker must run /app/worker) ==="
docker compose ps --format "{{.Service}} | {{.Command}} | {{.Status}}"

echo "=== worker log line ==="
docker compose logs worker 2>&1 | grep -iE "worker started|api listening" | head -2

echo "=== teardown ==="
docker compose down -v >/dev/null 2>&1 && echo "down"

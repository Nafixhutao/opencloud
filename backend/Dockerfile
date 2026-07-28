# syntax=docker/dockerfile:1
# Multi-stage build producing the control-plane runtime plus a separate
# PostgreSQL-client backup target. See docs/DEPLOYMENT.md.

FROM golang:1.26.5-alpine AS build
WORKDIR /src

# Cache module downloads separately from source for faster rebuilds.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
# Static build: no cgo, trimmed paths, no symbol table (smaller binaries).
ENV CGO_ENABLED=0 GOOS=linux
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags='-s -w' -o /out/api              ./cmd/api  && \
    go build -trimpath -ldflags='-s -w' -o /out/worker           ./cmd/worker && \
    go build -trimpath -ldflags='-s -w' -o /out/migrate          ./cmd/migrate && \
    go build -trimpath -ldflags='-s -w' -o /out/bootstrap-admin  ./cmd/bootstrap-admin && \
    go build -trimpath -ldflags='-s -w' -o /out/control-plane-backup ./cmd/control-plane-backup

# Backup/restore target: PostgreSQL client tools plus the static orchestrator.
# It runs unprivileged and writes only to the dedicated backup volume.
FROM postgres:18-alpine AS backup
WORKDIR /app
RUN install -d -m 0700 -o postgres -g postgres /backups
COPY --from=build /out/control-plane-backup /app/control-plane-backup
USER postgres
CMD ["/app/control-plane-backup", "schedule"]

# Distroless nonroot: no shell, unprivileged uid (SECURITY.md).
FROM gcr.io/distroless/static-debian12:nonroot AS runtime
WORKDIR /app
COPY --from=build /out/api /out/worker /out/migrate /out/bootstrap-admin /app/

USER nonroot:nonroot
EXPOSE 8080 9090
# CMD (not ENTRYPOINT): compose `command:` fully replaces it, so the worker
# service runs /app/worker instead of appending it as an arg to /app/api.
CMD ["/app/api"]

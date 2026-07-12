# syntax=docker/dockerfile:1
# Multi-stage build producing one small image that carries all three binaries
# (api, worker, migrate). The compose services differ only by which one they run.
# See docs/DEPLOYMENT.md.

FROM golang:1.26-alpine AS build
WORKDIR /src

# Cache module downloads separately from source for faster rebuilds.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
# Static build: no cgo, trimmed paths, no symbol table (smaller binaries).
ENV CGO_ENABLED=0 GOOS=linux
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags='-s -w' -o /out/api     ./cmd/api  && \
    go build -trimpath -ldflags='-s -w' -o /out/worker  ./cmd/worker && \
    go build -trimpath -ldflags='-s -w' -o /out/migrate ./cmd/migrate

# Distroless nonroot: no shell, unprivileged uid (SECURITY.md).
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/api /out/worker /out/migrate /app/

USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/api"]

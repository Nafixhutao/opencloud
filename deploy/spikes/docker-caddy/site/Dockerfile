FROM golang:1.26-alpine AS build

WORKDIR /src
COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/site main.go

FROM scratch
COPY --from=build --chown=65532:65532 /out/site /site
LABEL opencloud.managed="true" opencloud.spike="phase0-docker-caddy"
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/site"]

FROM golang:1.26-alpine AS build

WORKDIR /src
COPY main.go .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/site-runtime main.go

FROM scratch

LABEL org.opencontainers.image.title="OpenCloud static site runtime"
LABEL opencloud.managed="true"
COPY --from=build /out/site-runtime /site-runtime
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/site-runtime"]

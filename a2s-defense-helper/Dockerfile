ARG GO_IMAGE=golang:1.25-alpine
ARG ALPINE_IMAGE=alpine:3.22
FROM ${GO_IMAGE} AS build
ARG GOPROXY=https://proxy.golang.org,direct
WORKDIR /src
COPY a2s-defense-helper/go.mod a2s-defense-helper/go.sum ./
COPY cmd/a2s-defense-helper ./cmd/a2s-defense-helper
COPY internal/a2sdefense/types.go internal/a2sdefense/events.go internal/a2sdefense/sample_window.go internal/a2sdefense/nflog_linux.go internal/a2sdefense/rules.go internal/a2sdefense/runner.go internal/a2sdefense/server.go ./internal/a2sdefense/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/a2s-defense-helper ./cmd/a2s-defense-helper
FROM ${ALPINE_IMAGE}
RUN apk add --no-cache iptables
COPY --from=build /out/a2s-defense-helper /usr/local/bin/a2s-defense-helper
ENTRYPOINT ["a2s-defense-helper"]

ARG GO_IMAGE=golang:1.25-alpine
ARG ALPINE_IMAGE=alpine:3.22
FROM ${GO_IMAGE} AS build
ARG GOPROXY=https://proxy.golang.org,direct
WORKDIR /src
COPY go.mod go.sum ./
RUN GOPROXY="${GOPROXY}" go mod download
COPY cmd/a2s-defense-helper ./cmd/a2s-defense-helper
COPY internal/a2sdefense ./internal/a2sdefense
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/a2s-defense-helper ./cmd/a2s-defense-helper
FROM ${ALPINE_IMAGE}
RUN apk add --no-cache iptables
COPY --from=build /out/a2s-defense-helper /usr/local/bin/a2s-defense-helper
ENTRYPOINT ["a2s-defense-helper"]

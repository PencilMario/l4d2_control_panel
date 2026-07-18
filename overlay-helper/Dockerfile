ARG GO_IMAGE=golang:1.25-alpine
ARG ALPINE_IMAGE=alpine:3.22
FROM ${GO_IMAGE} AS build
ARG GOPROXY=https://proxy.golang.org,direct
WORKDIR /src
COPY go.mod go.sum ./
RUN GOPROXY="${GOPROXY}" go mod download
COPY cmd/overlay-helper ./cmd/overlay-helper
COPY internal/overlayfs ./internal/overlayfs
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/overlay-helper ./cmd/overlay-helper
FROM ${ALPINE_IMAGE}
COPY --from=build /out/overlay-helper /usr/local/bin/overlay-helper
ENTRYPOINT ["overlay-helper"]

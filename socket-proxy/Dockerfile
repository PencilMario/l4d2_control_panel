ARG GO_IMAGE=golang:1.25-alpine
ARG ALPINE_IMAGE=alpine:3.22
FROM ${GO_IMAGE} AS build
ARG GOPROXY=https://proxy.golang.org,direct
WORKDIR /src
COPY go.mod go.sum ./
RUN GOPROXY="${GOPROXY}" go mod download
COPY cmd/socket-proxy ./cmd/socket-proxy
COPY internal/socketproxy ./internal/socketproxy
COPY internal/traffic ./internal/traffic
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/socket-proxy ./cmd/socket-proxy
FROM ${ALPINE_IMAGE}
RUN addgroup -S proxy && adduser -S -D -H -G proxy proxy
COPY --from=build /out/socket-proxy /usr/local/bin/socket-proxy
ENTRYPOINT ["socket-proxy"]

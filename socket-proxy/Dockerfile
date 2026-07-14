ARG GO_IMAGE=golang:1.25-alpine
ARG ALPINE_IMAGE=alpine:3.22
FROM ${GO_IMAGE} AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/socket-proxy ./cmd/socket-proxy
COPY internal/socketproxy ./internal/socketproxy
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/socket-proxy ./cmd/socket-proxy
FROM ${ALPINE_IMAGE}
RUN addgroup -S proxy && adduser -S -D -H -G proxy proxy
COPY --from=build /out/socket-proxy /usr/local/bin/socket-proxy
EXPOSE 2375
ENTRYPOINT ["socket-proxy"]

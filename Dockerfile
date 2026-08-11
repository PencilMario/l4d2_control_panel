ARG NODE_IMAGE=node:22-alpine
ARG GO_IMAGE=golang:1.25-alpine
ARG ALPINE_IMAGE=alpine:3.22
FROM ${NODE_IMAGE} AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build:web

FROM ${GO_IMAGE} AS backend
ARG GOPROXY=https://proxy.golang.org,direct
WORKDIR /src
COPY go.mod go.sum ./
RUN GOPROXY="${GOPROXY}" go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/panel ./cmd/panel

FROM ${ALPINE_IMAGE} AS breakpad
ARG BREAKPAD_REPOSITORY=https://github.com/google/breakpad.git
ARG BREAKPAD_REF=main
RUN apk add --no-cache autoconf automake build-base ca-certificates git libtool linux-headers
WORKDIR /src/breakpad
RUN git clone --depth 1 --branch "${BREAKPAD_REF}" --recurse-submodules --shallow-submodules "${BREAKPAD_REPOSITORY}" .
RUN ./configure --disable-silent-rules
RUN make -j2 src/processor/minidump_stackwalk
RUN make -j2 src/tools/linux/dump_syms/dump_syms

FROM ${ALPINE_IMAGE}
RUN addgroup -S -g 10001 panel && adduser -S -D -H -u 10001 -G panel panel && apk add --no-cache ca-certificates libstdc++ tzdata
COPY --from=backend /out/panel /usr/local/bin/panel
COPY --from=breakpad /src/breakpad/src/processor/minidump_stackwalk /usr/local/bin/minidump_stackwalk
COPY --from=breakpad /src/breakpad/src/tools/linux/dump_syms/dump_syms /usr/local/bin/dump_syms
COPY --from=web /src/web/dist /opt/panel/web
USER panel
ENV L4D2_PANEL_WEB_ROOT=/opt/panel/web L4D2_PANEL_DATA_ROOT=/srv/l4d2-panel
EXPOSE 8080
ENTRYPOINT ["panel"]

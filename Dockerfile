FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.25-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/panel ./cmd/panel

FROM alpine:3.22
RUN addgroup -S panel && adduser -S -G panel panel && apk add --no-cache ca-certificates tzdata
COPY --from=backend /out/panel /usr/local/bin/panel
COPY --from=web /src/web/dist /opt/panel/web
USER panel
ENV L4D2_PANEL_WEB_ROOT=/opt/panel/web L4D2_PANEL_DATA_ROOT=/srv/l4d2-panel
EXPOSE 8080
ENTRYPOINT ["panel"]

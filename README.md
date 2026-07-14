# L4D2 Control Panel

Single-admin Go + React control plane for persistent Left 4 Dead 2 Docker instances. The current implementation establishes the secure data/API core, restricted container contract, serialized jobs, archive/path/overlay safety, player commands, cron validation, operational dashboard, runtime supervisor contract and single-host Compose boundary.

## Local development

Requirements: Go 1.24+, Node 22+, npm 10+.

```sh
go test ./...
cd web && npm ci && npm test -- --run && npm run build
L4D2_PANEL_ADMIN_PASSWORD='use-at-least-12-characters' go run ./cmd/panel
```

The panel stores data below `L4D2_PANEL_DATA_ROOT` (default `/srv/l4d2-panel`) and listens on `:8080`. Put HTTPS in front of it; session cookies are intentionally `Secure`, `HttpOnly`, and `SameSite=Strict`.

## Docker deployment

```sh
cp .env.example .env
# edit .env and use a long random administrator password
docker compose config
docker compose --profile images build runtime-image
docker compose up -d --build
```

If Docker Hub is unavailable, build official stages through Public ECR without changing the daemon: `docker compose build --build-arg OFFICIAL_REGISTRY=public.ecr.aws/docker panel`.

The browser endpoint binds to loopback by default for a reverse proxy. Only the socket proxy mounts `/var/run/docker.sock`; the Panel reaches its restricted HTTP API over an internal control network. Game containers created by the lifecycle adapter use host networking and persistent instance directories.

## Runtime integration smoke checklist

On a Linux Docker host, verify before exposing the service:

1. `docker compose config --quiet` and `docker compose build panel` succeed.
2. `curl http://127.0.0.1:8080/` returns the SPA through the chosen TLS proxy.
3. The Panel container has no Docker socket mount and `DOCKER_HOST` targets `socket-proxy:2375`.
4. A generated game spec contains managed labels, host networking, persistent `game/private` mounts and a read-only shared VPK mount.
5. `l4d2-supervisor status --json`, `stop`, and `attach` work; any other operation exits 64.
6. Recreate Panel/game containers and confirm database/game/private/shared data remain.

## Current integration boundary

Real Docker Engine lifecycle calls, A2S UDP queries, browser PTY WebSocket proxying, GitHub Release downloads, resumable VPK HTTP routes, durable audit/job journals, update rollback orchestration and full browser flows remain to be connected to the implemented contracts. They require the Linux L4D2 runtime integration suite described by the approved design before production use.

See [the approved design](docs/aegis/specs/2026-07-14-l4d2-control-panel-design.md) and [implementation plan](docs/aegis/plans/2026-07-14-l4d2-control-panel.md).

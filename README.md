# L4D2 Control Panel

Single-host, single-administrator control plane for persistent Left 4 Dead 2 dedicated servers. A Go API owns SQLite state, jobs, content deployment, A2S and a fixed native-console bridge; a React SPA provides instance, player, update, content and Cron operations.

The Panel never mounts `/var/run/docker.sock`. A repository-owned socket proxy exposes only the Docker API paths required for labeled game and maintenance containers. Game instances run unprivileged with host networking and persistent bind mounts.

## Requirements

- Linux x86-64 host with Docker Engine and Docker Compose.
- At least 12 GiB free before a first game install; a current App 222860 install uses about 9.3 GiB.
- A TLS reverse proxy on the same host.
- Go 1.24+ and Node 22+ only for local development.

## Deploy

```sh
cp .env.example .env
# Set a long, random L4D2_PANEL_ADMIN_PASSWORD and review the paths/ports.
docker compose --env-file .env config --quiet
docker compose --env-file .env --profile images build runtime-image
docker compose --env-file .env up -d --build
```

Both control services use host networking but bind loopback only:

- Panel: `127.0.0.1:${L4D2_PANEL_HTTP_PORT:-8080}`
- restricted Docker proxy: `127.0.0.1:${L4D2_PANEL_DOCKER_PROXY_PORT:-23750}`

Put HTTPS in front of the Panel. For example, Caddy can proxy WebSocket and SSE traffic without extra directives:

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

Session cookies are `Secure`, `HttpOnly` and `SameSite=Strict`; use the HTTPS origin for normal browser operation. The Panel does not manage firewall rules.

If registry or Steam downloads require a proxy, set `L4D2_PANEL_DOWNLOAD_PROXY` in `.env`. Digest-pinned `NODE_IMAGE`, `GO_IMAGE`, `ALPINE_IMAGE` and an alternate `STEAMCMD_IMAGE` can also be supplied without changing the Docker daemon.

## SteamCMD first install

Current L4D2 Steam content returns `Missing configuration` when an empty Linux install is requested directly. The runtime uses the established anonymous bootstrap sequence:

1. select the Windows platform and install App 222860;
2. switch to Linux and run `app_update 222860 validate`;
3. start `srcds_run` only after both stages succeed.

Later game updates use a fixed Linux-only SteamCMD maintenance container. Optional licensed Steam credentials can be encrypted from System Settings, but anonymous installation is supported and no credentials are written to container logs.

## Persistent data

The default root is `/srv/l4d2-panel`:

```text
panel/panel.db
packages/uploads/
packages/releases/
instances/<id>/game/
instances/<id>/private/
instances/<id>/backups/
instances/<id>/console/
shared-vpk/
```

Rebuilding or deleting a game container preserves these directories unless the administrator explicitly confirms data deletion. Content precedence is `package < shared VPK < private overlay`.

## Runtime and security checks

Before exposing a new host, verify:

```sh
docker compose --env-file .env ps
curl --fail http://127.0.0.1:${L4D2_PANEL_HTTP_PORT:-8080}/api/health
ss -ltn | grep -E '127\.0\.0\.1:(8080|23750)'
```

Then create an instance from the UI and confirm:

- the game container has `network_mode=host` and the three `io.l4d2-panel.*` labels;
- game/private/shared paths are persistent mounts and shared VPK is read-only;
- the container user is UID/GID 10001 and has no Docker socket or privileged mode;
- `l4d2-supervisor attach`, `status --json` and `stop` work, while other operations are rejected;
- A2S, players, console reconnect/replay, stop/start and container rebuild work without data loss.

## Development and verification

```sh
go test -count=1 ./...
go vet ./...
cd web
npm ci
npm test -- --run
npm run build
cd ..
docker compose --env-file .env.example config --quiet
```

On Windows, antivirus/file-indexing can transiently lock Go's randomly named test executables under `%TEMP%`. If affected, set `GOTMPDIR` to a dedicated temporary directory and run packages serially with `go test -p 1`; do not weaken product code to accommodate the local test host.

See [the approved design](docs/aegis/specs/2026-07-14-l4d2-control-panel-design.md), [implementation plan](docs/aegis/plans/2026-07-14-l4d2-control-panel.md) and [evidence bundle](docs/aegis/work/2026-07-14-l4d2-control-panel/50-evidence.md).

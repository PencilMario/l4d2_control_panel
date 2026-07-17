# L4D2 Control Panel

Single-host, single-administrator control plane for persistent Left 4 Dead 2 dedicated servers. A Go API owns SQLite state, jobs, content deployment, A2S and a fixed native-console bridge; a React SPA provides instance, player, update, content and Cron operations.

The Panel never mounts `/var/run/docker.sock`. A repository-owned socket proxy exposes only the Docker API paths required for labeled game and maintenance containers through a mode `0660` Unix socket in a named volume. Game instances run unprivileged with host networking and persistent bind mounts.

## Requirements

- Linux x86-64 host with Docker Engine and Docker Compose.
- At least 12 GiB free before a first game install; a current App 222860 install uses about 9.3 GiB.
- A TLS reverse proxy on the same host.
- Go 1.24+ and Node 22+ only for local development.

## Deploy

```sh
cp .env.example .env
# Set a long random administrator password, and set L4D2_PANEL_GAME_HOST to
# the LAN/Tailscale address on which SRCDS answers A2S (not 127.0.0.1).
docker compose --env-file .env config --quiet
docker compose --env-file .env --profile images build runtime-image
docker compose --env-file .env up -d --build
```

The Panel remains on the Compose network and publishes only its configured HTTP port. The restricted proxy uses host networking so its packet capture sees game traffic, but exposes no TCP listener:

- Panel: `0.0.0.0:${L4D2_PANEL_HTTP_PORT:-18081}` (published from container port `8080`)
- restricted Docker proxy: `/run/l4d2-panel/proxy.sock` in the `panel-proxy-run` named volume shared with the Panel

The proxy drops all capabilities and adds only `NET_RAW`, runs with a read-only root filesystem and `no-new-privileges`, and owns the shared socket as UID/GID 10001. The Panel receives only that restricted filesystem capability; it does not receive the Docker Engine socket.

The overview samples each instance every five seconds and keeps the latest 720 observations in Panel memory (about one hour). CPU, memory, block I/O, process count and uptime come from Docker; A2S supplies map, players and query latency; the restricted proxy counts RX/TX bytes only for the instance's declared game, SourceTV and plugin ports. History is intentionally reset when the Panel restarts and network values become unavailable if packet capture cannot run; other metrics continue independently.

For direct HTTP access, set `L4D2_PANEL_SECURE_COOKIE=false`. Keep the default
`true` when the Panel is served through HTTPS.

`L4D2_PANEL_GAME_HOST` is intentionally required. When the Panel runs in the provided Docker Compose stack, use `host.docker.internal`; the Panel uses Docker's default bridge and Compose maps this name to the same bridge gateway so it can query host-network SRCDS UDP ports. Loopback, host external addresses, and cross-bridge gateway paths may not return A2S traffic correctly from the Panel container.

Put HTTPS in front of the Panel. For example, Caddy can proxy WebSocket and SSE traffic without extra directives:

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:18081
}
```

Session cookies are `Secure`, `HttpOnly` and `SameSite=Strict`; use the HTTPS origin for normal browser operation. The Panel does not manage firewall rules.

If GitHub Release or Steam downloads require a proxy, set
`L4D2_PANEL_DOWNLOAD_PROXY` in `.env`. The Panel uses it as `HTTP_PROXY` and
`HTTPS_PROXY`, and passes it to SteamCMD maintenance containers. Override
`L4D2_PANEL_NO_PROXY` only when additional internal hosts must bypass it.
Digest-pinned `NODE_IMAGE`, `GO_IMAGE`, `ALPINE_IMAGE` and an alternate
`STEAMCMD_IMAGE` can also be supplied without changing the Docker daemon.

## SteamCMD first install

Current L4D2 Steam content returns `Missing configuration` when an empty Linux install is requested directly. Before the first game container is created, the Panel uses a restricted maintenance container for the established anonymous bootstrap sequence:

1. select the Windows platform and install App 222860;
2. switch to Linux and finish the initial `app_update 222860` without validation;
3. deploy the instance's selected plugin package and replay its private overlay;
4. create the run-only game container and start `srcds_run` only after every stage succeeds.

Later game updates and explicit integrity checks use a fixed Linux-only SteamCMD maintenance container with `validate`. Optional licensed Steam credentials can be encrypted from System Settings, but anonymous installation is supported and no credentials are written to container logs.

## Instance startup configuration

Upload at least one ZIP plugin package in Content Repository before creating an instance. Every instance independently stores a selected package and the package whose deployment last committed successfully.

The same configuration dialog is used for new and existing instances. It exposes the managed game, SourceTV and plugin ports, start map, game mode, tickrate and player limit. Additional SRCDS arguments are parsed as shell-style arguments and appended after the managed values; Panel-owned options such as `-port`, `-tickrate`, `+map` and `+tv_port` are rejected. The dialog previews the complete `srcds_run` command before submission.

Changing startup values or the selected package on an installed instance creates one serialized `reconfigure` Job. Package deployment and container rebuild preserve the instance's stopped/running intent, and the applied package ID advances only after deployment commits.

From an instance card, **Update** opens a selective forced-reinstall dialog. It can reinstall the game files, fully redeploy the instance's currently selected plugin package, or do both in one serialized Job; both choices are selected by default. The operation does not check for newer package versions or switch the selected package.

## Private files and console

Each instance has an independent **Private Files** Tab. File edits, uploads, renames and deletions are staged in the instance workspace; **Apply changes** commits the complete staged diff as one background Job. After a successful apply, snapshot pruning makes a best-effort attempt to retain the latest 20 snapshots by default. A prune failure is reported diagnostically without failing the committed apply, so retention can temporarily exceed 20. Restoring a snapshot also runs transactionally, and deleting a private override restores the current package/shared/Valve lower-layer file when one exists instead of leaving stale private content behind.

Uploads are chunked and resumable only when the instance, target path and complete file fingerprint match. The fingerprint includes the file name, size, last-modified time and digest. The browser resumes from the server-confirmed offset after interruption and refreshes the workspace only after completion; upload sessions do not expose game paths or command execution.

## Match and player operations

The instance player dialog combines a live SRCDS match summary with human-player operations. It shows hostname, map, human capacity, version/security, operating system and private/public addresses, followed by UserID, Steam UniqueID, connected time, ping, loss and A2S score for each human player. BOTs remain hidden. Desktop uses an operations table while mobile folds each row into a readable detail card; kick and permanent-ban actions continue to use the mapped numeric UserID.

The instance console follows the latest output while the viewport is at the bottom. Scrolling up pauses following without discarding incoming output; returning to the bottom resumes it. Reconnect replay follows the same rule, so user-selected history is not pulled away by live or replayed lines.

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

After a new shared VPK is published, each running instance receives one deferred restart task. The task waits indefinitely while players are online and restarts the instance when it becomes empty. Three consecutive player-query failures are treated as an abnormal state and trigger the restart. Stopped or uninstalled instances are not started automatically; multiple VPK uploads merge into the same per-instance task.

## Runtime and security checks

Before exposing a new host, verify:

```sh
docker compose --env-file .env ps
curl --fail http://127.0.0.1:${L4D2_PANEL_HTTP_PORT:-18081}/api/health
docker compose exec panel test -S /run/l4d2-panel/proxy.sock
docker compose exec panel test ! -e /var/run/docker.sock
docker compose exec socket-proxy sh -c 'test "$(stat -c %U:%G /run/l4d2-panel)" = root:10001'
docker compose exec socket-proxy sh -c 'test "$(stat -c %a /run/l4d2-panel)" = 750'
docker compose exec socket-proxy sh -c 'test "$(stat -c %U:%G /run/l4d2-panel/proxy.sock)" = root:10001'
docker compose exec socket-proxy sh -c 'test "$(stat -c %a /run/l4d2-panel/proxy.sock)" = 660'
test "$(docker inspect -f '{{json .HostConfig.CapAdd}}' "$(docker compose ps -q socket-proxy)")" = '["NET_RAW"]'
docker compose exec socket-proxy sh -c '! grep -q ":5CC6" /proc/net/tcp /proc/net/tcp6'
```

Then create an instance from the UI and confirm:

- the game container has `network_mode=host` and the three `io.l4d2-panel.*` labels;
- game/private/shared paths are persistent mounts and shared VPK is read-only;
- the container user is UID/GID 10001 and has no Docker socket or privileged mode;
- `l4d2-supervisor attach`, `status --json` and `stop` work, while other operations are rejected;
- A2S, players, console reconnect/replay, stop/start and container rebuild work without data loss.
- overview RX/TX totals increase only when traffic crosses declared ports; rates freeze while stopped, restart resets the runtime `StartedAt` boundary, and metric gaps render as unavailable rather than zero;
- the socket proxy has only `CAP_NET_RAW`, has no TCP listener on legacy port 23750, and removing the stack leaves no capture process or shared socket behind.

## Development and verification

```sh
go test -count=1 ./...
go vet ./...
cd web
npm ci
npm test -- --run
npm run build
npm run e2e
cd ..
docker compose --env-file .env.example config --quiet
```

Playwright starts an `e2e`-tagged Go fixture on `127.0.0.1:18082`. The fixture uses real HTTP, Secure cookies, SQLite, jobs, SSE, WebSocket, content and update routes while replacing Docker, SRCDS, A2S, Steam and GitHub boundaries. It is excluded from production builds. Local loopback is added to `NO_PROXY`, so the browser suite remains local when download proxies are configured.

The Linux fault-injection acceptance uses disposable, unlabelled containers and a temporary data root. It covers hard Panel interruption, VPK part/metadata divergence, package interruption before journal commit and bounded SRCDS crash restart. Verify the managed game container ID and Docker daemon start signature before and after, then remove every fixture artifact. Docker-daemon restart and ENOSPC injection require a dedicated disposable host; do not run them on a shared Docker host.

On Windows, antivirus/file-indexing can transiently lock Go's randomly named test executables under `%TEMP%`. If affected, set `GOTMPDIR` to a dedicated temporary directory and run packages serially with `go test -p 1`; do not weaken product code to accommodate the local test host.

See the overall [approved design](docs/aegis/specs/2026-07-14-l4d2-control-panel-design.md), [implementation plan](docs/aegis/plans/2026-07-14-l4d2-control-panel.md) and [evidence bundle](docs/aegis/work/2026-07-14-l4d2-control-panel/50-evidence.md). Private file management and console follow have a separate [approved design](docs/aegis/specs/2026-07-15-private-file-manager-console-follow-design.md), [implementation plan](docs/aegis/plans/2026-07-15-private-file-manager-console-follow.md) and [evidence bundle](docs/aegis/work/2026-07-15-private-file-manager-console-follow/50-evidence.md).

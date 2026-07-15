# Baseline read set

- `CONTEXT.md`: canonical definitions for observations, current bandwidth, cumulative traffic and the performance history window.
- `docs/aegis/specs/2026-07-15-instance-performance-history-design.md`: approved scope and compatibility boundary.
- `docs/aegis/work/2026-07-15-overview-live-status/50-evidence.md`: current live overview contract and nullable metric behavior.
- `internal/docker/client.go`: Docker inspect/stats owner.
- `cmd/socket-proxy/main.go`, `internal/socketproxy/policy.go`, `docker-compose.yml`: restricted Docker boundary and deployment owner.
- `internal/httpapi/server.go`: authenticated overview HTTP owner.
- `web/src/app/App.tsx`, `web/src/styles/app.css`: overview polling and presentation owner.

## Facts, assumptions and unknowns

- Fact: game containers use Host networking, so Docker network stats cannot reliably identify traffic per instance.
- Fact: each instance owns unique configured game, SourceTV and plugin ports.
- Fact: the current browser triggers one overview observation per instance every five seconds.
- Assumption: production uses Linux with cgroup/container timestamps and an AF_PACKET-capable kernel.
- Unknown: the exact deployment interfaces carrying LAN, WAN or Tailscale traffic; the collector must enumerate non-loopback interfaces and tolerate interface changes.
- Unknown: Windows cannot provide the production capture path; Linux smoke evidence remains required.

## Compatibility boundary

Preserve Host-networked game containers, unprivileged SRCDS, the Panel-without-Docker-socket rule, the Docker endpoint whitelist, lifecycle behavior, existing overview fields, `null` versus real zero semantics and all player/content/update routes.

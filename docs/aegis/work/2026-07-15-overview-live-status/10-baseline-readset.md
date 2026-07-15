# Baseline read set

- `docs/aegis/specs/2026-07-14-l4d2-control-panel-design.md`: approved overview, lifecycle, Docker+A2S health and player-query contract.
- `README.md`: required `L4D2_PANEL_GAME_HOST`, restricted Docker proxy and verification commands.
- `internal/docker/client.go`: Docker liveness and resource owner.
- `internal/a2s/client.go`: current repository-owned UDP parser to retire in favor of a maintained library adapter.
- `internal/players/service.go`: A2S_INFO/A2S_PLAYER and console UserID join boundary.
- `internal/httpapi/server.go`: authenticated API contract owner.
- `web/src/app/App.tsx`: overview refresh and presentation owner.

## Facts, assumptions and unknowns

- Fact: the UI only enriches instances whose persisted `actual_state` is `running` and silently maps observation errors to numeric zero.
- Fact: overview player count currently depends on A2S_INFO, A2S_PLAYER and console `status`, although A2S_INFO already carries the authoritative live count.
- Fact: Docker stats requests set `one-shot=true`, which bypasses Docker's second CPU sample and can leave the CPU delta at zero.
- Fact: `github.com/rumblefrog/go-a2s v1.0.3` is MIT licensed, keeps challenge exchanges on one UDP connection and supports split A2S_PLAYER responses.
- Unknown: a real Docker/SRCDS runtime is not available on this Windows host; deterministic protocol/API/browser tests must cover the contract, with live-host verification recorded as residual risk.

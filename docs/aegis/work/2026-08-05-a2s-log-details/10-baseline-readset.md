# Baseline read set

- `docs/aegis/specs/2026-08-05-a2s-defense-instance-logs-design.md`
- `docs/aegis/specs/2026-08-05-a2s-defense-log-details-design.md`
- `internal/a2sdefense/events.go`, `nflog_linux.go`, `runner.go`, `rules.go`
- `internal/a2sdefense/*_test.go`
- `internal/gamelogs/manager.go` and `manager_test.go`

Baseline evidence: `go test ./internal/a2sdefense ./internal/gamelogs ./cmd/a2s-defense-helper ./cmd/panel -count=1` passed at `22f36b7`.

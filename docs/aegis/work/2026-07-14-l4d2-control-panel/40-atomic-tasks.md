# Atomic task checkpoint

- [ ] Backend persistence expansion (active: auth/session/job/audit/content/scheduler)
- [ ] Authentication and complete HTTP contract
- [ ] Real Docker lifecycle, reconciliation and persistent jobs
- [ ] Complete content and update pipelines
- [ ] Console, A2S, players, scheduler and audit integration
- [ ] React API integration and complete pages
- [ ] Runtime images, deployment and full verification (partial: core images verified; full runtime smoke pending)

Next: make credentials and sessions survive reopening SQLite, then persist jobs and audit events.

Evidence: config tests passed via a deliberately named compiled test binary (Windows blocks the exact temporary name `config.test.exe`); `go test -count=1 ./internal/store` passed. Drift check: continue; scope and compatibility boundary unchanged.

Authentication/API evidence: `go test -count=1 ./internal/auth ./internal/httpapi` passed. Sessions are intentionally in memory for the first implementation, so Panel restart logs the administrator out without weakening password persistence requirements. Drift check: continue; no alternate control path added.

Container/job evidence: `go test -count=1 ./internal/docker ./internal/ports ./internal/jobs` passed. This slice defines the restricted canonical container/exec contract; live Engine calls remain for Linux integration. Drift check: continue; no raw exec or bridge-network fallback introduced.

Content evidence: `go test -count=1 ./internal/safepath ./internal/archive ./internal/content` passed, including Windows/POSIX absolute paths, traversal, symlink, size, hot-path and private-overlay checks. Full backup/rollback orchestration remains in the job pipeline. Drift check: continue; precedence is unchanged.

Operations evidence: `go test -count=1 ./internal/players ./internal/scheduler` passed. Numeric UserID commands and cron parsing are canonical; live A2S/PTY attach remain Linux integration surfaces. Drift check: continue; no player-name interpolation or arbitrary exec added.

Frontend evidence: `npm test -- --run` passed 2 component tests and `npm run build` produced the production bundle. The tested main journey covers operational visibility and stop confirmation; live API hydration remains deployment integration. Drift check: continue; no destructive shortcut added.

Local release evidence: `go test ./...` and `go vet ./...` passed; frontend tests/build passed; `docker compose --env-file .env.example config --quiet` passed. Next: isolated remote Docker image build on `sirphomesv`. Drift check: continue; remote actions are build-only and do not start services.

Remote evidence: Linux Docker 29.6.1 and Compose 5.2.0 detected; Compose config passed. Passwordless sudo reached the daemon, but the configured `docker.1panel.live` mirror returned HTTP 403 for `golang`, `node`, and `alpine`, so image compilation remains unverified. Temporary source was removed. Drift decision: needs-verification; do not claim full deployment or design acceptance.

Registry workaround evidence: Public ECR plus build-only HTTP(S) proxy arguments built the complete Panel image without daemon restart. `dockerproxy.net` supplied the SteamCMD base, and the runtime image built after the duplicate-user defect was reproduced, regression-tested, and fixed. Existing host containers were not restarted. Drift decision: needs-verification because SRCDS itself and the full browser-to-container journey were not launched.

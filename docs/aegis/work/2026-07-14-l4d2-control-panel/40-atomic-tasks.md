# Atomic task checkpoint

- [x] Backend foundation and persistence
- [x] Authentication and HTTP contract
- [x] Container lifecycle, ports and jobs
- [x] Safe content and update pipelines
- [x] Console, A2S, players, scheduler and audit
- [x] React administration interface
- [ ] Runtime images, deployment and full verification (remote verification active)

Next: write failing authentication and HTTP contract tests.

Evidence: config tests passed via a deliberately named compiled test binary (Windows blocks the exact temporary name `config.test.exe`); `go test -count=1 ./internal/store` passed. Drift check: continue; scope and compatibility boundary unchanged.

Authentication/API evidence: `go test -count=1 ./internal/auth ./internal/httpapi` passed. Sessions are intentionally in memory for the first implementation, so Panel restart logs the administrator out without weakening password persistence requirements. Drift check: continue; no alternate control path added.

Container/job evidence: `go test -count=1 ./internal/docker ./internal/ports ./internal/jobs` passed. This slice defines the restricted canonical container/exec contract; live Engine calls remain for Linux integration. Drift check: continue; no raw exec or bridge-network fallback introduced.

Content evidence: `go test -count=1 ./internal/safepath ./internal/archive ./internal/content` passed, including Windows/POSIX absolute paths, traversal, symlink, size, hot-path and private-overlay checks. Full backup/rollback orchestration remains in the job pipeline. Drift check: continue; precedence is unchanged.

Operations evidence: `go test -count=1 ./internal/players ./internal/scheduler` passed. Numeric UserID commands and cron parsing are canonical; live A2S/PTY attach remain Linux integration surfaces. Drift check: continue; no player-name interpolation or arbitrary exec added.

Frontend evidence: `npm test -- --run` passed 2 component tests and `npm run build` produced the production bundle. The tested main journey covers operational visibility and stop confirmation; live API hydration remains deployment integration. Drift check: continue; no destructive shortcut added.

Local release evidence: `go test ./...` and `go vet ./...` passed; frontend tests/build passed; `docker compose --env-file .env.example config --quiet` passed. Next: isolated remote Docker image build on `sirphomesv`. Drift check: continue; remote actions are build-only and do not start services.

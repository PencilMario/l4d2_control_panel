# Atomic task checkpoint

- [x] Backend foundation and persistence
- [ ] Authentication and HTTP contract (active)
- [ ] Container lifecycle, ports and jobs
- [ ] Safe content and update pipelines
- [ ] Console, A2S, players, scheduler and audit
- [ ] React administration interface
- [ ] Runtime images, deployment and full verification

Next: write failing authentication and HTTP contract tests.

Evidence: config tests passed via a deliberately named compiled test binary (Windows blocks the exact temporary name `config.test.exe`); `go test -count=1 ./internal/store` passed. Drift check: continue; scope and compatibility boundary unchanged.

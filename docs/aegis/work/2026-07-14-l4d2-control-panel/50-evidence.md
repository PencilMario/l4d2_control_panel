# Evidence bundle

## Verified locally

- Go unit/integration packages: `go test ./...`
- Static analysis: `go vet ./...`
- React component behavior: `cd web && npm test -- --run`
- React production bundle: `cd web && npm run build`
- Compose schema/interpolation: `docker compose --env-file .env.example config --quiet`

## Remote Linux check

- Host: SSH alias `sirphomesv`
- Runtime: Linux, Docker 29.6.1, Compose 5.2.0
- Compose configuration parsed successfully.
- Docker daemon access succeeded with passwordless sudo.
- Panel image build stopped before compilation because the host mirror `docker.1panel.live` returned HTTP 403 for base-image manifests.
- The isolated `/tmp/l4d2-panel-build.*` source directory was removed; no service was started.

## Residual acceptance gaps

- Real Docker lifecycle adapter and container reconciliation.
- Durable admin credential, sessions, jobs, audits, journals, and secrets.
- Bidirectional PTY WebSocket with replay and browser terminal.
- A2S query and status-to-UserID mapping.
- Package upload/Release acquisition, chunked VPK endpoints, manifests, backup/rollback, full update pipelines.
- Private-file UI/API, scheduler execution, monitoring and full E2E/fault-injection suite.
- Successful Linux image build and L4D2 runtime smoke test.

Confidence: B for implemented contracts and local build; C for deployment; not an authoritative completion signal.

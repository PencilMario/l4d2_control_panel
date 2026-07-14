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
- The host mirror `docker.1panel.live` returned HTTP 403, so build-only registry/proxy arguments were used without restarting Docker.
- Panel image built successfully through Public ECR; Go dependency download used the authorized HTTP proxy.
- Runtime image built successfully using `dockerproxy.net/cm2network/steamcmd:root`.
- A duplicate `steam` user build defect was reproduced, covered by `go test ./runtime`, fixed in the runtime Dockerfile, and reverified by a successful image build.
- No service was started and the existing nine containers were not restarted.

## Residual acceptance gaps

- Real Docker lifecycle adapter and container reconciliation.
- Durable admin credential, sessions, jobs, audits, journals, and secrets.
- Bidirectional PTY WebSocket with replay and browser terminal.
- A2S query and status-to-UserID mapping.
- Package upload/Release acquisition, chunked VPK endpoints, manifests, backup/rollback, full update pipelines.
- Private-file UI/API, scheduler execution, monitoring and full E2E/fault-injection suite.
- L4D2 installation/start, PTY attach and persistent-data smoke test on Linux.

Confidence: B for implemented contracts and local build; C for deployment; not an authoritative completion signal.

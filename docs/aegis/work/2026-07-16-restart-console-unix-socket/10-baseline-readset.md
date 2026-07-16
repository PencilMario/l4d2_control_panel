# Baseline Read Set

- `CONTEXT.md`: defines game instance, desired state, actual state, and live overview terminology.
- `internal/docker/client.go`: canonical Docker HTTP and hijack transport owner.
- `internal/docker/client_test.go`: existing HTTP and Unix Socket transport tests.
- `internal/lifecycle/service.go`: restart is Stop followed by Start; Stop faults the instance on Docker stop errors.
- `internal/lifecycle/service_test.go`: lifecycle state and engine integration tests.
- `runtime/supervisor.py`: supervisor stop writes `quit`, so PID 1 can exit before Docker receives its stop request.
- `docker-compose.yml`: production `DOCKER_HOST` is `unix:///run/l4d2-panel/proxy.sock`.
- `internal/socketproxy/policy.go`: both exec attach and lifecycle endpoints are permitted.
- Commit `7d0aa62`: migrated normal Docker HTTP calls to a Unix Socket but left the hijack path dialing `docker:80`.

## Facts, assumptions, unknowns

- Fact: `NewEngine` maps Unix Socket HTTP traffic to `http://docker` with a custom `http.Transport`.
- Fact: `AttachSupervisor` bypasses that transport and TCP-dials the mapped `docker` host.
- Fact: Docker uses `304 Not Modified` when stop targets an already stopped container, while the client rejects every non-2xx response.
- Fact: restart aborts after any Stop error and persists `faulted`; a stopped container cannot serve supervisor attach.
- Assumption to prove by RED tests: these two transport/status gaps reproduce the reported console and restart symptoms.
- Unknown: the user's deployed container logs are unavailable because the local Docker daemon is not running.

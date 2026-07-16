# Restart and Unix Socket Console Repair Intent

## Requested outcome

After an administrator restarts a game instance, the instance returns to the running state instead of becoming faulted, and the browser console can attach through the configured Docker Unix Socket proxy.

## Scope

- Repair Docker hijack connections so `AttachSupervisor` uses the same Unix Socket transport selected by `NewEngine`.
- Treat Docker's `304 Not Modified` response to `POST /containers/{id}/stop` as an idempotent stop success.
- Protect the restart lifecycle and console attach paths with deterministic regression tests.

## Non-goals

- No frontend, HTTP API, lifecycle state model, A2S timeout, supervisor protocol, or socket-proxy policy changes.
- No retry, alternate transport, or compatibility fallback is added.

## Success criteria

- A Unix-backed engine can create and hijack a supervisor attach exec over the Unix Socket.
- Restart continues to Docker start when the supervisor has already stopped the container.
- Other Docker API failures remain errors.
- Docker, lifecycle, HTTP API, frontend, build, and end-to-end regressions pass.

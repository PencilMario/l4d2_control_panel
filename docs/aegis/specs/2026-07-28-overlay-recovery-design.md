# OverlayFS Restart Recovery Design

## Problem

Docker or host restarts remove the kernel OverlayFS mounts used by every game instance. The persistent `game -> overlay/merged` links and overlay upper/work directories remain, but current containers then start against an empty merged directory and fail because `srcds_run` is absent.

## Design

Panel startup remains the canonical recovery owner. Before lifecycle container reconciliation, it reads the shared game state. When the state is `ready` and has an active release, it calls the existing overlay-helper `Ensure(instance ID, active release ID)` operation for every persisted instance. Only after all mounts are available does ordinary container reconciliation adopt containers and run health checks.

An unavailable or non-ready shared game state does not invent a release or alter data. Recovery returns an error containing the affected instance ID; the existing startup path logs reconciliation as deferred and leaves containers and instance data intact.

## Compatibility Boundary

- Keep the database schema, instance directory layout, container labels and public HTTP API unchanged.
- Keep overlay-helper as the only privileged mount owner.
- Keep lifecycle `Reconcile` responsible only for container/database reconciliation.
- Do not automatically start instances whose desired state is stopped.

## Verification

- A failing unit test proves startup recovery calls `Ensure` for all instances before reconciliation.
- Error tests prove missing shared state and per-instance mount failures are reported without continuing.
- Full Go tests and vet cover regressions.
- Remote verification proves all seven mounts survive a Panel-only restart and all seven SRCDS processes remain running.

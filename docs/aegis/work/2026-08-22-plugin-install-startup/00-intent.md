# Plugin Install and Startup Repair Intent

## Requested outcome

Make plugin package installation readable by the Panel runtime and make an existing game container start only after its per-instance OverlayFS view has been restored.

## Scope

- Repair ownership of `/srv/l4d2-panel/packages` during Compose initialization.
- Reuse `provisioning.Service` as the shared-game release resolver and Overlay owner.
- Ensure a single instance Overlay before Accelerator setup and Docker container start.
- Keep the existing startup recovery and safe `game/current` fallback for failed or legacy shared-game state.
- Deploy and verify the repair on the SSH host `琥珀`.

## Non-goals

- Do not recursively change ownership of the whole data root.
- Do not change instance schema, public APIs, container labels, or desired-state semantics.
- Do not automatically start instances whose desired state is stopped.
- Do not remove the existing privileged overlay-helper boundary.

## Risk hints

- Package archives and manifests can be created by root-owned helper processes.
- A host or shared-game update can remove kernel mounts while persistent Overlay directories remain.
- The remote database currently reports a failed shared-game migration, so fallback behavior must be tested before restart.

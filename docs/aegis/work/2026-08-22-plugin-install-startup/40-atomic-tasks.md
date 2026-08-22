# Atomic Tasks

1. Add failing provisioning tests for failed-state/current-link fallback and per-instance Overlay ensure.
2. Add a failing lifecycle test proving existing-container start orders Overlay ensure before Accelerator and Docker start.
3. Add a failing Compose test proving `panel-init` repairs only `packages` recursively.
4. Implement the shared release resolver and lifecycle hook.
5. Implement scoped Compose package ownership repair.
6. Run local focused and full verification, then deploy and perform remote runtime checks.

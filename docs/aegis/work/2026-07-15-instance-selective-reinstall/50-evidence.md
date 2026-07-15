# Instance Selective Reinstall Evidence

## Automated Verification

- `go test -p 1 ./... -count=1`: PASS for all production Go packages using a dedicated `GOTMPDIR` to avoid the documented Windows temporary-file lock.
- `go test -tags e2e ./cmd/e2e-fixture -count=1`: PASS.
- `npm test -- --run`: PASS, 4 files and 58 tests.
- `npm run build`: PASS, TypeScript and Vite production build completed.
- `npm run e2e -- --project=desktop`: PASS, 1 Playwright administration journey.
- `npm run e2e -- --project=mobile`: PASS, 1 Playwright administration journey.
- `git diff --check`: PASS.

## Behavior Evidence

- Coordinator tests prove package-only reinstall uses `Full` even when the same package is already applied.
- Coordinator tests prove combined reinstall performs one initial stop/start cycle and orders Steam validation before package deployment.
- Failure tests prove a package commit failure stops the restored instance before rollback and then restores its running intent.
- HTTP tests prove omitted fields retain game-only behavior, all three non-empty selections are accepted, and explicit empty selection is rejected.
- React tests prove both options default on, single-target payloads are correct, and empty selection disables confirmation.
- Playwright proves the real browser/API fixture submits both selections as one Job on desktop and mobile.

## Remove / Restore Check

- No temporary instrumentation or product fallback was added.
- `.gotmp`, Playwright output, Vite output, and installed dependencies remain ignored/generated workspace artifacts and are not included in the diff.
- Existing content repository hot/full package actions and scheduled game/package tasks remain active.

## Residual Risk

- Real SteamCMD and Docker maintenance behavior is covered by existing boundary tests and fixture replacements, not exercised against an external Docker daemon in this task.
- A successful Steam validation is intentionally not rolled back when a later package reinstall fails.

## Drift Check

- The implementation remains limited to selectable forced reinstall of the game and current package.
- No automatic GitHub release lookup, version comparison, or package selection was introduced.
- Compatibility is preserved for legacy `{ "confirm": true }` callers and existing independent update paths.
- Decision: `continue` to branch completion.

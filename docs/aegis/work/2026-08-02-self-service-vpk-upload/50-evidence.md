# Self-service VPK upload verification evidence

## Automated verification

- `go test ./...` completed successfully on 2026-08-02. All buildable Go packages passed.
- The first full Go run encountered three Windows `t.TempDir` cleanup errors in unchanged `internal/lifecycle` and `internal/maintenance` tests. `go test ./internal/lifecycle ./internal/maintenance -count=1` passed, and a fresh `go test ./...` then passed. No unrelated source change was made.
- `go test -tags=e2e ./cmd/e2e-fixture` passed after correcting the fixture's self-service authorization-key wiring.
- `npm test -- --run` in `web` passed 17 test files and 195 tests.
- `npm run build` in `web` completed successfully. Vite retained the existing warning that the main JavaScript chunk exceeds 500 kB.
- `git diff --check` reported no whitespace errors.

## Browser verification

The tagged e2e fixture served the production frontend at `http://127.0.0.1:4173`.

- Enabled self-service upload with an empty password through the authenticated settings API and opened `/uploadvpk` directly.
- Captured full-page desktop and mobile screenshots through Playwright.
- Desktop viewport `1440x900`: heading, upload tool, mode selector, empty read-only list, and pagination rendered; body width matched viewport width.
- Mobile viewport `390x844`: body width matched viewport width, with no horizontal overflow; upload drop target and mode selector remained within the viewport.
- Changed the setting to password `maps`, reloaded `/uploadvpk`, and observed the password form without console errors.
- Entered the password and verified successful transition to the upload/list workspace.
- Verified the read-only list exposed only `上一页` and `下一页` buttons, with no download, delete, rename, or overwrite controls.
- Verified no visible error state after authorization.

Playwright MCP rejected local file injection with `DOM.setFileInputFiles: Not allowed`, so the final file-picker-to-completion journey was not browser-automated. The same upload path is covered by the HTTP integration test `TestSelfServiceVPKAuthorizationSettingsListAndUpload`, and browser queue endpoint behavior is covered by `uploadQueue.test.ts` and `SelfServiceVPKPage.test.tsx`.

## Side effects and residual risk

- The e2e fixture process was stopped after verification and its temporary data root was released.
- The temporary browser upload payload was deleted.
- `npm run build` regenerated `web/public/vpk-cleaner.wasm`; this unrelated binary output is intentionally not committed.
- Public upload rate limiting and malware scanning remain explicit non-goals from the approved design.
- Confidence grade: B. Persistence, API, lifecycle, settings, responsive page, authorization, and upload protocol have direct automated evidence; the browser file chooser itself remains covered indirectly because of the automation permission boundary.

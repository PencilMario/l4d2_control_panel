# Evidence

## Test-first implementation

- Baseline: `npm test -- --run src/app/CrashReportsPage.test.tsx` -> 1 file, 3 tests passed.
- Red run after adding the new contracts: missing `stackwalk.ts` and missing vertical diagnostic list, as expected.
- Green focused run: `npm test -- --run src/app/stackwalk.test.ts src/app/CrashReportsPage.test.tsx` -> 2 files, 6 tests passed.
- The parser regression suite also covers `Found by:` annotations appended to the same frame line as the instruction pointer.

## Regression and build

- `npm test -- --run` -> 21 test files, 215 tests passed.
- `npx tsc --noEmit -p tsconfig.json` -> exit code 0.
- `npm run build` -> TypeScript and Vite production build completed successfully; Vite emitted only the existing chunk-size warning.
- `git diff --check` -> no whitespace errors.

## Browser verification

The local Vite build was checked at `http://127.0.0.1:4174/` with Playwright using intercepted session, instance, crash report, metadata and stackwalk responses. No remote data was changed.

- Desktop: four ordered rows `Stackwalk`, `AI 诊断`, `上传元数据`, `崩溃模块`; each row stayed within the detail container (`scrollWidth=clientWidth=606`).
- Mobile viewport `390x844`: page `scrollWidth=375`; all four rows stayed within their containers (`scrollWidth=clientWidth=325`).
- Stackwalk sample rendered 2 frame rows and 2 preserved log rows; `Found by` source text was visible.
- Browser console: 0 errors and 0 warnings during the successful run.
- Fresh post-fix desktop reload also rendered the same four rows, 2 frames and 2 log rows with 0 console errors.
- Screenshots were captured as `crash-diagnostics-desktop.png` and `crash-diagnostics-mobile.png` in the Playwright artifact output.

## Scope boundary

- Existing crash report API paths, download query parameters, AI Markdown reader and report list filtering remain unchanged.
- `web/public/vpk-cleaner.wasm` changed only because the build script regenerated it; it is generated output and is excluded from the feature commit.
- The main worktree remains outside this worktree and still owns the unrelated `deployment_test.go` modification.

## 安可服部署验收

- Deployment source: Git archive of `b988383` was staged into `/home/steam/l4d2_control_panel.next-b988383`; the existing `.env` and `/home/steam/l4d2-panel-data` were preserved. The live source directory was switched with the old source retained at `/home/steam/l4d2_control_panel.backup-5fe9de4-b988383` for short-term rollback.
- Remote build: `docker compose -p l4d2_control_panel --env-file .env build panel` completed; image ID `sha256:d9b9ee2b7be790c9578285b076034c28ad0b130429364ec30f8cdd029a23b121`.
- Service rollout: `docker compose -p l4d2_control_panel --env-file .env up -d --no-deps --force-recreate panel` recreated only `l4d2_control_panel-panel-1`; the existing game and helper containers were not recreated.
- Remote checks: revision file is `b988383`; Panel is `running healthy`; `curl -fsS http://127.0.0.1:18081/api/health` returned `{"containers_running":9,"database":"ok","docker_version":"29.2.1","status":"ok"}`; logs contain `panel listening on 0.0.0.0:18081`.
- Remote Playwright at `http://100.73.249.118:18081/`: desktop showed four ordered vertical rows (`Stackwalk`, `AI 诊断`, `上传元数据`, `崩溃模块`), with 2 frame rows and a preserved warning/log row. At 390px, document `scrollWidth=375` equaled `clientWidth=375`; all four diagnostic rows had equal `scrollWidth/clientWidth=325`; console errors and warnings were 0.
- Deployment archive and build log were removed after acceptance. The rollback source backup was intentionally retained; no remote game data was deleted.

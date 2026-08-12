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

# Evidence

## Regression and build

- Focused command: `npm run test -- --run src/app/CrashReportsPage.test.tsx src/app/stackwalk.test.ts` -> 2 files, 14 tests passed.
- Full frontend command: `npm run test -- --run` -> 21 files, 223 tests passed.
- Production command: `npm run build:web` -> TypeScript and Vite production build completed successfully; Vite emitted only the existing chunk-size warning.
- `git diff --check` -> no whitespace errors.

## Review follow-up

- Review identified that failed Stackwalk preview reads were cached as empty text, making failures indistinguishable from empty output and preventing retry.
- The frontend now caches only successful reads, exposes loading/error/empty states, and offers `重新读取 Stackwalk` after a failed read.
- Backend-non-success Stackwalk states stay in the stable empty state and do not trigger a download request.
- Search focus visibility and narrow-thread selector sizing were preserved for keyboard and mobile use.

## Scope boundary

- Changes are limited to `web/src/app/stackwalk.ts`, `web/src/app/StackwalkView.tsx`, `web/src/app/CrashReportsPage.tsx`, their focused tests, and crash-page CSS.
- Existing crash report API paths, download query parameters, raw Stackwalk text, DMP files, AI input, and backend persistence are unchanged.
- Generated build output, including `web/public/vpk-cleaner.wasm`, is excluded from the change.

## Remote acceptance

- Deployed revision `36c2e6d` to `/home/steam/l4d2_control_panel.next-36c2e6d` on 安可服; only the `panel` service was rebuilt/recreated. Existing helper containers and `/home/steam/l4d2-panel-data` remained in place.
- `curl -fsS http://127.0.0.1:18081/api/health` -> `status: ok`, database `ok`, 11 containers running; `panel` reported healthy after startup.
- Playwright desktop at `http://100.73.249.118:18081/`: authenticated crash page showed recent-list preview `#0 server.so!Crash + 0x10`; Stackwalk defaulted to `Thread 2（崩溃线程）`, showing only `server.so!Crash + 0x10` and `engine.so!Run + 0x20`.
- Playwright thread switch: selecting `Thread 0` showed `worker.so!Idle + 0x8`; ordinary `minidump_stackwalk` log text was absent from the rendered call-stack region.
- Playwright 390px viewport: `document.documentElement.scrollWidth` and `document.body.scrollWidth` were `375`, with the thread selector fitting at 279px; no horizontal overflow was observed.
- Browser console still reports the pre-auth `GET /api/session` 401 during initial page load; no new runtime error was introduced by the authenticated crash page checks.

## Production follow-up: CPU preamble parsing

- Root cause: real Breakpad Stackwalk output contains `1 CPU` before its first `Thread` heading. The permissive numeric-frame parser classified it as a frame in an implicit `Thread 0`; the later real `Thread 0 (crashed)` had the same ID, so the UI selected the preamble thread and rendered only `#1 CPU`.
- Added a parser regression using that Breakpad preamble. The new assertion failed before the implementation change, reproducing the extra preamble thread; it passes after the parser discards pre-heading frames once explicit Thread headings are present. Compact Stackwalk text without Thread headings remains supported.
- Focused regression: `npm run test -- --run src/app/CrashReportsPage.test.tsx src/app/stackwalk.test.ts` -> 2 files, 15 tests passed.
- Full frontend regression: `npm run test -- --run` -> 21 files, 224 tests passed. `npm run build:web` and `git diff --check` also passed; Vite retained only its existing chunk-size warning.
- Deployed revision `946d5f5` to 安可服, rebuilding/recreating only `panel`; `/api/health` returned `status: ok` and the panel reached `healthy`.
- Playwright against real report `d98b6e8354fd791712748ed6b15ff55d052678ed7bdf493f78f45a58ead1924e` (360,688 B): default selection is now only `Thread 0（崩溃线程）`; it renders 36 frames beginning `#0 engine_srv.so + 0x16bea2`, `#1 metamod.2.l4d2.so + 0x2d480`, `#2 engine_srv.so + 0x107a92`. The frame list has `scrollHeight: 1871` and `clientHeight: 360`, so the full call chain is scrollable.

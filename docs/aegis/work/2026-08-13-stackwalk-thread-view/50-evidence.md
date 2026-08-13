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

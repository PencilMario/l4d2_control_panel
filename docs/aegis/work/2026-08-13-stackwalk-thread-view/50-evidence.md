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

- Pending: deploy the committed frontend-only change to the existing `/home/steam/l4d2_control_panel` Compose source on 安可服.
- Pending: verify `http://100.73.249.118:18081/` with Playwright at desktop and 390px mobile widths, including the recent-list top frame, crashed-thread default, thread switching, hidden ordinary logs, and horizontal overflow.

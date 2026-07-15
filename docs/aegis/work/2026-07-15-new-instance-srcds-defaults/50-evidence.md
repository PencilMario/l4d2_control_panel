# Verification Evidence

日期：2026-07-15

## TDD

- Baseline: `npm test -- --run` -> 2 test files, 26 tests passed.
- RED: `npm test -- --run src/app/InstanceConfigModal.test.tsx` before implementation -> 1 failed, 4 passed; the new test received `coop` instead of `versus`.
- GREEN: `npm test -- --run src/app/InstanceConfigModal.test.tsx` after implementation -> 1 test file, 5 tests passed.

## Regression

- `npm test -- --run` after implementation -> 2 test files, 27 tests passed.
- `npm run build` -> TypeScript and Vite build completed successfully; 1779 modules transformed.

## Scope Review

- Changed owner: `web/src/app/InstanceConfigModal.tsx:createDefaults`.
- Protected edit path: the existing component test still passes with persisted `coop`, `8`, and custom `extra_args` values.
- No API, database, Docker, Supervisor, or existing-instance migration changes were made.
- Browser-level create flow and live SRCDS startup were not run; the component test verifies the user-visible initial values and complete preview, while deployment-host behavior remains outside this narrow frontend change.

Confidence: B. Direct RED/GREEN evidence and full frontend regression cover the changed owner; browser/server integration is intentionally unverified.

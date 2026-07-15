# Atomic tasks

- [x] Docker stats waits for a usable CPU sample.
- [x] Existing A2S packet parsing is replaced by the library adapter.
- [x] A2S_INFO summary supplies overview map/player count/capacity.
- [x] Live overview API distinguishes stopped, running and unavailable observations.
- [x] React polls live observations and renders unavailable metrics as `--`.
- [x] Focused and full regression/build verification is recorded.

## TodoCheckpointDraft

- Active slice: none; completion candidate verification and branch review.
- Completed: all six atomic tasks above.
- Evidence refs: all focused RED/GREEN evidence; full Go tests/vet; tagged fixture; 26 Vitest tests; production build; desktop/mobile Playwright journey.
- Blocked on: none.
- Next: commit the verified branch and choose an integration path.

## DriftCheckDraft

- Scope: aligned with overview truthfulness and A2S reuse.
- Compatibility: existing action/player contracts retained.
- New owner/fallback: one overview observation contract replaces the browser-side multi-owner join; no permanent fallback added.
- Retirement: hand-written A2S parsing and silent numeric-zero fallback are explicit.
- Decision: continue; no scope or compatibility drift found.

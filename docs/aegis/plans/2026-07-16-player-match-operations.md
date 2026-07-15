# Player Match And Operations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a live match summary and human-player operations details to each instance's player modal.

**Architecture:** Replace the player-only status regex with a structured `ParseStatusSnapshot` owner that parses match metadata and human rows from one isolated console response. Merge status identities with A2S scores in `players.Service`, expose an additive API shape, and render the confirmed summary-card plus responsive operations-table/card layout in React.

**Tech Stack:** Go 1.24, SRCDS console/A2S, chi JSON API, React 19, TypeScript, Vitest/Testing Library, Playwright.

**Baseline / Authority Refs:** `docs/aegis/specs/2026-07-16-player-match-operations-design.md`, `CONTEXT.md`, `internal/players/status.go`, `internal/players/service.go`, `internal/docker/client.go`, `web/src/app/App.tsx`.

**Compatibility Boundary:** Preserve top-level player snapshot fields, numeric UserID moderation, A2S score ownership, authenticated routes, human-only display, console framing and existing kick/ban confirmations.

**Verification:** `go test ./internal/players ./internal/httpapi ./cmd/e2e-fixture -tags e2e -count=1`, `go test -p 1 ./... -count=1`, `npm test -- --run`, `npm run build`, and desktop/mobile Playwright journeys.

---

### Task 1: Structured L4D2 Status Snapshot

**Files:**
- Modify: `internal/players/status.go`
- Modify: `internal/players/status_test.go`

**Why this task exists:** One canonical parser must turn the supplied real response into match metadata and actionable human identities while excluding BOTs.

**Impact / Compatibility:** Keep `ParseStatus` as a compatibility wrapper while new code uses `ParseStatusSnapshot`. Existing single-column and L4D2 entity-column rows remain accepted.

**Repair Track:** Consolidate scattered/partial status parsing into the status parser owner.

**Retirement Track:** Retire direct player-only regex consumption from `players.Service`; retain the wrapper only for tests or callers until repository search shows no external owner.

**Verification:** `go test ./internal/players -run TestParseStatus -count=1`

- [ ] Add a failing test with the exact supplied hostname/version/udp/os/map/players/human/BOT response and assert every match/player field plus BOT exclusion.
- [ ] Run the focused test and confirm the structured parser is missing.
- [ ] Add `MatchInfo`, `StatusSnapshot`, enriched `StatusPlayer`, anchored summary patterns and tolerant field parsing.
- [ ] Keep `ParseStatus(raw)` delegating to `ParseStatusSnapshot(raw).Players`.
- [ ] Run all player parser tests until green.

### Task 2: Status-First Player Aggregation And API Compatibility

**Files:**
- Modify: `internal/players/service.go`
- Modify: `internal/players/service_test.go`
- Modify: `internal/httpapi/server_test.go`

**Why this task exists:** Operational identity must remain visible without a unique A2S score match while old JSON consumers keep working.

**Impact / Compatibility:** Add `match`, `unique_id`, `connected`, `ping`, and `loss`; preserve `map`, `max_players`, `name`, `user_id`, `score`, and `duration`. Score becomes nullable only when no unique A2S join exists.

**Repair Track:** Make status humans the primary result set and use A2S solely to enrich score/duration.

**Retirement Track:** Retire the A2S-first loop and name-to-UserID slice map after status-first regression tests pass.

**Verification:** `go test ./internal/players ./internal/httpapi -run 'Test(Service|OnlinePlayers)' -count=1`

- [ ] Add failing service tests for unique join, duplicate-name null scores, status-only humans, A2S-only fallback and legacy fields.
- [ ] Run focused tests and verify contract failures.
- [ ] Enrich `OnlinePlayer` and `Snapshot`, implement status-first merge with consumed A2S indices, and preserve compatibility fields.
- [ ] Add an HTTP JSON contract assertion for `match` and player operational fields.
- [ ] Run player and HTTP tests until green.

### Task 3: Responsive Match And Player Operations UI

**Files:**
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/app/App.test.tsx`
- Modify: `web/src/styles/app.css`

**Why this task exists:** Administrators need the confirmed at-a-glance match summary and dense desktop operations table without sacrificing mobile readability.

**Impact / Compatibility:** Keep the modal entry point, error behavior and confirmation dialogs. Desktop uses semantic table headers; the responsive breakpoint presents each human as a detail card using labels.

**Verification:** `npm test -- --run src/app/App.test.tsx && npm run build`

- [ ] Add failing React tests for summary fields, all player columns, unknown/null rendering, action visibility and mobile labels.
- [ ] Run focused Vitest and confirm the new content is absent.
- [ ] Add typed snapshot helpers, summary grid, semantic operations table and responsive data labels.
- [ ] Style the confirmed layout using existing palette, spacing and modal conventions.
- [ ] Run focused tests and production build until green.

### Task 4: Real Browser Fixture And Documentation

**Files:**
- Modify: `cmd/e2e-fixture/main.go`
- Modify: `web/e2e/control-panel.spec.ts`
- Modify: `README.md`
- Create: `docs/aegis/work/2026-07-16-player-match-operations/50-evidence.md`

**Why this task exists:** The actual authenticated browser journey must prove the additive API and responsive UI rather than relying only on unit tests.

**Impact / Compatibility:** Extend only fixture data and assertions; production lifecycle/content behavior stays unchanged.

**Verification:** Full commands from the plan header.

- [ ] Enrich `fixturePlayers.Online` with match and operational fields.
- [ ] Assert match summary and player data in both desktop and mobile Playwright projects while retaining kick confirmation coverage.
- [ ] Document the player match/operations view in README.
- [ ] Run gofmt, `git diff --check`, full Go/Vitest/build, desktop Playwright and mobile Playwright.
- [ ] Record exact evidence, side-effect checks and residual Docker/SRCDS risk.

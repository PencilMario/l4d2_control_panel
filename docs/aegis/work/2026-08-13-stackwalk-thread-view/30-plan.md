# Stackwalk Thread View Implementation Plan

> **For agentic workers:** Inline execution in the existing `crash-manual-ai` worktree. Keep the change frontend-only.

**Goal:** Show only useful per-thread call frames in Stackwalk and identify recent reports by their crashed-thread top frame.

**Architecture:** Extend the existing parser with `StackwalkThread` groups while retaining the raw text field for compatibility. `StackwalkView` selects one parsed thread and renders frames only; `CrashReportsPage` derives the top frame from the report's available Stackwalk text for the selected detail and uses a safe signature fallback in the list when no parsed frame exists.

**Compatibility Boundary:** Do not change any API, downloaded artifact, database field, DMP, backend Stackwalk file, AI input, or download action.

**Verification:** Targeted parser/page tests, full frontend tests, TypeScript check, production build, `git diff --check`, and browser inspection of desktop/mobile Stackwalk rendering.

## Tasks

- [x] Add failing parser and page assertions for Thread grouping, log suppression, Thread switching, and top-frame list labels.
- [x] Implement parser groups and view selection.
- [x] Implement list top-frame label derivation and responsive styles.
- [x] Run frontend verification and inspect the final diff.
- [x] Commit the frontend-only change and deploy the requested live update.

# Automatic Crash Report Token Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the deployment script automatically create and persist `L4D2_PANEL_CRASH_REPORT_TOKEN` when it is missing, while preserving an existing value.

**Architecture:** Keep the deployment script as the owner of host `.env` initialization and repair. Generate a cryptographically random, shell-safe token with the existing `openssl`/`od` fallback strategy used for the administrator password, then atomically rewrite only the missing environment key without changing other settings.

**Tech Stack:** Bash, POSIX utilities, Docker Compose deployment tests.

**Baseline / Authority Refs:** `deploy.sh`, `deploy_test.sh`, `.env.example`, `README.md`, `docker-compose.yml`; the existing deployment contract preserves `.env` across updates and uses mode `0600`.

**Compatibility Boundary:** Existing non-empty `L4D2_PANEL_CRASH_REPORT_TOKEN` values remain byte-for-byte unchanged. Existing `.env` values and deployment flow remain unchanged. The generated token is passed to Compose through the existing environment variable and is not printed.

**Verification:** `bash -n deploy.sh deploy_test.sh`, `bash deploy_test.sh`, and `go test ./internal/config ./internal/accelerator`.

---

### Task 1: Token generation and environment repair

**Files:**
- Modify: `deploy.sh`
- Test: `deploy_test.sh`
- Modify: `.env.example`
- Modify: `README.md`

**Why this task exists:** A missing token currently disables the Accelerator receiver and causes enabled-instance plugin reinstalls to fail after the package has already been deployed.

**Impact / Compatibility:** The deployment script remains the canonical owner. Existing configured tokens are preserved; only missing or empty keys are added. The `.env` file remains mode `0600` and is updated atomically.

**Repair Track:** Add `generate_token`, include a generated token in new `.env` files, and add `ensure_crash_report_token` for existing files.

**Retirement Track:** Retire the empty-token default from the automated deployment path. Keep the explicit empty value in manual Compose behavior as an intentional opt-out only when an operator deliberately removes the key or bypasses `deploy.sh`.

- [ ] Write tests for new-file generation, existing-token preservation, empty-token repair, and safe token format.
- [ ] Run `bash deploy_test.sh` and verify the new tests fail because the token is not generated or repaired.
- [ ] Implement the smallest script changes and update operator documentation.
- [ ] Run the deployment and Go configuration tests.
- [ ] Run shell syntax and diff checks.

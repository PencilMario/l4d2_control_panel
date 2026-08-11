# Accelerator 管理、崩溃查看与 AI 分析 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `aegis:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在现有 Go Panel 中完成可配置的 Accelerator 独立安装、完整上传兼容、崩溃在线查看、服务端 stackwalk 和可选 AI 分析。

**Architecture:** `internal/accelerator` 作为独立 capability manager，负责下载缓存、归档校验、实例文件所有权、SourceMod `core.cfg` 补丁和卸载冲突；它通过窄接口接入 `lifecycle`、插件部署和实例更新任务。`internal/crashreports` 继续拥有 Accelerator wire protocol 和内容寻址存储，并扩展为模块 symbol/binary artifact 与分析状态的唯一 owner；`internal/crashanalysis` 负责异步 stackwalk、脱敏和 OpenAI-compatible 请求。SQLite 只保存实例开关和全局非敏感设置，敏感 AI key 复用现有 encrypted secrets，React 页面通过现有认证 API 访问。

**Tech Stack:** Go 1.25, `net/http`, chi v5, SQLite/modernc, `archive/zip`, `os/exec`, React 19, Vite, Vitest, existing jobs/lifecycle/update managers.

**Baseline / Authority Refs:** `docs/aegis/specs/2026-08-10-accelerator-management-analysis-design.md`; `docs/aegis/specs/2026-08-09-accelerator-crash-report-receiver-design.md`; `CONTEXT.md`; `internal/crashreports`; `internal/httpapi/server.go`; `internal/store`; `internal/domain/models.go`; `internal/lifecycle/service.go`; `internal/provisioning/service.go`; `internal/updates/{coordinator,game,pipeline,shared_reconciler}.go`; `cmd/panel/main.go`; `docker-compose.yml`; upstream `accelerator/extension/extension.cpp` v2 CrashSignature and Core config behavior.

**Compatibility Boundary:** Existing authenticated `/api` routes, session cookies, ordinary plugin package ownership, Overlay/private files, game-log mounts, and host-network deployment remain stable. Public `/submit`, `/symbols/submit`, and `/binary/submit` still require the shared token, loopback TCP source, and a managed instance `server-id.txt`; `X-Forwarded-For`, client paths, `UserID`, and `code_file` are never authorization inputs. `/binary/submit` changes from deliberate rejection to bounded, content-addressed handling of the upstream `code_file` field. No Throttle fallback is retained.

**Verification:** Every task has focused red-green tests. Before handoff run `gofmt`, `go test -p 1 ./... -count=1`, `go vet ./...`, `npm test -- --run`, `npm run build`, `docker compose config --quiet`, `git diff --check`, the real dump multipart/hash/download check using `D:\Windows\Download\crash_rmvtruf2n5tk.dmp`, and a controlled fake Accelerator package install. No live instance restart or remote host rebuild is part of the automated plan.

---

## Scope Check

Facts:

- The receiver already has bounded dump/metadata/symbol storage, loopback checking, managed-instance authorization, authenticated list/detail/download routes, and 90-day default cleanup in the current worktree.
- Instance configuration is a `domain.Instance` persisted by `internal/store.Store`; changes while a container exists are queued through `jobs.Manager`.
- Full plugin deployment and container lifecycle are separate owners, so Accelerator must be called at both boundaries rather than being inserted into a normal plugin archive.
- Existing global settings use the `system_settings` table and existing secrets use `internal/secrets.Service`.
- Upstream v2 CrashSignature records are `2|timestamp|os|cpu|crashed|reason|address|thread|M|debug_file|debug_id|...|F|module|offset|...`; upstream binary uploads use multipart `code_file`, `debug_identifier`, and `code_identifier`.

Assumptions:

- `L4D2_PANEL_LISTEN` contains a concrete TCP port in production; the manager derives `http://127.0.0.1:<port>` from it and never exposes a non-loopback upload URL.
- The configured Accelerator download URL is a complete HTTPS archive URL. A GitHub proxy is applied only when the URL host is `github.com` or `www.github.com`, using the existing global proxy base URL.
- `minidump_stackwalk` accepts the standard `<dump> <symbol-path>` command line. A missing executable is a report-level analysis failure, never an upload failure.
- A report directory is the canonical owner of its derived files; uploaded artifacts are global content-addressed objects referenced by manifests.

Unknowns intentionally bounded by the plan:

- The provided dump may not resolve without Valve/L4D2 symbols; the test proves receipt, parsing/manifest behavior, and stackwalk failure visibility rather than inventing game symbols.
- Public SourceMod/Metamod runtime packages may not contain symbols. The built-in symbol manifest records only verified SourceMod/Metamod sources and the implementation does not package L4D2/Valve artifacts.

## File Ownership Map

- `internal/crashreports/signature.go`: v2 signature parser, module identity, artifact lookup decisions, and `Y/U/N` response construction.
- `internal/crashreports/config.go`, `manager.go`, `protocol.go`, `artifacts.go`: bounded report/artifact storage, manifests, retention, and three public handlers.
- `internal/crashreports/*_test.go`: protocol, authorization, artifact, retention, and AI-outbound safety contracts.
- `internal/domain/models.go`, `internal/store/store.go`, `internal/store/migrations.go`, `internal/store/accelerator.go`: instance capability flags and global settings contracts.
- `internal/accelerator/archive.go`, `corecfg.go`, `manager.go`: download/cache, archive allowlist, Core KeyValues patch, manifest ownership, install/remove.
- `internal/accelerator/*_test.go`: URL/proxy, archive safety, idempotence, restoration, and conflict behavior.
- `internal/crashanalysis/stackwalk.go`, `redaction.go`, `ai.go`, `worker.go`: stackwalk process limits, structured redaction, AI client, persisted queue worker.
- `internal/crashanalysis/*_test.go`: process failure states, timeout/output caps, redaction, request body, retry and restart recovery.
- `internal/lifecycle/service.go`, `internal/provisioning/service.go`, `internal/updates/{coordinator,game,shared_reconciler}.go`: Ensure hook after game/package deployment and before container creation/rebuild.
- `internal/httpapi/server.go`, `internal/httpapi/accelerator.go`, `internal/httpapi/crashreports_test.go`: settings, instance fields, report filtering/download/analyze routes, and job wiring.
- `cmd/panel/main.go`: compose managers, start analysis worker, inject capability hooks, and graceful shutdown.
- `web/src/api/client.ts`, `web/src/app/InstanceConfigModal.tsx`, `web/src/app/CrashReportsPage.tsx`, `web/src/app/App.tsx`, `web/src/styles/app.css`: API types, instance toggles, report page, navigation, and responsive states.
- `.env.example`, `docker-compose.yml`, `README.md`, `assets/crash-symbols/manifest.json`, `assets/crash-symbols/README.md`: deployment defaults, tool path, source/license/hash records, and operator documentation.

## Shared Contracts

The first implementation task owns these names so later tasks do not invent incompatible fields:

```go
type AcceleratorSettings struct {
    DownloadURL    string `json:"download_url"`
    UseGitHubProxy bool   `json:"use_github_proxy"`
}

type CrashAnalysisSettings struct {
    Endpoint string `json:"endpoint"`
    Model    string `json:"model"`
    APIKeySet bool  `json:"api_key_set"`
}

type Instance struct {
    // existing fields remain unchanged
    AcceleratorEnabled bool `json:"accelerator_enabled"`
    AutoCrashAnalysis  bool `json:"auto_crash_analysis"`
}

type Module struct {
    Index          int    `json:"index"`
    DebugFile      string `json:"debug_file"`
    DebugIdentifier string `json:"debug_identifier"`
    CodeIdentifier string `json:"code_identifier,omitempty"`
    Platform       string `json:"platform,omitempty"`
    Architecture   string `json:"architecture,omitempty"`
    Decision       string `json:"decision,omitempty"`
    SymbolArtifact string `json:"symbol_artifact,omitempty"`
    BinaryArtifact string `json:"binary_artifact,omitempty"`
}
```

`Report` gains `InstanceID`, parsed signature fields, `Modules`, `Stackwalk`, and `AIAnalysis`. Binary upload limits are `MaxBinaryBytes = 256 << 20` and `MaxRequestBytes = 512 << 20`; existing dump and metadata limits remain unchanged. Binary download is `file=binary&artifact=<artifact-id>` and rejects missing, unrelated, or unsafe artifact IDs.

### Task 1: Expand Receiver Protocol And Artifact Domain

**Files:**

- Create: `internal/crashreports/signature.go`, `internal/crashreports/artifacts.go`, `internal/crashreports/signature_test.go`, `internal/crashreports/artifacts_test.go`
- Modify: `internal/crashreports/config.go`, `internal/crashreports/manager.go`, `internal/crashreports/protocol.go`, `internal/crashreports/manager_test.go`, `internal/crashreports/protocol_test.go`

**Why this task exists:** The existing receiver is intentionally incomplete for the new contract: it rejects legal binary uploads, returns `N` for every module, and cannot associate a report with module artifacts or an instance ID.

**Impact / Compatibility:** Preserve existing dump/metadata upload response `OK|<crash-id>`, pending token format, token/loopback checks, and old tests. Retire only the binary rejection branch. Keep `AuthorizeInstance func(context.Context,string,string) error` as a compatibility fallback and add optional `ResolveInstance func(context.Context,string,string) (string,error)` for report association.

**Repair Track:** Move module identity and artifact decision logic into `signature.go`; make the canonical storage owner `Manager`, including binary and symbol manifests. The public handler passes upstream `code_file` into a basename-only field and never uses it as a filesystem path.

**Retirement Track:** Remove `binary_upload_disabled` and the old all-`N` response. Keep the existing legacy authorization callback only for tests/embedders that do not need instance IDs; production wiring uses the resolver.

**Verification:** `go test -p 1 ./internal/crashreports -count=1`; `go test -p 1 ./internal/httpapi -run 'Crash|Accelerator' -count=1`.

- [ ] Write failing parser tests for a real v2 string containing Linux/Windows platform, architecture, two `M` records, and `F` records; assert debug file, debug identifier, module count, and malformed/oversized rejection.
- [ ] Run the focused parser tests and observe failure because `ParseSignature` and module types are absent.
- [ ] Implement `ParseSignature` with an indexed token walk: accept the version prefix `2`, parse the eight header fields, consume `M|debug_file|debug_identifier` triples, skip `F|module|offset` frame records, reject unknown record kinds and excess modules, and return platform/architecture normalized to lowercase.
- [ ] Write failing artifact tests for symbol and binary content-addressed writes, duplicate uploads, manifest identity fields, basename sanitization, symlink rejection, and cleanup of unreferenced uploaded artifacts while retaining builtin objects.
- [ ] Implement `SaveSymbol` and `SaveBinary` using `incoming` temporary files, SHA-256, `0600` files, atomic rename, JSON artifact manifests, and identifiers validated as bounded printable values. Store symbol objects under `symbols/uploaded/<hash>.sym` and binary objects under `binaries/<hash>.bin`.
- [ ] Write failing pre-submit tests for these exact decisions: existing symbol => `N`; Linux missing symbol => `Y`; Windows missing binary with binary enabled => `U`; existing binary or unknown module => `N`; response contains exactly one decision per parsed module and a presubmit token.
- [ ] Implement artifact lookup by `(platform, architecture, debug_identifier, code_identifier)` and return `Y/U/N` without trusting `debug_file` paths. Include builtin SourceMod/Metamod manifests in the lookup namespace without adding game symbols.
- [ ] Write failing HTTP tests for upstream `/binary/submit` multipart field `code_file`, successful `OK` response, content hash persistence, duplicate response, separate binary size limit, token/loopback/managed-instance rejection, and no arbitrary path creation.
- [ ] Modify `BinaryHandler` to parse one `code_file`, call `SaveBinary`, and return `OK`; reuse the existing bounded multipart parser and instance authorization.
- [ ] Extend `Report` JSON and `Receive` to persist `InstanceID`, parsed signature, module list, and artifact references. Update `ResolveInstance` callback usage while retaining `AuthorizeInstance` fallback.
- [ ] Write retention tests proving report directories and all derived files expire together, referenced binary/symbol objects remain, unreferenced uploaded objects expire after the grace period, and builtin objects remain.
- [ ] Run `go test -p 1 ./internal/crashreports -count=1` and `git diff --check`.

### Task 2: Persist Capability And Analysis Settings

**Files:**

- Modify: `internal/domain/models.go`, `internal/store/store.go`, `internal/store/migrations.go`, `internal/store/store_test.go`, `internal/config/config.go`, `internal/config/config_test.go`
- Create: `internal/store/accelerator.go`, `internal/store/accelerator_test.go`

**Why this task exists:** The capability toggle must survive Panel restarts and ordinary instance updates, while URL/proxy and analysis configuration must be global and independently editable.

**Impact / Compatibility:** Add migration version 13 with `accelerator_enabled INTEGER NOT NULL DEFAULT 0` and `auto_crash_analysis INTEGER NOT NULL DEFAULT 0`; append columns to `selectInstance`, `scanInstance`, and `fields` without changing existing JSON names. Existing rows remain disabled.

**Repair Track:** `system_settings` remains the owner of non-sensitive global settings; `secrets.Service` remains the owner of API keys. Validation is centralized in store methods, not duplicated in React.

**Retirement Track:** No environment variable becomes the persistent owner of these settings. Existing `L4D2_PANEL_CRASH_REPORT_TOKEN` and retention environment variables remain receiver bootstrap settings; Throttle and old release proxy settings are not reused as Accelerator source URL.

**Verification:** `go test -p 1 ./internal/store ./internal/config -count=1`.

- [ ] Add failing store tests that a new instance defaults both flags false, round-trips true values, and old schema rows migrate without losing package/plugin fields.
- [ ] Run those tests and confirm the migration/scan contract is missing.
- [ ] Add migration 13 and update instance SQL scan/field order; implement `domain.Instance` flags and verify `CreateInstance`, `UpdateInstance`, `Instance`, and `Instances` all round-trip them.
- [ ] Add failing settings tests for default empty download URL, `use_github_proxy=false`, empty endpoint/model, HTTPS download URL validation, and analysis endpoint validation (HTTPS or loopback HTTP only).
- [ ] Implement `AcceleratorSettings`, `CrashAnalysisSettings`, constants, and atomic upsert/get methods in `internal/store/accelerator.go`. Store booleans as `0/1` and endpoint/model as JSON or dedicated keys with strict parsing.
- [ ] Add config fields `StackwalkPath` and `PanelPort`; parse the port from `L4D2_PANEL_LISTEN`, default stackwalk path to `/usr/local/bin/minidump_stackwalk`, and allow `L4D2_PANEL_STACKWALK_PATH` override only as a plain filesystem path.
- [ ] Run focused store/config tests, then `go test -p 1 ./internal/store ./internal/config -count=1`.

### Task 3: Build The Accelerator Capability Manager

**Files:**

- Create: `internal/accelerator/archive.go`, `internal/accelerator/corecfg.go`, `internal/accelerator/manager.go`, `internal/accelerator/manifest.go`, `internal/accelerator/*_test.go`

**Why this task exists:** Accelerator is a Panel-managed capability with a separate lifecycle from ordinary plugin archives, downloads, and SourceMod Core configuration.

**Impact / Compatibility:** Only write inside `instances/<id>/game/left4dead2/addons/sourcemod` and `instances/<id>/accelerator-manifest.json`; never write to `private`, `logs`, or the crash report root. Install is transactional: failed download, archive validation, patch, or manifest write leaves the previous install intact.

**Repair Track:** `Manager.Ensure(ctx, domain.Instance)` becomes the canonical owner for enabled install and disabled removal. It receives a `DownloadURLProvider`, GitHub proxy provider, token, panel port, HTTP client, and cache root through dependency injection.

**Retirement Track:** Accelerator files are not managed by `updates.Pipeline` package manifests. Any previous package archive containing Accelerator files remains subject to the package pipeline, but the capability manifest takes ownership only of its own allowlisted paths and reports conflicts rather than silently overwriting them.

**Verification:** `go test -p 1 ./internal/accelerator -count=1`.

- [ ] Add failing download tests with `httptest.Server` for direct HTTPS URL, GitHub proxy URL transformation, non-GitHub URL unchanged, SHA-256 cache reuse, HTTP status failure, and archive byte limit.
- [ ] Implement `resolveDownloadURL` and `downloadArchive`: reject non-HTTPS URLs, apply the configured proxy only to GitHub hosts, stream into a `0600` cache temp file, hash while writing, atomically rename to `<sha256>.zip`, and extract only from the cached file.
- [ ] Add failing archive tests for valid SourceMod paths, absolute paths, `../` traversal, backslash traversal, symlink entries, unknown top-level paths, missing autoload/extension/gamedata, and architecture-specific extension mismatch.
- [ ] Implement `validateArchive` with `archive/zip`, slash-normalized relative paths, no symlink mode bits, explicit allowlisted prefixes (`addons/sourcemod/extensions`, `addons/sourcemod/gamedata`, `addons/sourcemod/plugins`, `addons/sourcemod/scripting` only when present in the package), and required Accelerator entries. Extract into a staging directory and atomically apply only after validation.
- [ ] Add failing Core KeyValues tests for preserving comments/unknown keys, updating exactly `MinidumpUrl`, `MinidumpSymbolUrl`, `MinidumpBinaryUrl`, `MinidumpPresubmit`, `MinidumpSymbolUpload`, and `MinidumpBinaryUpload`, repeated patch idempotence, and restoration of original values.
- [ ] Implement `patchCoreConfig` as a token-aware KeyValues patcher: preserve unrelated bytes and comments where possible, reject malformed nesting/duplicate managed keys, and record original/written values in the manifest. Generated URLs are `http://127.0.0.1:<port>/<endpoint>?token=<url-escaped-token>`.
- [ ] Add failing manifest tests for file hashes, Core config hash, previous values, written values, managed paths, source archive hash, and JSON atomicity. Add conflict tests for externally modified managed files or Core keys during disable.
- [ ] Implement `Ensure` and `Remove`: stage files, patch Core config, write manifest, and rename staged files only after all checks; on disable remove only files whose current hash equals the manifest hash and restore only unchanged Core keys. Return a typed conflict error with paths.
- [ ] Run `go test -p 1 ./internal/accelerator -count=1` and `go vet ./internal/accelerator`.

### Task 4: Integrate Accelerator With Lifecycle, Provisioning, And Updates

**Files:**

- Modify: `internal/lifecycle/service.go`, `internal/lifecycle/service_test.go`, `internal/provisioning/service.go`, `internal/provisioning/service_test.go`, `internal/updates/coordinator.go`, `internal/updates/coordinator_test.go`, `internal/updates/game.go`, `internal/updates/game_test.go`, `internal/updates/shared_reconciler.go`, `internal/updates/shared_game_test.go`
- Create: `internal/accelerator/integration_test.go`

**Why this task exists:** Enabling Accelerator must survive package install, plugin reinstall/update, container rebuild, and first provisioning without being mixed into plugin package ownership.

**Impact / Compatibility:** Existing operations retain their ordering and rollback behavior. Ensure failures fail the containing job before a successful state is reported; stopped instances remain stopped and desired state is never changed by Ensure.

**Repair Track:** Add the smallest `AcceleratorEnsurer` interface to each owner that already knows the instance ID. Call `Ensure` after package/private deployment commits where the game tree is available, and before `docker.BuildContainerSpec` in `Start`/`Rebuild` so the new container sees the configured files.

**Retirement Track:** Do not add a second installer to `lifecycle`, `updates`, or `httpapi`; all calls delegate to `internal/accelerator.Manager`. Do not use `Pipeline.AfterDeploy` as a global callback because it lacks the instance identity and would create an unbounded shared owner.

**Verification:** Focused lifecycle/update/provisioning tests plus `go test -p 1 ./internal/lifecycle ./internal/provisioning ./internal/updates -count=1`.

- [ ] Add failing fake-ensurer tests proving provisioning invokes Ensure after package deployment, package coordinator invokes Ensure after `ApplyPackage`, and game reinstall invokes Ensure after a full package transaction or private apply.
- [ ] Add failing lifecycle tests proving first start, stopped rebuild, and running rebuild invoke Ensure; a false flag invokes removal; an Ensure error prevents container creation/start and leaves the job failed.
- [ ] Add `WithAccelerator` options and minimal interface methods without changing existing constructors used by tests; update `cmd/panel` wiring later in Task 6.
- [ ] Run focused tests to see the new expectations fail.
- [ ] Implement hooks in `provisioning.Service`, `updates.Coordinator`, `updates.GameCoordinator`, `updates.SharedGameRebuilder`, and `lifecycle.Service` at the exact boundaries above. Preserve existing transaction rollback and `DesiredState` handling.
- [ ] Run all affected package tests and verify no ordinary package archive receives Accelerator files through the new hook.

### Task 5: Add Stackwalk, Redaction, And AI Worker

**Files:**

- Create: `internal/crashanalysis/stackwalk.go`, `internal/crashanalysis/redaction.go`, `internal/crashanalysis/ai.go`, `internal/crashanalysis/worker.go`, `internal/crashanalysis/*_test.go`
- Modify: `internal/crashreports/config.go`, `internal/crashreports/manager.go`, `internal/crashreports/artifacts.go`

**Why this task exists:** Online viewing needs a local diagnostic result, and optional AI analysis must be asynchronous and provably unable to send raw crash artifacts or secrets.

**Impact / Compatibility:** Upload success is independent of tool/AI success. Report manifests use explicit `queued`, `running`, `succeeded`, `failed`, and `unconfigured` states. Worker restart converts `running` to `queued` and re-enqueues reports from manifests.

**Repair Track:** `crashanalysis.Service` is the canonical analysis owner; `crashreports.Manager` only supplies validated local files and atomically persists results. AI input is built from parsed signature, selected metadata fields, and redacted/truncated stackwalk.

**Retirement Track:** No external crash service, Throttle request, or browser-side stackwalk is added. Raw `.dmp`, binary, symbol, token, ServerID, UserID, absolute path, IP, and unredacted command line remain local.

**Verification:** `go test -p 1 ./internal/crashanalysis ./internal/crashreports -count=1` and a fake OpenAI-compatible `httptest.Server` that inspects the complete request body.

- [ ] Add failing stackwalk tests using a temporary executable script for success, non-zero exit, timeout, missing executable, and output over `MaxStackwalkOutputBytes`; assert status/error persistence and bounded output.
- [ ] Implement `runStackwalk` with `exec.CommandContext`, fixed argument list (`dump path`, `symbol root`), no shell, a timeout context, `io.LimitReader`, and atomic `stackwalk.txt` write. Store executable basename/version and symbol coverage summary.
- [ ] Add failing redaction tests for absolute Unix/Windows paths, IPv4/IPv6 with ports, ServerID UUIDs, UserID/SteamID, token query values, command-line switches, and oversized text; assert replacements and that raw input values do not remain.
- [ ] Implement structured metadata extraction plus `Redact` for metadata and stackwalk. Hash the final AI input and store only the hash, model, timestamps, status, and bounded error in the manifest.
- [ ] Add failing AI client tests for endpoint normalization, JSON model/messages format, API key header, timeout, response size cap, one retry for 5xx/429, no retry for 4xx, and rejection of HTTP endpoints outside loopback.
- [ ] Implement `OpenAIClient` against an OpenAI-compatible chat-completions endpoint and parse only the assistant text. Treat model output as display text; never execute or interpret it as a Panel command.
- [ ] Add failing worker tests for manual enqueue, auto enqueue, duplicate coalescing, missing endpoint/model => `unconfigured` without retry, and restart recovery of queued/running manifests.
- [ ] Implement the worker queue and `Run(ctx, reportID, requestAI)` orchestration. Add a `crashreports.AnalysisStore` interface with atomic status/result methods so the analysis package does not reach into report directories.
- [ ] Run focused analysis tests and inspect an HTTP request body to prove the provided dump bytes, a test binary byte sequence, metadata token, and absolute path are absent.

### Task 6: Wire Persistence, Jobs, And HTTP APIs

**Files:**

- Create: `internal/httpapi/accelerator.go`, `internal/httpapi/accelerator_test.go`
- Modify: `internal/httpapi/server.go`, `internal/httpapi/crashreports_test.go`, `cmd/panel/main.go`, `cmd/panel/crashreports.go`, `cmd/panel/main_test.go`

**Why this task exists:** The backend must expose the new settings and report workflow while preserving existing session/job behavior.

**Impact / Compatibility:** All management routes stay behind the existing `requireAuth`, mutation lease, and audit middleware. Public receiver handlers keep their outer `ServeMux` registration. Manual analysis returns an existing persistent `Job`; auto analysis enters the analysis queue without blocking upload.

**Repair Track:** Add route-level DTOs and map domain values at the HTTP boundary. Use `artifact=<id>` for binary downloads, instance/Crash Signature/status filters for list, and explicit error codes for unavailable manager, unconfigured AI, artifact mismatch, and analysis failure.

**Retirement Track:** Remove no existing API; the old crash download route is extended to `stackwalk`, `ai`, and `binary` with the new artifact query requirement. The old `github-releases` proxy setting remains for plugin releases and is not silently repurposed.

**Verification:** `go test -p 1 ./internal/httpapi ./cmd/panel -count=1` and `go vet ./internal/httpapi ./cmd/panel`.

- [ ] Add failing API tests for `GET/PUT /api/settings/accelerator`, `GET/PUT /api/settings/crash-analysis`, API-key set/clear status without plaintext response, and validation of endpoint/download URL.
- [ ] Add failing instance API tests for create/update/list JSON flags, stopped enable without auto-start, running enable returning a reconfigure job, running disable returning a restart job, and failed Ensure propagating to job status.
- [ ] Add failing report API tests for instance/signature/status filters, detail modules/statuses, stackwalk/AI downloads, binary download requiring a manifest artifact ID, manual analyze job, 404/403/422 cases, and no filesystem path leakage.
- [ ] Implement settings handlers using store methods and `secrets.Service` under names `accelerator_ai_api_key`; never return the secret value. Register routes alongside existing settings routes.
- [ ] Extend `instanceInput` and `apply` with `accelerator_enabled` and `auto_crash_analysis`. Treat a flag change as a reconfigure job when a container exists; the job calls Ensure/Remove and restarts only a previously running instance. For an uninstalled/stopped instance, the job performs only Ensure/Remove and leaves desired state unchanged.
- [ ] Implement report filters and extended downloads. Parse `artifact` as a strict content hash/manifest ID and pass it to `Manager.OpenArtifact` after checking the report reference.
- [ ] Wire `crashanalysis.Worker`, `Accelerator.Manager`, resolver callbacks, and cleanup into `cmd/panel/main.go`; start workers after stores/secrets are ready, enqueue recoverable reports at startup, and stop workers before the job manager during shutdown.
- [ ] Inject Accelerator into lifecycle/provisioning/update owners and analysis enqueue callback into `crashreports.Manager`. Ensure the `PanelPort` used by install URLs equals the configured listener port.
- [ ] Run the focused API/cmd tests and then `go test -p 1 ./... -count=1`.

### Task 7: Build The Panel Crash Reports Experience

**Files:**

- Create: `web/src/app/CrashReportsPage.tsx`, `web/src/app/CrashReportsPage.test.tsx`
- Modify: `web/src/api/client.ts`, `web/src/app/App.tsx`, `web/src/app/App.test.tsx`, `web/src/app/InstanceConfigModal.tsx`, `web/src/app/InstanceConfigModal.test.tsx`, `web/src/styles/app.css`

**Why this task exists:** Administrators need a usable report list/detail workflow and per-instance Accelerator/auto-analysis controls, including loading, empty, failed, and retry states.

**Impact / Compatibility:** Preserve existing navigation and component props where possible. New report page uses the existing `api`, `apiBlob`, job polling, icon library, and CSS vocabulary; it does not expose raw files inline or send any browser request to an AI endpoint.

**Verification:** `cd web; npm test -- --run; npm run build` plus focused Vitest tests for the new page/modal behavior.

- [ ] Add failing `CrashReportsPage` tests for list loading, empty state, instance/signature/status filters, detail selection, metadata/stackwalk/AI sections, binary download URL with `artifact`, manual analyze action, AI unconfigured/error states, and retry.
- [ ] Implement typed crash report DTOs and download helpers in `web/src/api/client.ts`.
- [ ] Implement `CrashReportsPage` with a dense scan-friendly list and unframed detail layout: status badges, module table, redacted stackwalk display, AI output display, authenticated download buttons, and explicit error/empty/loading states.
- [ ] Add the `崩溃报告` navigation item and page title/render branch in `App.tsx`; load the page only when selected so existing overview polling remains unchanged.
- [ ] Add failing `InstanceConfigModal` tests for both toggles defaulting from API instance values and serialized into create/update payloads without changing package selection behavior.
- [ ] Extend `InstanceConfigValues`/`ConfigurableInstance`, render two accessible switch controls, and preserve the existing async job notice flow after save.
- [ ] Add settings controls for Accelerator source URL/proxy and crash-analysis endpoint/model/API-key status in the existing settings page, including validation and saving states.
- [ ] Add focused CSS for compact report rows, module tables, preformatted diagnostics, responsive detail layout, and switch controls; verify text stays within parent containers at desktop and mobile widths.
- [ ] Run all frontend tests/build and inspect the main report workflow with a local dev server or Playwright if the environment supports it.

### Task 8: Deployment, Built-In Symbol Records, And End-To-End Evidence

**Files:**

- Create: `assets/crash-symbols/README.md`, `assets/crash-symbols/manifest.json`
- Modify: `.env.example`, `docker-compose.yml`, `README.md`, `docs/aegis/INDEX.md`
- Test: `deployment_test.go`, `internal/httpapi/crashreports_test.go`, `cmd/panel/crashreports_test.go`

**Why this task exists:** Operators need explicit defaults and a repeatable evidence path for the supplied dump without accidentally distributing Valve/L4D2 symbols or weakening the loopback boundary.

**Impact / Compatibility:** Keep host network, `NO_PROXY=localhost,127.0.0.1`, data-root mount, cap drop, and existing healthcheck. Add only stackwalk path, accelerator source/proxy, and AI endpoint/model environment bootstrap values that are safe to expose; API keys stay in encrypted secrets.

**Repair Track:** Document Accelerator as Panel-managed and independent of normal plugin packages, document `MinidumpBinaryUpload=yes`, and make the no-Throttle behavior explicit in deployment docs and tests.

**Retirement Track:** Remove README statements claiming `/binary/submit` is intentionally forbidden. Retain the older receiver details as the lower-level protocol reference, updating its compatibility section to link to this plan's implementation result only after code lands.

**Verification:** `go test -p 1 ./... -count=1`, `go vet ./...`, `npm test -- --run`, `npm run build`, `docker compose config --quiet`, and `git diff --check`.

- [ ] Add failing deployment assertions for host network, no published ports, loopback healthcheck, crash token/retention defaults, stackwalk path, and no Throttle URL.
- [ ] Implement environment/docs updates with default retention 90 and explicit binary upload enabled in the generated Core configuration example.
- [ ] Search official SourceMod and Metamod build artifacts for redistributable symbol sources; add only SourceMod/Metamod entries with source URL, version, license, and SHA-256 to `assets/crash-symbols/manifest.json`. Do not add L4D2/Valve paths or files. When an artifact is not redistributable as a symbol bundle, record the public source/build URL and keep it as an operator-fetchable source rather than pretending it is built in.
- [ ] Use the provided dump in a real local multipart receiver test: assert `MDMP` acceptance, report ID equals dump SHA-256 `D98B6E8354FD791712748ED6B15FF55D052678ED7BDF493F78F45A58EAD1924E`, duplicate upload is idempotent, authenticated download bytes match, and no dump is copied into the repository.
- [ ] Run the complete verification bundle, inspect `git status --short`, and confirm only intended files changed; leave remote instance restart/rebuild as an explicit manual operation.

## Rollback Surface

- Capability installation is isolated to the per-instance Accelerator manifest and allowlisted SourceMod paths. A failed Ensure removes only its staging directory; a disable conflict leaves the externally modified file/value untouched.
- SQLite migration 13 is additive with disabled defaults. Reverting code leaves new columns/settings harmless; rollback of an enabled instance requires running the capability Remove job before code removal.
- Crash report cleanup is constrained to `panel/crash-dumps`; it does not touch instance game/private/log directories.
- Analysis results and worker status are derived files/manifest fields. Removing the stackwalk tool or AI endpoint produces visible failures, not upload failures.

## Plan Self-Review

- Spec coverage: goals, source URL/proxy, instance lifecycle, Core config keys, full three-endpoint protocol, artifacts, 90-day derived-file retention, stackwalk, AI redaction, APIs, frontend states, built-in symbol scope, and deployment evidence each have a named task.
- Self-review for incomplete steps: task steps contain concrete paths, contracts, commands, and expected outcomes rather than empty implementation slots.
- Type consistency: `domain.Instance` flags, store settings, `crashreports.Module`, `Manager.SaveBinary`, `Manager.OpenArtifact`, `crashanalysis.Worker`, and HTTP DTOs are defined before their consumers.
- Compatibility check: the old receiver response and auth boundary are preserved; only binary rejection and all-`N` decisions retire.
- Verification check: each major task has focused commands and the final bundle covers backend, frontend, deployment, supplied dump, and diff hygiene.
- Dual-track check: ordinary plugin package ownership, Throttle fallback, browser-side parsing, and game symbol distribution are explicitly retired or excluded.

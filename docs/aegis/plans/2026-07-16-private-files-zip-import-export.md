# Private Files ZIP Import and Export Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add secure ZIP export and atomic full-replacement ZIP import to the selected instance's private-files workspace without automatically applying it to the game directory.

**Architecture:** Add a focused `PrivateManager` archive module that validates and stages ZIPs under the existing instance lock, then publishes them through the same journaled workspace-replacement transaction used by snapshot restore. Expose exact GET/POST archive routes, then add toolbar controls that state the destructive replacement behavior and reload the staged diff after import.

**Tech Stack:** Go 1.25, `archive/zip`, Chi HTTP, React 19, TypeScript, Vitest/Testing Library, Playwright.

**Baseline / Authority Refs:** `docs/aegis/specs/2026-07-16-private-files-zip-import-export-design.md`, `docs/aegis/specs/2026-07-15-private-file-manager-console-follow-design.md`, `CONTEXT.md`, `internal/content/private.go`, `internal/content/private_state.go`, `internal/archive/inspect.go`, `internal/httpapi/server.go`, `web/src/app/PrivateFilesPage.tsx`.

**Compatibility Boundary:** Existing single-file CRUD, resumable upload, applied manifest, snapshots, diff, apply Job, lower-layer restoration and private-path safety remain unchanged. Import replaces only `instances/<id>/private/`, preserves the archive's sole top-level directory, leaves snapshots and the applied manifest intact, and never applies automatically.

**Verification:** `go test ./internal/content ./internal/httpapi -count=1`, `go test ./... -count=1`, `cd web && npm test -- --run`, `cd web && npm run build`, `cd web && npm run e2e -- --project=desktop`, and `cd web && npm run e2e -- --project=mobile`.

---

## Scope Check

Facts: `PrivateManager` is the canonical workspace owner and all managers share the same per-instance `RWMutex`; snapshot restore already provides a journaled directory replacement and startup recovery; the HTTP mutation audit is middleware-owned; the UI already stages changes and uses `busy` plus `reload` for consistent async behavior.

Assumptions: use a 2 GiB compressed upload limit to align with the existing private upload ceiling, a 4 GiB expanded limit, 2 GiB per file, 10,000 entries and a 400:1 maximum compression ratio. These are implementation constants rather than new settings UI.

Bounded unknown: Windows may again produce the documented temporary-file lock flake. Exact commands remain authoritative; a serial rerun with a dedicated `GOTMPDIR` may diagnose that host condition but cannot replace the exact verification record.

Repair track: extract the existing snapshot restore publication block into the single canonical workspace replacement helper before adding ZIP import. Retirement track: the inline replacement sequence in `RestoreSnapshot` retires; its journal format, recovery scanner and public behavior remain active and are reused rather than duplicated.

## File Ownership Map

- Create `internal/content/private_archive.go`: ZIP limits, stable archive errors, export, validation, extraction and import orchestration.
- Create `internal/content/private_archive_test.go`: archive round trip, replacement, security, limits, rollback and metadata invariants.
- Modify `internal/content/private_state.go`: extract the existing journaled workspace publication into one locked helper used by restore and import.
- Modify `internal/content/private_test.go`: preserve snapshot-restore transaction and recovery behavior after extraction.
- Modify `internal/httpapi/server.go`: archive routes, download spooling, media/confirmation checks and stable error mapping.
- Modify `internal/httpapi/server_test.go`: authenticated archive contract, full replacement, download headers, limits, audit and missing-instance behavior.
- Modify `web/src/api/client.ts`: add authenticated blob response support using the existing request/error parser.
- Modify `web/src/app/PrivateFilesPage.tsx`: import/export controls, warning, status and stale selection cleanup.
- Modify `web/src/app/PrivateFilesPage.test.tsx`: destructive warning, cancel, import reload/no auto-apply, export and instance ownership.
- Modify `web/e2e/control-panel.spec.ts`: real HTTP export/import journey on desktop and mobile.
- Modify `docs/aegis/work/2026-07-16-private-files-zip-import-export/40-atomic-tasks.md`: live checkbox state.
- Modify `docs/aegis/work/2026-07-16-private-files-zip-import-export/50-evidence.md`: final command evidence and residual risk.

### Task 1: Canonical Workspace Replacement Transaction

**Files:**
- Modify: `internal/content/private_state.go:1330`
- Test: `internal/content/private_test.go`

**Why this task exists:**
- ZIP import and snapshot restore must not become two owners of destructive workspace replacement.
- Existing crash recovery and rollback guarantees must cover both entry points.

**Impact / Compatibility:**
- Only private content internals change; journal version, `restore-*` path validation, failure hooks and `RestoreSnapshot` behavior remain stable.

**Verification:**
- `go test ./internal/content -run 'TestPrivateSnapshot|TestPrivateRestore' -count=1`

- [ ] **Step 1: Add a regression test that exercises restore failure after the publication helper extraction**

```go
func TestPrivateSnapshotRestoreStillPreservesWorkspaceOnJournalFailure(t *testing.T) {
	root := t.TempDir()
	manager := NewPrivateManager(root, 1<<20)
	_, _ = manager.Save(context.Background(), "abc", "cfg/value.cfg", []byte("snapshot"))
	if err := manager.ApplyChanges(context.Background(), "abc"); err != nil { t.Fatal(err) }
	snapshots, err := manager.Snapshots(context.Background(), "abc")
	if err != nil || len(snapshots) != 1 { t.Fatalf("snapshots=%v err=%v", snapshots, err) }
	_, _ = manager.Save(context.Background(), "abc", "cfg/value.cfg", []byte("current"))
	setPrivateRestoreFailureHook(func(stage string) error {
		if stage == "journal" { return errors.New("journal fault") }
		return nil
	})
	t.Cleanup(func() { setPrivateRestoreFailureHook(nil) })
	if err = manager.RestoreSnapshot(context.Background(), "abc", snapshots[0].ID); err == nil {
		t.Fatal("restore unexpectedly succeeded")
	}
	raw, readErr := manager.Read(context.Background(), "abc", "cfg/value.cfg")
	if readErr != nil || string(raw) != "current" { t.Fatalf("raw=%q err=%v", raw, readErr) }
}
```

- [ ] **Step 2: Run the regression and confirm the current behavior is green before refactoring**

Run: `go test ./internal/content -run 'TestPrivateSnapshotRestoreStillPreservesWorkspaceOnJournalFailure' -count=1`

Expected: PASS, proving the refactor starts from an observed contract.

- [ ] **Step 3: Extract the publication owner and route snapshot restore through it**

```go
func (m *PrivateManager) replacePrivateWorkspaceLocked(instanceID, base, work, staging string) (err error) {
	preJournal := true
	defer func() {
		if preJournal { m.cleanupPrivate(base, "restore-prejournal", func() error { return os.RemoveAll(work) }) }
	}()
	workspace := filepath.Join(base, "private")
	backup := filepath.Join(work, "old")
	hadOld := false
	if _, err = os.Stat(workspace); err == nil { hadOld = true } else if !errors.Is(err, os.ErrNotExist) { return err }
	journal := privateRestoreJournal{Version: 1, InstanceID: instanceID, Stage: "prepared", HadOld: hadOld}
	journalPath := filepath.Join(work, "journal.json")
	if err = runPrivateRestoreFailureHook("journal"); err != nil { return err }
	if err = writeJSONAtomic(journalPath, journal); err != nil { return err }
	preJournal = false
	fail := func(cause error) error { return errors.Join(cause, m.rollbackPrivateRestore(work, base, journal)) }
	journal.Stage = "old_moved"
	if err = writeJSONAtomic(journalPath, journal); err != nil { return fail(err) }
	if hadOld { if err = os.Rename(workspace, backup); err != nil { return fail(err) } }
	if err = os.Rename(staging, workspace); err != nil { return fail(err) }
	journal.Stage = "published"
	if err = writeJSONAtomic(journalPath, journal); err != nil { return fail(err) }
	journal.Stage = "committed"
	if err = writeJSONAtomic(journalPath, journal); err != nil { return fail(err) }
	m.cleanupPrivate(base, "restore-work", func() error { return os.RemoveAll(work) })
	return nil
}
```

Remove the duplicated publication block from `RestoreSnapshot`; after `copyPrivateRestoreTree(source, staging)` call `return m.replacePrivateWorkspaceLocked(instanceID, base, work, staging)`.

- [ ] **Step 4: Run focused and full content tests**

Run: `go test ./internal/content -run 'TestPrivateSnapshot|TestPrivateRestore' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the transaction refactor**

```text
git add internal/content/private_state.go internal/content/private_test.go
git commit -m "refactor(private): 统一工作区替换事务"
```

### Task 2: ZIP Archive Content Owner

**Files:**
- Create: `internal/content/private_archive.go`
- Create: `internal/content/private_archive_test.go`
- Modify: `internal/content/private_state.go`

**Why this task exists:**
- The server needs one safe, testable owner for exact-path ZIP export and all-or-nothing workspace replacement.
- Archive validation must complete before the current tree is switched.

**Impact / Compatibility:**
- Import mutates only the staged workspace; it does not write the game directory, applied manifest or snapshots.
- Top-level archive paths are preserved exactly; no common-root stripping is reused from package deployment.

**Verification:**
- `go test ./internal/content -run 'TestPrivateZIP' -count=1`

- [ ] **Step 1: Write failing round-trip and replacement tests**

```go
func TestPrivateZIPImportReplacesWorkspaceAndPreservesMetadata(t *testing.T) {
	root := t.TempDir()
	m := NewPrivateManager(root, 1<<20)
	_, _ = m.Save(context.Background(), "abc", "old/keep.cfg", []byte("old"))
	_ = m.MakeDir(context.Background(), "abc", "old-empty")
	if err := m.ApplyChanges(context.Background(), "abc"); err != nil { t.Fatal(err) }
	before, _ := m.Snapshots(context.Background(), "abc")
	raw := privateZIP(t, map[string][]byte{"bundle/cfg/new.cfg": []byte("new")}, []string{"bundle/empty"})
	if err := m.ImportZIP(context.Background(), "abc", bytes.NewReader(raw), DefaultPrivateArchiveLimits); err != nil { t.Fatal(err) }
	entries, err := m.Tree(context.Background(), "abc")
	if err != nil { t.Fatal(err) }
	assertPrivatePaths(t, entries, "bundle", "bundle/cfg", "bundle/cfg/new.cfg", "bundle/empty")
	after, _ := m.Snapshots(context.Background(), "abc")
	if !reflect.DeepEqual(before, after) { t.Fatalf("snapshots changed: before=%v after=%v", before, after) }
	diff, err := m.Diff(context.Background(), "abc")
	if err != nil || diff.Summary.Added != 2 || diff.Summary.Deleted != 2 { t.Fatalf("diff=%+v err=%v", diff, err) }
}

func TestPrivateZIPExportRoundTripsFilesAndEmptyDirectories(t *testing.T) {
	root := t.TempDir()
	m := NewPrivateManager(root, 1<<20)
	_, _ = m.Save(context.Background(), "abc", "top/data.bin", []byte{0, 1, 2})
	_ = m.MakeDir(context.Background(), "abc", "top/empty")
	var archive bytes.Buffer
	if err := m.ExportZIP(context.Background(), "abc", &archive); err != nil { t.Fatal(err) }
	if err := m.ImportZIP(context.Background(), "def", bytes.NewReader(archive.Bytes()), DefaultPrivateArchiveLimits); err != nil { t.Fatal(err) }
	raw, err := m.Read(context.Background(), "def", "top/data.bin")
	if err != nil || !bytes.Equal(raw, []byte{0, 1, 2}) { t.Fatalf("raw=%v err=%v", raw, err) }
	entries, _ := m.Tree(context.Background(), "def")
	assertPrivatePaths(t, entries, "top", "top/data.bin", "top/empty")
}

func privateZIP(t *testing.T, files map[string][]byte, directories []string) []byte {
	t.Helper()
	var raw bytes.Buffer
	w := zip.NewWriter(&raw)
	for _, directory := range directories {
		if _, err := w.Create(strings.TrimSuffix(directory, "/") + "/"); err != nil { t.Fatal(err) }
	}
	for name, body := range files {
		entry, err := w.Create(name)
		if err != nil { t.Fatal(err) }
		if _, err = entry.Write(body); err != nil { t.Fatal(err) }
	}
	if err := w.Close(); err != nil { t.Fatal(err) }
	return raw.Bytes()
}

func assertPrivatePaths(t *testing.T, entries []PrivateEntry, want ...string) {
	t.Helper()
	got := make([]string, len(entries))
	for index, entry := range entries { got[index] = entry.Path }
	if !reflect.DeepEqual(got, want) { t.Fatalf("paths=%v want=%v", got, want) }
}
```

- [ ] **Step 2: Write failing table tests for invalid and limited ZIPs**

Cover encrypted entries, symlinks, `../`, absolute/drive paths, backslashes, duplicate normalized paths, case-fold collisions, file-as-parent conflicts, file-count, single-file, expanded-size and compression-ratio limits. Each case calls `ImportZIP`, asserts the matching stable sentinel (`ErrPrivateArchiveInvalid`, `ErrPrivateArchivePath`, `ErrPrivateArchiveConflict`, `ErrPrivateArchiveUnsupported` or `ErrPrivateArchiveTooLarge`), then verifies a sentinel old file is unchanged.

```go
for _, test := range tests {
	t.Run(test.name, func(t *testing.T) {
		root := t.TempDir()
		m := NewPrivateManager(root, 1<<20)
		_, _ = m.Save(context.Background(), "abc", "sentinel.cfg", []byte("before"))
		err := m.ImportZIP(context.Background(), "abc", bytes.NewReader(test.raw), test.limits)
		if !errors.Is(err, test.want) { t.Fatalf("err=%v want=%v", err, test.want) }
		raw, readErr := m.Read(context.Background(), "abc", "sentinel.cfg")
		if readErr != nil || string(raw) != "before" { t.Fatalf("sentinel=%q err=%v", raw, readErr) }
	})
}
```

- [ ] **Step 3: Run tests and verify RED**

Run: `go test ./internal/content -run 'TestPrivateZIP' -count=1`

Expected: FAIL because `ImportZIP`, `ExportZIP`, limits and stable errors do not exist.

- [ ] **Step 4: Implement archive limits, stable errors and export**

```go
type PrivateArchiveLimits struct {
	MaxCompressedBytes int64
	MaxExpandedBytes uint64
	MaxFileBytes uint64
	MaxFiles int
	MaxCompressionRatio float64
}

var DefaultPrivateArchiveLimits = PrivateArchiveLimits{
	MaxCompressedBytes: 2 << 30,
	MaxExpandedBytes: 4 << 30,
	MaxFileBytes: 2 << 30,
	MaxFiles: 10_000,
	MaxCompressionRatio: 400,
}

var ErrPrivateArchiveInvalid = errors.New("invalid private ZIP archive")
var ErrPrivateArchivePath = errors.New("invalid private ZIP path")
var ErrPrivateArchiveConflict = errors.New("conflicting private ZIP paths")
var ErrPrivateArchiveUnsupported = errors.New("unsupported private ZIP entry")
var ErrPrivateArchiveTooLarge = errors.New("private ZIP archive exceeds limits")

func (m *PrivateManager) ExportZIP(ctx context.Context, instanceID string, output io.Writer) error
func (m *PrivateManager) ImportZIP(ctx context.Context, instanceID string, input io.Reader, limits PrivateArchiveLimits) error
```

`ExportZIP` must take the instance lock, recover and establish the baseline, scan and sort the workspace, emit directory headers with trailing `/`, emit regular files without a common-root transform, check `ctx.Err()` between entries and close the ZIP writer without closing `output`.

- [ ] **Step 5: Implement preflight, extraction and journaled replacement**

Spool at most `MaxCompressedBytes + 1` bytes into `restore-<uuid>/archive.zip`. Open it with `zip.NewReader`, preflight every header before extraction, and build a case-folded node map that distinguishes explicit entries from implicit parents. Accept only regular files/directories, preserve explicit empty directories, verify compressed ratio and cumulative sizes, use `safepath.Join` for every target, copy each file with an exact-size limit, sync files/directories, rescan the staged tree, then call `replacePrivateWorkspaceLocked`.

Malformed ZIP containers wrap `ErrPrivateArchiveInvalid`; unsafe path syntax wraps `ErrPrivateArchivePath`; duplicate, case-folded or file/directory collisions wrap `ErrPrivateArchiveConflict`; encryption, symlinks and special entries wrap `ErrPrivateArchiveUnsupported`; configured limits wrap `ErrPrivateArchiveTooLarge`. Raw host paths stay out of these public error messages. On any pre-journal failure remove the work directory. Do not call `writePrivateManifest`, `createPrivateSnapshot` or game deployment code.

- [ ] **Step 6: Run archive, content and race-oriented serialization tests**

Run: `go test ./internal/content -run 'TestPrivateZIP|TestPrivateSnapshot|TestPrivateRestore' -count=1`

Expected: PASS.

- [ ] **Step 7: Commit the archive owner**

```text
git add internal/content/private_archive.go internal/content/private_archive_test.go internal/content/private_state.go
git commit -m "feat(private): 支持工作区 ZIP 原子替换与导出"
```

### Task 3: Authenticated HTTP Archive Contract

**Files:**
- Modify: `internal/httpapi/server.go:177`
- Modify: `internal/httpapi/server_test.go:106`

**Why this task exists:**
- The browser needs exact endpoints that enforce destructive confirmation and do not expose partial downloads or host paths.

**Impact / Compatibility:**
- Existing private routes and mutation auditing stay active. GET remains read-only; POST is audited by existing middleware.

**Verification:**
- `go test ./internal/httpapi -run 'TestPrivateFileAPIContract|TestPrivateArchiveAPI' -count=1`

- [ ] **Step 1: Add failing HTTP contract tests**

```go
func TestPrivateArchiveAPI(t *testing.T) {
	s, db := testServer(t)
	t.Cleanup(func() { _ = db.Close() })
	_ = db.CreateInstance(context.Background(), domain.Instance{ID: "abc", NodeID: "local", Name: "abc"})
	root := t.TempDir()
	private := content.NewPrivateManager(root, 1<<20)
	s = New(db, s.auth, WithContent(nil, private, nil, nil, nil))
	cookie := loginCookie(t, s)
	_, _ = private.Save(context.Background(), "abc", "old.cfg", []byte("old"))

	request := func(method, target, mediaType string, body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, bytes.NewReader(body))
		r.AddCookie(cookie)
		if mediaType != "" { r.Header.Set("Content-Type", mediaType) }
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}

	if w := request(http.MethodPost, "/api/instances/abc/private/archive", "application/zip", []byte("not read")); w.Code != 428 { t.Fatalf("confirmation=%d %s", w.Code, w.Body.String()) }
	if w := request(http.MethodPost, "/api/instances/abc/private/archive?confirm=true", "text/plain", []byte("bad")); w.Code != 415 { t.Fatalf("media=%d %s", w.Code, w.Body.String()) }
	archive := httpPrivateZIP(t, map[string]string{"folder/new.cfg": "new"})
	if w := request(http.MethodPost, "/api/instances/abc/private/archive?confirm=true", "application/zip", archive); w.Code != 204 { t.Fatalf("import=%d %s", w.Code, w.Body.String()) }
	if _, err := private.Read(context.Background(), "abc", "old.cfg"); !errors.Is(err, os.ErrNotExist) { t.Fatalf("old file retained: %v", err) }
	w := request(http.MethodGet, "/api/instances/abc/private/archive", "", nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/zip" || !strings.Contains(w.Header().Get("Content-Disposition"), "private-files-abc.zip") { t.Fatalf("export=%d headers=%v", w.Code, w.Header()) }
}

func httpPrivateZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var raw bytes.Buffer
	w := zip.NewWriter(&raw)
	for name, body := range files {
		entry, err := w.Create(name)
		if err != nil { t.Fatal(err) }
		if _, err = io.WriteString(entry, body); err != nil { t.Fatal(err) }
	}
	if err := w.Close(); err != nil { t.Fatal(err) }
	return raw.Bytes()
}
```

Add GET and POST archive paths to the missing-instance table in `TestPrivateFileAPIContract`, and assert the successful POST audit target/result.

- [ ] **Step 2: Run HTTP tests and verify RED**

Run: `go test ./internal/httpapi -run 'TestPrivateFileAPIContract|TestPrivateArchiveAPI' -count=1`

Expected: FAIL with archive routes returning 404.

- [ ] **Step 3: Register routes and handlers**

```go
r.Get("/api/instances/{id}/private/archive", s.exportPrivateArchive)
r.Post("/api/instances/{id}/private/archive", s.importPrivateArchive)
```

`exportPrivateArchive` creates a temporary `.zip`, calls `s.private.ExportZIP`, seeks to the start, then sets `Content-Type: application/zip` and `Content-Disposition: attachment; filename="private-files-<id>.zip"` before `http.ServeContent`. No response header is committed before archive creation succeeds.

`importPrivateArchive` requires `confirm=true`, accepts only `application/zip`, wraps the body with `http.MaxBytesReader` using `DefaultPrivateArchiveLimits.MaxCompressedBytes`, and calls `ImportZIP`. Map limits to `413 archive_too_large`; malformed containers to `422 invalid_private_archive`; unsafe paths to `422 invalid_archive_path`; collisions to `409 archive_path_conflict`; unsupported entries to `422 unsupported_archive_entry`; and filesystem failures to generic `500 private_archive_error`. Return concise Chinese messages such as “ZIP 超出允许的大小或压缩限制” and “ZIP 导入失败，原工作区未更改” without host paths, then return `204` on success.

- [ ] **Step 4: Run HTTP and content suites**

Run: `go test ./internal/content ./internal/httpapi -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the HTTP contract**

```text
git add internal/httpapi/server.go internal/httpapi/server_test.go
git commit -m "feat(api): 提供私有文件 ZIP 导入导出接口"
```

### Task 4: Private Files Toolbar Experience

**Files:**
- Modify: `web/src/api/client.ts`
- Modify: `web/src/app/PrivateFilesPage.tsx:378`
- Modify: `web/src/app/PrivateFilesPage.test.tsx`

**Why this task exists:**
- Administrators need obvious import/export commands and an unambiguous warning that import deletes workspace content not present in the ZIP.

**Impact / Compatibility:**
- Single-file upload remains labeled “上传文件”. ZIP import is separate, clears stale selection/editor/upload recovery state only after success, and never queues `/private/apply`.

**Verification:**
- `cd web && npm test -- --run src/app/PrivateFilesPage.test.tsx && npm run build`

- [ ] **Step 1: Write failing import/export interaction tests**

```tsx
it("explains and performs a full workspace ZIP replacement without applying", async () => {
  const { calls } = mockPrivateAPI();
  const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
  render(<PrivateFilesPage instances={[instance]} queue={vi.fn()} />);
  const file = new File([new Uint8Array([80, 75, 3, 4])], "replacement.zip", { type: "application/zip" });
  fireEvent.change(screen.getByLabelText("导入 ZIP"), { target: { files: [file] } });
  await waitFor(() => expect(confirm).toHaveBeenCalledWith(expect.stringContaining("ZIP 中不存在的现有文件和未应用更改将被删除，不会保留")));
  expect(confirm.mock.calls[0][0]).toContain("历史应用快照不受影响");
  expect(confirm.mock.calls[0][0]).toContain("不会自动应用到游戏目录");
  expect(await screen.findByText("工作区已完全替换，请检查差异后应用更改。")).toBeVisible();
  expect(calls.some(({ path, init }) => path.endsWith("/private/archive?confirm=true") && init?.method === "POST")).toBe(true);
  expect(calls.some(({ path }) => path.endsWith("/private/apply"))).toBe(false);
});

it("cancels ZIP import before sending a request", async () => {
  const { calls } = mockPrivateAPI();
  vi.spyOn(window, "confirm").mockReturnValue(false);
  render(<PrivateFilesPage instances={[instance]} queue={vi.fn()} />);
  fireEvent.change(screen.getByLabelText("导入 ZIP"), { target: { files: [new File(["zip"], "cancel.zip")] } });
  await Promise.resolve();
  expect(calls.some(({ path }) => path.includes("/private/archive"))).toBe(false);
});
```

Add an export test that stubs `URL.createObjectURL`, spies on `HTMLAnchorElement.prototype.click`, clicks “导出 ZIP”, verifies GET `/api/instances/abc/private/archive`, download name `private-files-abc.zip`, and URL revocation. Add a two-instance test proving the captured instance ID owns an import even if selection changes before completion.

- [ ] **Step 2: Run the page tests and verify RED**

Run: `cd web && npm test -- --run src/app/PrivateFilesPage.test.tsx`

Expected: FAIL because the import/export controls do not exist.

- [ ] **Step 3: Add blob support and toolbar operations**

```ts
export async function apiBlob(path: string, init: RequestInit = {}): Promise<Blob> {
  const response = await request(path, init);
  return response.blob();
}
```

```tsx
const importZIP = async (event: ChangeEvent<HTMLInputElement>) => {
  const file = event.target.files?.[0];
  event.target.value = "";
  if (!file) return;
  const owner = instanceID;
  const ownerName = instances.find((item) => item.id === owner)?.name || owner;
  const warning = `将 ${file.name} 导入 ${ownerName}。\n\n导入会完全替换当前私有文件工作区。ZIP 中不存在的现有文件和未应用更改将被删除，不会保留；历史应用快照不受影响。导入后不会自动应用到游戏目录。`;
  if (!window.confirm(warning)) return;
  await run(async () => {
    await api<void>(`/api/instances/${owner}/private/archive?confirm=true`, { method: "POST", headers: { "Content-Type": "application/zip" }, body: file });
    if (owner !== instanceIDRef.current) return;
    setSelectedPath("");
    setEditor("");
    setEditing(false);
    setActiveUpload(null);
    setStatus("工作区已完全替换，请检查差异后应用更改。");
    await reload();
  });
};
```

`exportZIP` captures the current instance, calls `apiBlob`, creates/revokes an object URL, clicks a temporary anchor named `private-files-<id>.zip`, and reports completion only if the same instance remains selected. Add icon-and-text toolbar controls using Lucide `FileArchive` and `Download`; the import input uses `accept=".zip,application/zip"` and both controls honor `busy` and empty instance state.

- [ ] **Step 4: Run page tests and production build**

Run: `cd web && npm test -- --run src/app/PrivateFilesPage.test.tsx && npm run build`

Expected: PASS with no TypeScript errors.

- [ ] **Step 5: Commit the toolbar experience**

```text
git add web/src/api/client.ts web/src/app/PrivateFilesPage.tsx web/src/app/PrivateFilesPage.test.tsx
git commit -m "feat(web): 增加私有文件 ZIP 导入导出"
```

### Task 5: Real HTTP Acceptance and Evidence

**Files:**
- Modify: `web/e2e/control-panel.spec.ts`
- Modify: `docs/aegis/work/2026-07-16-private-files-zip-import-export/40-atomic-tasks.md`
- Modify: `docs/aegis/work/2026-07-16-private-files-zip-import-export/50-evidence.md`

**Why this task exists:**
- Unit tests cannot prove the browser warning, binary download, server extraction and staged-diff refresh work together on desktop and mobile.

**Impact / Compatibility:**
- Extend the existing administration journey without weakening its private apply, snapshot restore, layout or console-follow assertions.

**Verification:**
- Full matrix in the plan header.

- [ ] **Step 1: Extend Playwright with export and replacement import**

After the existing restored private workspace is applied, download “导出 ZIP” and assert a non-empty `private-files-<id>.zip`. Import a fixed valid ZIP containing `imported/new.cfg`, accept the destructive warning, then assert the tree contains `imported/new.cfg`, no longer contains the prior `cfg/seeded.cfg` or `cfg/binary.bin`, and the status bar reports unapplied changes. Query `/__e2e/private-lower` to prove the previously applied `cfg/seeded.cfg` remains in the game directory until “应用更改” is explicitly clicked.

- [ ] **Step 2: Run focused and full verification**

Run sequentially:

```text
go test ./internal/content ./internal/httpapi -count=1
go test ./... -count=1
cd web
npm test -- --run
npm run build
npm run e2e -- --project=desktop
npm run e2e -- --project=mobile
```

Expected: all commands exit 0. Record exact pass counts, durations, any known Windows lock retry and the fact that desktop/mobile were run sequentially because they share fixture port `127.0.0.1:18082`.

- [ ] **Step 3: Audit ownership, scope and warning text**

Run:

```text
rg -n "ImportZIP|ExportZIP|replacePrivateWorkspaceLocked" internal
rg -n "完全替换当前私有文件工作区|不会自动应用到游戏目录" web/src web/e2e
rg -n "ApplyChanges|/private/apply" internal/content/private_archive.go web/src/app/PrivateFilesPage.tsx
git diff --check
git status --short
```

Expected: one content owner for ZIP import/export, one workspace publication helper, warning text present in UI/tests, no import call to apply, no whitespace errors and only task documentation changes before the final evidence commit.

- [ ] **Step 4: Record evidence and commit acceptance coverage**

```text
git add web/e2e/control-panel.spec.ts docs/aegis/work/2026-07-16-private-files-zip-import-export/40-atomic-tasks.md docs/aegis/work/2026-07-16-private-files-zip-import-export/50-evidence.md
git commit -m "test(private-files): 验证 ZIP 完整替换导入流程"
```

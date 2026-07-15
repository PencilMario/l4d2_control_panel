# Private File Manager and Console Follow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a complete, staged `private/` file manager and make the native console follow new output only while the administrator remains at the bottom.

**Architecture:** Extend the filesystem-backed `PrivateManager` into the sole owner of private workspace metadata, applied manifests, lower-layer copies, journals and 20 retained snapshots. Keep package deployment in `updates.Pipeline`, but replace its blind private-tree copy with a rebase call that captures the newly deployed lower layer before replaying private files. Add a focused React private-files page and a small tested console-follow hook instead of growing the existing `ContentPage` and `Terminal` further.

**Tech Stack:** Go 1.24, Chi HTTP API, filesystem JSON manifests and atomic renames, existing persistent Job manager, React 19, TypeScript, Vitest/Testing Library, Playwright.

**Baseline / Authority Refs:** `docs/aegis/specs/2026-07-15-private-file-manager-console-follow-design.md`, `docs/aegis/specs/2026-07-14-l4d2-control-panel-design.md`, `internal/content/private.go`, `internal/content/overlay.go`, `internal/updates/pipeline.go`, `internal/httpapi/server.go`, `web/src/app/App.tsx`.

**Compatibility Boundary:** Preserve `instances/<id>/private/`, overlay order `plugin < shared VPK < private`, per-instance Job serialization, update rollback, existing authenticated API conventions, PTY WebSocket protocol and all unrelated content flows. Never expose `game/`, accept symlinks or execute uploaded content.

**Verification:** `go test ./internal/content ./internal/updates ./internal/httpapi`, `go test ./...`, `npm test -- --run`, `npm run build`, `npm run e2e -- --project=desktop`, and `npm run e2e -- --project=mobile`.

---

## Scope check and file ownership

Facts: private files already live on disk and API mutations are audited; `jobs.Manager` already serializes by instance; package deployment already journals game-file changes. Assumption: the existing upload limit configuration can supply a private-upload limit; until it is separately configurable, use the current server-side content limit rather than inventing settings UI. Unknown requiring implementation-time verification: shared-VPK reconciliation is not currently represented in `updates.Pipeline`; tests must identify its canonical owner before wiring `RebaseAndApply`, and the call must occur after every lower-layer writer.

Files and responsibilities:

- `internal/content/private.go`: workspace CRUD, safe paths and compatibility wrappers.
- `internal/content/private_state.go`: manifests, diffs, lower-layer cache, journals, snapshots and apply/rebase transactions.
- `internal/content/private_uploads.go`: resumable private-file upload sessions.
- `internal/updates/pipeline.go`: call the private rebase owner after lower layers deploy; retire blind `PrivateManager.Apply` use.
- `internal/httpapi/server.go`: authenticated file-manager and apply/history contracts.
- `web/src/app/PrivateFilesPage.tsx`: independent private-files Tab and all file interactions.
- `web/src/app/useConsoleFollow.ts`: scroll-follow state machine.
- `web/src/app/App.tsx`: navigation/wiring only; retire the embedded private form and inline follow logic.
- matching Go, Vitest and Playwright tests prove every slice before implementation.

### Task 1: Safe workspace tree and staged diffs

**Files:**
- Modify: `internal/content/private.go`
- Create: `internal/content/private_state.go`
- Modify: `internal/content/private_test.go`

**Why this task exists:** Administrators need directories and complete file operations without touching the game tree, plus a truthful count of unapplied changes.

**Impact / Compatibility:** Keep `Save`, `Read`, `List`, `History` and `Delete` callable until HTTP migration completes. All new operations remain rooted under `private/`; empty directories become first-class entries.

**Verification:** `go test ./internal/content -run 'TestPrivate(Workspace|Diff|Path)' -count=1`

- [ ] **Step 1: Write failing workspace and diff tests**

```go
func TestPrivateWorkspaceCRUDAndDiff(t *testing.T) {
    root := t.TempDir()
    manager := NewPrivateManager(root, 1<<20)
    ctx := context.Background()
    require.NoError(t, manager.MakeDir(ctx, "abc", "cfg/sourcemod"))
    _, err := manager.Save(ctx, "abc", "cfg/server.cfg", []byte("first"))
    require.NoError(t, err)
    require.NoError(t, manager.Move(ctx, "abc", "cfg/server.cfg", "cfg/sourcemod/server.cfg", false))
    tree, err := manager.Tree(ctx, "abc")
    require.NoError(t, err)
    require.Equal(t, []PrivateEntry{
        {Path: "cfg", Kind: "directory"},
        {Path: "cfg/sourcemod", Kind: "directory"},
        {Path: "cfg/sourcemod/server.cfg", Kind: "file", Size: 5},
    }, stripVolatile(tree))
    diff, err := manager.Diff(ctx, "abc")
    require.NoError(t, err)
    require.Equal(t, DiffSummary{Added: 1}, diff.Summary)
}

func TestPrivatePathRejectsEscapeSymlinkAndOverwrite(t *testing.T) {
    manager := NewPrivateManager(t.TempDir(), 1<<20)
    ctx := context.Background()
    _, err := manager.Save(ctx, "abc", "../outside", []byte("x"))
    require.ErrorContains(t, err, "unsafe")
    require.NoError(t, manager.MakeDir(ctx, "abc", "cfg"))
    _, err = manager.Save(ctx, "abc", "cfg/a.cfg", []byte("a"))
    require.NoError(t, err)
    _, err = manager.Save(ctx, "abc", "cfg/b.cfg", []byte("b"))
    require.NoError(t, err)
    require.ErrorContains(t, manager.Move(ctx, "abc", "cfg/a.cfg", "cfg/b.cfg", false), "exists")
}
```

- [ ] **Step 2: Run the tests and verify RED**

Run: `go test ./internal/content -run 'TestPrivate(Workspace|Path)' -count=1`

Expected: build failure because `MakeDir`, `Move`, `Tree`, `Diff`, `PrivateEntry` and `DiffSummary` do not exist.

- [ ] **Step 3: Add the workspace contract and atomic metadata helpers**

```go
type PrivateEntry struct {
    Path string `json:"path"`
    Kind string `json:"kind"`
    Hash string `json:"hash,omitempty"`
    Size int64 `json:"size,omitempty"`
    UpdatedAt time.Time `json:"updated_at"`
}

type PrivateChange struct {
    Path string `json:"path"`
    Kind string `json:"kind"`
    BeforeHash string `json:"before_hash,omitempty"`
    AfterHash string `json:"after_hash,omitempty"`
}

type DiffSummary struct {
    Added int `json:"added"`
    Modified int `json:"modified"`
    Deleted int `json:"deleted"`
}
type PrivateDiff struct {
    Changes []PrivateChange `json:"changes"`
    Summary DiffSummary `json:"summary"`
}

func (m *PrivateManager) MakeDir(ctx context.Context, instanceID, name string) error
func (m *PrivateManager) Move(ctx context.Context, instanceID, from, to string, overwrite bool) error
func (m *PrivateManager) Tree(ctx context.Context, instanceID string) ([]PrivateEntry, error)
func (m *PrivateManager) Diff(ctx context.Context, instanceID string) (PrivateDiff, error)
```

Implement each method with `safepath.Join`, `rejectSymlinkParents`, `os.Lstat`, same-filesystem temporary names and sorted slash-separated output. Store applied state at `instances/<id>/private-applied.json` using an unexported versioned structure:

```go
type privateManifest struct {
    Version int `json:"version"`
    AppliedAt time.Time `json:"applied_at"`
    Entries map[string]manifestEntry `json:"entries"`
}
type manifestEntry struct { Kind, Hash string; Size int64 }
```

- [ ] **Step 4: Run target and existing content tests**

Run: `go test ./internal/content -count=1`

Expected: PASS, including existing Save/Read/List/History/Delete behavior.

- [ ] **Step 5: Commit the workspace slice**

```bash
git add internal/content/private.go internal/content/private_state.go internal/content/private_test.go
git commit -m "feat(content): 支持私有文件工作区与差异"
```

### Task 2: Transactional apply, lower-layer restoration and snapshots

**Files:**
- Modify: `internal/content/private_state.go`
- Modify: `internal/content/private_test.go`
- Modify: `internal/updates/pipeline.go`
- Modify: `internal/updates/pipeline_test.go`

**Why this task exists:** Deleting an applied private file must reveal the correct lower layer, while package updates must refresh that lower layer before replaying private content.

**Impact / Compatibility:** `PrivateManager` remains the private-layer owner; `Pipeline` remains the package-layer owner. Apply journals protect the game tree and state manifest. Package update rollback must still restore the exact pre-update bytes.

**Repair Track:** Root cause is blind `ApplyTree(private, game)` with no applied manifest or removal path. Change the canonical private owner and its one package-pipeline call site.

**Retirement Track:** Retire `PrivateManager.Apply` after `ApplyChanges` and `RebaseAndApply` cover manual applies and lower-layer deployments. Keep `content.ApplyTree` for generic staged package copying.

**Verification:** `go test ./internal/content ./internal/updates -run 'TestPrivateApply|Test.*PrivateLast' -count=1`

- [ ] **Step 1: Write failing restoration, rollback and retention tests**

```go
func TestPrivateApplyDeleteRestoresCapturedLowerLayer(t *testing.T) {
    root := t.TempDir()
    game := filepath.Join(root, "instances", "abc", "game", "left4dead2", "cfg", "server.cfg")
    require.NoError(t, os.MkdirAll(filepath.Dir(game), 0750))
    require.NoError(t, os.WriteFile(game, []byte("package"), 0640))
    manager := NewPrivateManager(root, 1<<20)
    _, err := manager.Save(context.Background(), "abc", "cfg/server.cfg", []byte("private"))
    require.NoError(t, err)
    require.NoError(t, manager.ApplyChanges(context.Background(), "abc"))
    require.NoError(t, manager.Delete(context.Background(), "abc", "cfg/server.cfg"))
    require.NoError(t, manager.ApplyChanges(context.Background(), "abc"))
    raw, err := os.ReadFile(game)
    require.NoError(t, err)
    require.Equal(t, "package", string(raw))
}

func TestPrivateApplyFailureRollsBackGameAndManifest(t *testing.T) {
    manager, root := newFailingPrivateManager(t, "after-first-copy")
    seedAppliedPrivate(t, manager, root, "abc", "cfg/a.cfg", "old")
    _, err := manager.Save(context.Background(), "abc", "cfg/a.cfg", []byte("new"))
    require.NoError(t, err)
    require.Error(t, manager.ApplyChanges(context.Background(), "abc"))
    require.Equal(t, "old", readGameFile(t, root, "abc", "cfg/a.cfg"))
    require.Equal(t, "old", readAppliedHashContent(t, root, "abc", "cfg/a.cfg"))
}

func TestPrivateSnapshotsKeepNewestTwenty(t *testing.T) {
    manager := NewPrivateManager(t.TempDir(), 1<<20)
    for i := 0; i < 21; i++ {
        _, err := manager.Save(context.Background(), "abc", "cfg/value.cfg", []byte(strconv.Itoa(i)))
        require.NoError(t, err)
        require.NoError(t, manager.ApplyChanges(context.Background(), "abc"))
    }
    snapshots, err := manager.Snapshots(context.Background(), "abc")
    require.NoError(t, err)
    require.Len(t, snapshots, 20)
}
```

- [ ] **Step 2: Run and verify RED**

Run: `go test ./internal/content -run 'TestPrivateApply|TestPrivateSnapshots' -count=1`

Expected: build failure because transactional apply and snapshots are absent.

- [ ] **Step 3: Implement the journaled apply state machine**

Use these public methods and fixed paths:

```go
type PrivateSnapshot struct { ID string `json:"id"`; AppliedAt time.Time `json:"applied_at"`; Summary DiffSummary `json:"summary"` }

func (m *PrivateManager) ApplyChanges(ctx context.Context, instanceID string) error
func (m *PrivateManager) RebaseAndApply(ctx context.Context, instanceID string) error
func (m *PrivateManager) Snapshots(ctx context.Context, instanceID string) ([]PrivateSnapshot, error)
func (m *PrivateManager) RestoreSnapshot(ctx context.Context, instanceID, snapshotID string) error
```

`ApplyChanges` writes `backups/private/apply-<uuid>/journal.json`, backs up every affected game target and the old manifest, captures lower bytes for newly private paths under `backups/private/lower/`, restores lower bytes for deleted paths, copies the current private tree, writes the new manifest atomically, creates `backups/private/snapshots/<RFC3339Nano>-<uuid>/tree` and commits by deleting the journal. On any error, replay journal backups and keep the old manifest. `RebaseAndApply` refreshes lower bytes for every current private path from the already-deployed game tree, then copies private bytes and commits the refreshed manifest. Snapshot pruning happens only after commit.

- [ ] **Step 4: Replace package pipeline's blind private replay**

Change the existing block in `internal/updates/pipeline.go` to:

```go
private := content.NewPrivateManager(p.root, 1<<20)
if err := private.RebaseAndApply(ctx, instanceID); err != nil {
    return fail(err)
}
```

Add a pipeline test that begins with `package-v1`, applies `private`, deploys `package-v2`, removes the private file, calls `ApplyChanges`, and expects `package-v2` rather than `package-v1` or stale private bytes.

- [ ] **Step 5: Run content and update regression tests**

Run: `go test ./internal/content ./internal/updates -count=1`

Expected: PASS; existing hot/full update rollback tests remain green.

- [ ] **Step 6: Commit the transactional apply slice**

```bash
git add internal/content/private_state.go internal/content/private_test.go internal/updates/pipeline.go internal/updates/pipeline_test.go
git commit -m "feat(content): 原子应用并恢复私有覆盖下层"
```

### Task 3: Resumable uploads and complete HTTP contract

**Files:**
- Create: `internal/content/private_uploads.go`
- Create: `internal/content/private_uploads_test.go`
- Modify: `internal/httpapi/server.go`
- Modify: `internal/httpapi/server_test.go`
- Modify: `cmd/panel/main.go`

**Why this task exists:** The browser needs safe, complete file-manager operations without loading binary files into the text editor or memory.

**Impact / Compatibility:** Existing GET download and PUT text endpoints remain during UI migration. New mutations use existing authentication, same-origin checks, audit middleware and instance validation.

**Verification:** `go test ./internal/content ./internal/httpapi -run 'TestPrivateUpload|TestPrivateFileAPI' -count=1`

- [ ] **Step 1: Write failing upload-session tests**

```go
func TestPrivateUploadResumesAndCompletesAtomically(t *testing.T) {
    manager := NewPrivateUploadManager(t.TempDir(), 8<<20)
    session, err := manager.Begin("abc", "addons/file.bin", 6, sha256Hex([]byte("abcdef")))
    require.NoError(t, err)
    n, err := manager.Write(session.ID, 0, bytes.NewReader([]byte("abc")))
    require.Equal(t, int64(3), n)
    require.NoError(t, err)
    recovered, err := manager.Recover(session.ID)
    require.NoError(t, err)
    require.Equal(t, int64(3), recovered.Offset)
    _, err = manager.Write(session.ID, 3, bytes.NewReader([]byte("def")))
    require.NoError(t, err)
    require.NoError(t, manager.Complete(session.ID))
    require.Equal(t, "abcdef", readPrivateFile(t, manager.root, "abc", "addons/file.bin"))
}
```

- [ ] **Step 2: Run and verify RED**

Run: `go test ./internal/content -run TestPrivateUpload -count=1`

Expected: build failure because `PrivateUploadManager` is undefined.

- [ ] **Step 3: Implement bounded private upload sessions**

```go
type PrivateUploadSession struct {
    ID, InstanceID, Path, Hash string
    Size, Offset int64
}
type PrivateUploadManager struct { root string; maxBytes int64 }
func NewPrivateUploadManager(root string, maxBytes int64) *PrivateUploadManager
func (m *PrivateUploadManager) Begin(instanceID, path string, size int64, hash string) (PrivateUploadSession, error)
func (m *PrivateUploadManager) Write(id string, offset int64, reader io.Reader) (int64, error)
func (m *PrivateUploadManager) Recover(id string) (PrivateUploadSession, error)
func (m *PrivateUploadManager) Complete(id string) error
```

Store `.part` and `.json` under `instances/<id>/backups/private/uploads/`; verify offset, declared size and SHA-256, then atomically rename into the safe private target. Reject a session whose metadata instance/path does not resolve beneath the same instance.

- [ ] **Step 4: Write failing API contract tests**

Test these exact routes and statuses with an authenticated server:

```text
GET    /api/instances/{id}/private/tree                 200
GET    /api/instances/{id}/private/diff                 200
POST   /api/instances/{id}/private/directories          201
POST   /api/instances/{id}/private/move                 204
POST   /api/instances/{id}/private/uploads              201
PATCH  /api/instances/{id}/private/uploads/{uploadID}   204 + Upload-Offset
POST   /api/instances/{id}/private/uploads/{uploadID}/complete 204
GET    /api/instances/{id}/private/snapshots            200
POST   /api/instances/{id}/private/snapshots/{snapshotID}/restore 204
POST   /api/instances/{id}/private/apply                202
```

Assert `../`, symlink targets, wrong offsets and silent overwrite return structured 4xx responses; assert mutations appear in audit events.

- [ ] **Step 5: Implement handlers and wire managers**

Add request shapes with stable lower-case JSON:

```go
type privatePathRequest struct { Path string `json:"path"` }
type privateMoveRequest struct { From string `json:"from"`; To string `json:"to"`; Overwrite bool `json:"overwrite"` }
type privateUploadRequest struct { Path string `json:"path"`; Size int64 `json:"size"`; SHA256 string `json:"sha256"` }
```

`applyPrivate` must call `s.private.ApplyChanges` inside the existing `startJob` closure and report stages `snapshot`, `restore-lower`, `apply-private`, `commit`. Restore only changes the workspace; the next explicit apply creates the Job and snapshot.

- [ ] **Step 6: Run API and full Go regression tests**

Run: `go test ./internal/content ./internal/httpapi ./cmd/panel -count=1`

Expected: PASS with existing API routes still accepted.

- [ ] **Step 7: Commit the API slice**

```bash
git add internal/content/private_uploads.go internal/content/private_uploads_test.go internal/httpapi/server.go internal/httpapi/server_test.go cmd/panel/main.go
git commit -m "feat(api): 提供完整私有文件管理接口"
```

### Task 4: Independent private-files Tab

**Files:**
- Create: `web/src/app/PrivateFilesPage.tsx`
- Create: `web/src/app/PrivateFilesPage.test.tsx`
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/app/App.test.tsx`
- Modify: `web/src/styles/app.css`

**Why this task exists:** Administrators need the approved tree-and-editor workflow as a first-class instance Tab, including mobile use and visible staged state.

**Impact / Compatibility:** Add page key `private`; keep packages/shared VPK in `content`. Retire the embedded private list and “保存并立即应用” form only after the new Tab tests pass.

**Repair Track:** The existing content page mixes repository management and a partial private editor. Move private ownership to a focused component.

**Retirement Track:** Delete `privateFiles`, `privateHistory`, `privatePath`, `privateText`, `editPrivate` and their rendered panels from `ContentPage`; no compatibility reason remains once the Tab uses the new API.

**Verification:** `cd web && npm test -- --run src/app/PrivateFilesPage.test.tsx src/app/App.test.tsx && npm run build`

- [ ] **Step 1: Write failing main-journey component tests**

```tsx
it("stages edits and applies them only from the fixed status bar", async () => {
  render(<PrivateFilesPage instances={[instance]} queue={queue} />);
  await user.click(await screen.findByRole("treeitem", { name: "cfg" }));
  await user.click(screen.getByRole("button", { name: "编辑 server.cfg" }));
  await user.clear(screen.getByLabelText("文件内容"));
  await user.type(screen.getByLabelText("文件内容"), "hostname staged");
  await user.click(screen.getByRole("button", { name: "保存到暂存区" }));
  expect(queue).not.toHaveBeenCalled();
  expect(await screen.findByText("1 项修改未应用")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "应用更改" }));
  expect(queue).toHaveBeenCalledWith("/api/instances/abc/private/apply", {});
});

it("does not offer text editing for binary files", async () => {
  render(<PrivateFilesPage instances={[instance]} queue={queue} />);
  await user.click(await screen.findByRole("treeitem", { name: "addons" }));
  expect(screen.getByText("plugin.vpk")).toBeVisible();
  expect(screen.queryByRole("button", { name: "编辑 plugin.vpk" })).not.toBeInTheDocument();
});
```

- [ ] **Step 2: Run and verify RED**

Run: `cd web && npm test -- --run src/app/PrivateFilesPage.test.tsx`

Expected: FAIL because the component does not exist.

- [ ] **Step 3: Implement the focused page and stable types**

```tsx
export type PrivateEntry = { path: string; kind: "file" | "directory"; hash?: string; size?: number; updated_at: string };
export type PrivateDiff = { changes: Array<{ path: string; kind: "added" | "modified" | "deleted" }>; summary: { added: number; modified: number; deleted: number } };

export function PrivateFilesPage({ instances, queue }: {
  instances: Instance[];
  queue: (path: string, body: unknown) => Promise<void>;
}) {
  const [instanceID, setInstanceID] = useState(instances[0]?.id ?? "");
  const [entries, setEntries] = useState<PrivateEntry[]>([]);
  const [diff, setDiff] = useState<PrivateDiff>({ changes: [], summary: { added: 0, modified: 0, deleted: 0 } });
  const [selectedPath, setSelectedPath] = useState("");
  const [editor, setEditor] = useState("");
  const reload = useCallback(async () => {
    const [nextEntries, nextDiff] = await Promise.all([
      api<PrivateEntry[]>(`/api/instances/${instanceID}/private/tree`),
      api<PrivateDiff>(`/api/instances/${instanceID}/private/diff`),
    ]);
    setEntries(nextEntries);
    setDiff(nextDiff);
  }, [instanceID]);
  useEffect(() => { if (instanceID) void reload(); }, [instanceID, reload]);
  const saveText = async () => {
    await api(`/api/instances/${instanceID}/private/${encodeRelativePath(selectedPath)}`, {
      method: "PUT",
      headers: { "Content-Type": "text/plain; charset=utf-8" },
      body: editor,
    });
    await reload();
  };
  return (
    <section aria-labelledby="private-files-title">
      <h1 id="private-files-title">私有文件</h1>
      <PrivateToolbar instanceID={instanceID} reload={reload} />
      <div className="private-files-layout">
        <PrivateTree entries={entries} selectedPath={selectedPath} onSelect={setSelectedPath} />
        <PrivateEditor path={selectedPath} value={editor} onChange={setEditor} onSave={saveText} />
      </div>
      <PrivateChangeBar diff={diff} disabled={!diff.changes.length} onApply={() => queue(`/api/instances/${instanceID}/private/apply`, {})} />
    </section>
  );
}
```

Define `PrivateToolbar`, `PrivateTree`, `PrivateEditor` and `PrivateChangeBar` in the same focused file for the first slice. They must render `role="tree"`/`treeitem`, keyboard-operable disclosure buttons, a toolbar, full-width editor, error/status regions and a fixed `.private-change-bar`. On screens below 760px, render the same `PrivateTree` in a focus-managed drawer; do not maintain a second navigation state. Extract a child only if its own test or reuse boundary appears during refactor.

- [ ] **Step 4: Add navigation and remove the old private UI**

Update the page union and render branch:

```tsx
type Page = "overview" | "private" | "content" | "jobs" | "schedules" | "settings";
// navigation label: 私有文件
{page === "private" && <PrivateFilesPage instances={instances} queue={queue} />}
```

Delete the private state/actions/panels from `ContentPage`; leave package and shared-VPK behavior unchanged.

- [ ] **Step 5: Run component regression and build**

Run: `cd web && npm test -- --run src/app/PrivateFilesPage.test.tsx src/app/App.test.tsx && npm run build`

Expected: PASS with no TypeScript errors or accessibility-query failures.

- [ ] **Step 6: Commit the private-files UI**

```bash
git add web/src/app/PrivateFilesPage.tsx web/src/app/PrivateFilesPage.test.tsx web/src/app/App.tsx web/src/app/App.test.tsx web/src/styles/app.css
git commit -m "feat(web): 新增实例私有文件管理标签页"
```

### Task 5: Console follow-latest state machine

**Files:**
- Create: `web/src/app/useConsoleFollow.ts`
- Create: `web/src/app/useConsoleFollow.test.tsx`
- Modify: `web/src/app/App.tsx`
- Modify: `web/src/app/App.test.tsx`

**Why this task exists:** New output must follow only when the administrator is already at the bottom; entering or sending a command intentionally resumes following.

**Impact / Compatibility:** Do not alter WebSocket payloads, the 500-chunk cap or command submission. Reconnect preserves a user-disabled follow state within the mounted terminal.

**Repair Track:** Current terminal has no output ref or scroll intent state. Add one canonical hook.

**Retirement Track:** No fallback remains in `Terminal`; all programmatic scrolling goes through `forceFollow` or `followAfterRender`.

**Verification:** `cd web && npm test -- --run src/app/useConsoleFollow.test.tsx src/app/App.test.tsx`

- [ ] **Step 1: Write failing scroll-intent tests**

```tsx
it("stops following after the user scrolls up and resumes at bottom", async () => {
  const { result } = renderHook(() => useConsoleFollow());
  const node = fakeScrollable({ scrollTop: 700, scrollHeight: 1000, clientHeight: 300 });
  act(() => result.current.attach(node));
  act(() => result.current.forceFollow());
  expect(result.current.following()).toBe(true);
  node.scrollTop = 400;
  fireEvent.scroll(node);
  expect(result.current.following()).toBe(false);
  node.scrollTop = 700;
  fireEvent.scroll(node);
  expect(result.current.following()).toBe(true);
});

it("new output scrolls only while following", () => {
  const { result } = renderHook(() => useConsoleFollow());
  const node = fakeScrollable({ scrollTop: 400, scrollHeight: 1000, clientHeight: 300 });
  act(() => result.current.attach(node));
  act(() => result.current.followAfterRender());
  expect(node.scrollTop).toBe(400);
  act(() => result.current.forceFollow());
  expect(node.scrollTop).toBe(1000);
});
```

- [ ] **Step 2: Run and verify RED**

Run: `cd web && npm test -- --run src/app/useConsoleFollow.test.tsx`

Expected: FAIL because `useConsoleFollow` is missing.

- [ ] **Step 3: Implement the hook with bottom tolerance**

```ts
const BOTTOM_TOLERANCE = 4;
export function useConsoleFollow() {
  const node = useRef<HTMLElement | null>(null);
  const following = useRef(false);
  const attach = useCallback((value: HTMLElement | null) => { node.current = value; }, []);
  const atBottom = useCallback(() => !node.current || node.current.scrollHeight - node.current.clientHeight - node.current.scrollTop <= BOTTOM_TOLERANCE, []);
  const onScroll = useCallback(() => { following.current = atBottom(); }, [atBottom]);
  const forceFollow = useCallback(() => { following.current = true; requestAnimationFrame(() => node.current?.scrollTo({ top: node.current.scrollHeight })); }, []);
  const followAfterRender = useCallback(() => { if (following.current) requestAnimationFrame(() => node.current?.scrollTo({ top: node.current.scrollHeight })); }, []);
  return { attach, onScroll, forceFollow, followAfterRender, following: () => following.current };
}
```

- [ ] **Step 4: Wire exact terminal events**

Attach the hook to the `<pre>` and call:

```tsx
useLayoutEffect(() => { follow.forceFollow(); }, [instance.id]);
useLayoutEffect(() => { follow.followAfterRender(); }, [lines]);
<pre ref={follow.attach} onScroll={follow.onScroll}>{lines.join("")}</pre>
```

In submit, call `follow.forceFollow()` immediately before `socket.current?.send(...)`. Do not call `forceFollow` from `ws.onmessage`; line rendering uses `followAfterRender`, which respects the stored state.

- [ ] **Step 5: Run terminal tests and build**

Run: `cd web && npm test -- --run src/app/useConsoleFollow.test.tsx src/app/App.test.tsx && npm run build`

Expected: PASS; TypeScript accepts the ref callback and requestAnimationFrame tests use fake timers.

- [ ] **Step 6: Commit the console behavior**

```bash
git add web/src/app/useConsoleFollow.ts web/src/app/useConsoleFollow.test.tsx web/src/app/App.tsx web/src/app/App.test.tsx
git commit -m "fix(web): 按用户滚动状态跟随控制台输出"
```

### Task 6: Browser acceptance for file management and console intent

**Files:**
- Modify: `cmd/e2e-fixture/main.go`
- Modify: `web/e2e/control-panel.spec.ts`
- Modify: `web/src/styles/app.css`

**Why this task exists:** Unit tests cannot prove the real HTTP/Job journey, responsive layout or browser scroll geometry.

**Impact / Compatibility:** Extend the deterministic fixture only; production binaries must continue excluding e2e-tagged code.

**Verification:** `cd web && npm run e2e -- --project=desktop` and `cd web && npm run e2e -- --project=mobile`.

- [ ] **Step 1: Add a failing end-to-end journey**

Add one serial journey that:

```ts
await page.getByRole("button", { name: "私有文件" }).click();
await page.getByRole("button", { name: "新建目录" }).click();
await page.getByLabel("目录路径").fill("cfg/generated");
await page.getByRole("button", { name: "确认新建" }).click();
await page.getByRole("button", { name: "新建文件" }).click();
await page.getByLabel("文件路径").fill("cfg/generated/test.cfg");
await page.getByLabel("文件内容").fill("sm_cvar fixture 1");
await page.getByRole("button", { name: "保存到暂存区" }).click();
await expect(page.getByText(/1 项新增未应用/)).toBeVisible();
const applyJob = await captureJob(page, "/private/apply", () => page.getByRole("button", { name: "应用更改" }).click());
await waitForJob(page, applyJob.ID);
await page.reload();
await expect(page.getByText("test.cfg", { exact: true })).toBeVisible();
```

Continue through rename, binary upload/download affordances, delete/apply, snapshot restore/apply and refresh recovery. Seed a lower-layer fixture file and assert delete restores its fixture content through a read-only fixture diagnostic endpoint.

- [ ] **Step 2: Add console geometry assertions**

Generate enough fixture output to overflow the `<pre>`, then assert:

```ts
const terminal = page.locator(".terminal-modal pre");
await expect.poll(() => terminal.evaluate((el) => el.scrollTop + el.clientHeight >= el.scrollHeight - 4)).toBe(true);
await terminal.evaluate((el) => { el.scrollTop = 0; el.dispatchEvent(new Event("scroll")); });
const held = await terminal.evaluate((el) => el.scrollTop);
await page.locator(".terminal-modal input").fill("emit-new-output");
// fixture sends output without form submission through its test control
await expect.poll(() => terminal.textContent()).toContain("new-output");
expect(await terminal.evaluate((el) => el.scrollTop)).toBe(held);
await terminal.evaluate((el) => { el.scrollTop = el.scrollHeight; el.dispatchEvent(new Event("scroll")); });
await expect.poll(() => terminal.evaluate((el) => el.scrollTop + el.clientHeight >= el.scrollHeight - 4)).toBe(true);
```

Also assert submitting a command from the scrolled-up position forces the bottom and resumes following.

- [ ] **Step 3: Run desktop E2E and verify RED**

Run: `cd web && npm run e2e -- --project=desktop`

Expected: FAIL at the first missing private Tab or scroll-follow assertion before implementation; after Tasks 1–5 it must PASS.

- [ ] **Step 4: Correct only acceptance-exposed responsive defects**

Use `.private-files-layout { grid-template-columns: minmax(220px, 0.32fr) minmax(0, 1fr); }` on desktop and a focus-managed overlay drawer below 760px. Add Playwright geometry checks that editor/status-bar bounds stay inside the viewport and no horizontal page overflow exists.

- [ ] **Step 5: Run desktop and mobile acceptance**

Run: `cd web && npm run e2e -- --project=desktop`

Run: `cd web && npm run e2e -- --project=mobile`

Expected: PASS for both configured `desktop` and `mobile` projects.

- [ ] **Step 6: Commit browser acceptance**

```bash
git add cmd/e2e-fixture/main.go web/e2e/control-panel.spec.ts web/src/styles/app.css
git commit -m "test(e2e): 验证私有文件管理与控制台跟随"
```

### Task 7: Full regression, documentation and retirement audit

**Files:**
- Modify: `README.md`
- Create: `docs/aegis/work/2026-07-15-private-file-manager-console-follow/50-evidence.md`

**Why this task exists:** The handoff needs reproducible proof, documented operational semantics and confirmation that duplicate owners are gone.

**Impact / Compatibility:** Documentation must describe only verified behavior. Do not modify unrelated dirty files while preparing evidence.

**Verification:** All commands below exit zero; source searches show one private apply owner and no old immediate-apply UI.

- [ ] **Step 1: Run the complete verification matrix**

```bash
go test ./internal/content ./internal/updates ./internal/httpapi ./cmd/panel -count=1
go test ./... -count=1
cd web && npm test -- --run
cd web && npm run build
cd web && npm run e2e -- --project=desktop
cd web && npm run e2e -- --project=mobile
```

Expected: every command exits zero with no unexpected warnings. Run the configured mobile Playwright project as established in Task 6.

- [ ] **Step 2: Audit retirement and safety boundaries**

Run:

```bash
rg -n "保存并立即应用|private\.Apply\(" web/src internal
rg -n "RebaseAndApply|ApplyChanges" internal
rg -n "game/left4dead2|os\.Symlink|exec\.Command" web/src/app/PrivateFilesPage.tsx internal/content/private*.go
```

Expected: no old UI or blind `private.Apply` call; manual apply and lower-layer rebase each have one canonical call path; the private API does not expose game paths, create symlinks or execute commands.

- [ ] **Step 3: Update README and evidence**

Document the independent Tab, staged/apply workflow, 20 snapshots, lower-layer restoration and console follow rules. Record exact command outputs, desktop/mobile project names, any real-Docker test not run and residual operational risks in the evidence file.

- [ ] **Step 4: Commit final documentation**

```bash
git add README.md docs/aegis/work/2026-07-15-private-file-manager-console-follow/50-evidence.md
git commit -m "docs: 记录私有文件管理验证"
```

## Risks and rollback surface

- Lower-layer ownership is the highest-risk boundary. Implementation must locate every plugin/shared/Valve writer and call `RebaseAndApply` only after the complete lower layer is present.
- Apply journals and snapshots can consume disk; preflight space checks and post-commit retention are mandatory. Snapshot cleanup never precedes a successful commit.
- Existing instances lack a manifest. Migration must establish a conservative baseline without deleting ambiguous game files; a real existing-data fixture is required before release.
- The working tree is currently dirty in product files. Execution must use an isolated worktree or explicitly preserve and reconcile those edits before touching overlapping paths.
- Rollback is commit-based: each task is independently revertible. Do not revert the state-format task after production has written versioned manifests without also supplying a reader/migration path.

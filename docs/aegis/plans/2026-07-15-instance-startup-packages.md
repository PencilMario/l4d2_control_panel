# Instance Startup Configuration and Package Selection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let every new and existing instance independently choose a plugin package, edit SRCDS startup values and extra arguments, preview the final command, and provision the selected package before the first SRCDS process starts.

**Architecture:** Add selected-package state beside the existing applied-package state, centralize startup argv parsing in `internal/srcds`, and inject a new `internal/provisioning` service into lifecycle startup. Existing package Pipeline and persistent per-instance Jobs remain the mutation/rollback owners; React uses one controlled modal for create and edit.

**Tech Stack:** Go 1.25, SQLite, Docker Engine HTTP API, Python 3 Supervisor, React, TypeScript, Vitest, Testing Library and Playwright.

**Baseline / Authority Refs:** `docs/aegis/specs/2026-07-15-instance-startup-package-design.md`, `docs/aegis/specs/2026-07-14-l4d2-control-panel-design.md`, `README.md`, `docs/aegis/work/2026-07-15-instance-startup-packages/10-baseline-readset.md`.

**Compatibility Boundary:** Preserve Host networking, persistent mounts/data, fixed PTY console, per-instance Job serialization, existing package manifests and `package_version`, explicit hot/full content operations, and empty-extra-argument behavior. Do not expose Shell, Docker Exec or runtime entrypoint control.

**Verification:** `go test -count=1 ./...`, `go vet ./...`, `cd web && npm test -- --run`, `cd web && npm run build`, `cd web && npm run e2e`, `docker compose --env-file .env.example config --quiet`, plus desktop/mobile Playwright screenshots and viewport overflow checks.

---

### Task 1: Persist Selected Package Identity

**Files:**
- Modify: `internal/domain/models.go:19`
- Modify: `internal/store/migrations.go:3`
- Modify: `internal/store/store.go:31`
- Test: `internal/store/store_test.go`

**Why this task exists:**
- A requested package must remain distinguishable from the last package whose deployment committed successfully.
- Existing instances need an additive, reopen-safe migration without losing package history.

**Impact / Compatibility:**
- `PackageVersion` remains the applied package ID.
- Add `SelectedPackageID` and backfill it from `package_version`; no existing column or row is removed.

**Verification:** `go test -count=1 ./internal/store ./internal/domain`

**Repair Track:**
- Root cause: the current single `package_version` field cannot represent a pending package switch or first-start selection.
- Canonical owner: `domain.Instance` plus SQLite instance rows.
- Smallest change: one additive column and matching CRUD fields.
- Compatibility: old applied IDs remain intact and seed the new selected ID.
- Verification: create/update/reopen and legacy-schema migration tests.

**Retirement Track:**
- Old owner: callers treating `PackageVersion` as both desired and applied state.
- Active status: active until all create/update/deploy paths use `SelectedPackageID` for intent.
- Keep reason: database compatibility and applied-state reporting.
- Convergence trigger: all successful deployment paths update `PackageVersion` only after commit.
- Removal verification: no code assigns `PackageVersion` before a successful deployment.

- [ ] **Step 1: Write failing persistence and migration tests**

Add tests equivalent to:

```go
func openLegacyInstanceDatabase(t *testing.T, path, packageID string) *sql.DB {
    t.Helper()
    db, err := sql.Open("sqlite", path)
    if err != nil { t.Fatal(err) }
    _, err = db.Exec(`
        CREATE TABLE instances (
            id TEXT PRIMARY KEY, node_id TEXT NOT NULL, name TEXT NOT NULL UNIQUE,
            container_id TEXT NOT NULL, game_port INTEGER NOT NULL UNIQUE,
            sourcetv_port INTEGER NOT NULL DEFAULT 0, start_map TEXT NOT NULL,
            game_mode TEXT NOT NULL, tickrate INTEGER NOT NULL, max_players INTEGER NOT NULL,
            extra_args TEXT NOT NULL, runtime_image TEXT NOT NULL, package_version TEXT NOT NULL,
            desired_state TEXT NOT NULL, actual_state TEXT NOT NULL,
            created_at TEXT NOT NULL, updated_at TEXT NOT NULL
        );
        CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
        INSERT INTO schema_migrations(version, applied_at) VALUES (1, CURRENT_TIMESTAMP), (2, CURRENT_TIMESTAMP);
        INSERT INTO instances VALUES ('legacy','local','Legacy','',27015,0,'map','coop',100,8,'','runtime',?, 'stopped','stopped',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);
    `, packageID)
    if err != nil { t.Fatal(err) }
    return db
}

func TestSelectedPackagePersistsIndependentlyFromAppliedPackage(t *testing.T) {
    s, _ := Open(filepath.Join(t.TempDir(), "panel.db"))
    defer s.Close()
    value := domain.Instance{
        ID: "one", NodeID: "local", Name: "One", GamePort: 27015,
        StartMap: "c2m1_highway", GameMode: "coop", Tickrate: 100,
        MaxPlayers: 8, RuntimeImage: "runtime", SelectedPackageID: "selected",
        PackageVersion: "applied", DesiredState: domain.StateStopped,
        ActualState: domain.StateStopped,
    }
    if err := s.CreateInstance(context.Background(), value); err != nil { t.Fatal(err) }
    got, err := s.Instance(context.Background(), value.ID)
    if err != nil { t.Fatal(err) }
    if got.SelectedPackageID != "selected" || got.PackageVersion != "applied" {
        t.Fatalf("instance=%#v", got)
    }
}

func TestMigrationBackfillsSelectedPackageFromAppliedPackage(t *testing.T) {
    path := filepath.Join(t.TempDir(), "panel.db")
    legacy := openLegacyInstanceDatabase(t, path, "package-a")
    legacy.Close()
    s, err := Open(path)
    if err != nil { t.Fatal(err) }
    defer s.Close()
    got, err := s.Instance(context.Background(), "legacy")
    if err != nil { t.Fatal(err) }
    if got.SelectedPackageID != "package-a" || got.PackageVersion != "package-a" {
        t.Fatalf("instance=%#v", got)
    }
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test -count=1 ./internal/store`

Expected: compile failure because `domain.Instance.SelectedPackageID` does not exist, followed by missing-column failure once the field is added without migration logic.

- [ ] **Step 3: Add the domain field and additive migration**

Use explicit JSON names for the new and applied fields:

```go
type Instance struct {
    ID, NodeID, Name, ContainerID, StartMap, GameMode string
    ExtraArgs         string `json:"extra_args"`
    RuntimeImage      string
    PackageVersion    string `json:"applied_package_id"`
    SelectedPackageID string `json:"package_id"`
    GamePort           int
    SourceTVPort       int   `json:"sourcetv_port"`
    PluginPorts        []int `json:"plugin_ports"`
    Tickrate           int
    MaxPlayers         int
    DesiredState       InstanceState
    ActualState        InstanceState
    CreatedAt          time.Time
    UpdatedAt          time.Time
}
```

Add `selected_package_id TEXT NOT NULL DEFAULT ''` to the fresh schema. In `Store.Open`, inspect `PRAGMA table_info(instances)` and, when missing, run one transaction containing:

```sql
ALTER TABLE instances ADD COLUMN selected_package_id TEXT NOT NULL DEFAULT '';
UPDATE instances SET selected_package_id = package_version WHERE selected_package_id = '';
INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (3, CURRENT_TIMESTAMP);
```

Update insert, update, select, scan and `fields` ordering so `SelectedPackageID` round-trips transactionally.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run: `go test -count=1 ./internal/store`

Expected: PASS, including reopen and legacy migration coverage.

- [ ] **Step 5: Commit the slice**

```bash
git add internal/domain/models.go internal/store/migrations.go internal/store/store.go internal/store/store_test.go
git commit -m "feat(store): persist selected instance package"
```

### Task 2: Canonicalize SRCDS Arguments

**Files:**
- Create: `internal/srcds/command.go`
- Create: `internal/srcds/command_test.go`
- Modify: `go.mod`
- Modify: `go.sum`
- Modify: `internal/docker/lifecycle.go:27`
- Modify: `internal/docker/lifecycle_test.go`
- Modify: `internal/docker/client_test.go:25`
- Modify: `internal/lifecycle/service.go:176`
- Modify: `runtime/supervisor.py:131`
- Modify: `runtime/dockerfile_test.go`

**Why this task exists:**
- Preview, API validation, container transport and Supervisor execution must agree on argument order and quoting.
- Reserved Panel-owned flags must not silently make ports or health state inaccurate.

**Impact / Compatibility:**
- Keep `SRCDS_EXTRA_ARGS` as a fallback for old/manual runtime use.
- New Panel-created containers pass validated tokens through `SRCDS_EXTRA_ARGS_JSON` and Supervisor prefers that value.

**Verification:** `go test -count=1 ./internal/srcds ./internal/docker ./internal/lifecycle ./runtime`

**Repair Track:**
- Root cause: raw text is parsed only inside Python after container start, too late for validation and not reusable for preview tests.
- Canonical owner: new `internal/srcds` package.
- Smallest change: one parser/command builder and JSON token transport.
- Compatibility: raw env fallback stays readable.
- Verification: reserved flags, quoted values, JSON env and Python argv assertions.

**Retirement Track:**
- Old owner: Python-only `shlex.split(SRCDS_EXTRA_ARGS)`.
- Active status: retained only as compatibility fallback.
- Keep reason: manual runtime users and older container definitions.
- Convergence trigger: all Panel-created containers include JSON argv.
- Removal verification: deployment upgrade policy explicitly drops old runtime containers.

- [ ] **Step 1: Add failing parser and command-order tests**

Create tests equivalent to:

```go
func TestCommandAppendsValidatedExtraArguments(t *testing.T) {
    value := domain.Instance{GamePort: 27015, SourceTVPort: 27020, StartMap: "c2m1_highway", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, ExtraArgs: `-strictportbind +hostname "Night Coop"`}
    got, err := Command(value)
    if err != nil { t.Fatal(err) }
    want := []string{"./srcds_run", "-game", "left4dead2", "-console", "-port", "27015", "-tickrate", "100", "+map", "c2m1_highway", "+mp_gamemode", "coop", "-maxplayers", "8", "+tv_enable", "1", "+tv_port", "27020", "-strictportbind", "+hostname", "Night Coop"}
    if !slices.Equal(got, want) { t.Fatalf("got=%q want=%q", got, want) }
}

func TestParseExtraArgsRejectsPanelOwnedOptions(t *testing.T) {
    for _, raw := range []string{"-port 27016", "+map c1m1_hotel", "-tickrate=30", "+tv_port 27020"} {
        if _, err := ParseExtraArgs(raw); err == nil { t.Fatalf("accepted %q", raw) }
    }
}

func TestParseExtraArgsRejectsBrokenQuoting(t *testing.T) {
    if _, err := ParseExtraArgs(`+hostname "unterminated`); err == nil { t.Fatal("accepted invalid quoting") }
}
```

Extend Docker tests to expect `SRCDS_EXTRA_ARGS_JSON=["-strictportbind","+hostname","Night Coop"]`.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test -count=1 ./internal/srcds ./internal/docker ./internal/lifecycle ./runtime`

Expected: missing `internal/srcds` package/functions and missing JSON environment assertion.

- [ ] **Step 3: Implement parser, builder and JSON transport**

Add `github.com/mattn/go-shellwords` and implement:

```go
package srcds

var reserved = map[string]struct{}{
    "-game": {}, "-console": {}, "-port": {}, "-tickrate": {},
    "+map": {}, "+mp_gamemode": {}, "-maxplayers": {},
    "+tv_enable": {}, "+tv_port": {},
}

func ParseExtraArgs(raw string) ([]string, error) {
    args, err := shellwords.Parse(raw)
    if err != nil { return nil, fmt.Errorf("invalid extra arguments: %w", err) }
    for _, arg := range args {
        key := strings.SplitN(arg, "=", 2)[0]
        if _, blocked := reserved[key]; blocked {
            return nil, fmt.Errorf("%s is managed by the Panel", key)
        }
    }
    return args, nil
}

func Command(value domain.Instance) ([]string, error) {
    args := []string{"./srcds_run", "-game", "left4dead2", "-console", "-port", strconv.Itoa(value.GamePort), "-tickrate", strconv.Itoa(value.Tickrate), "+map", value.StartMap, "+mp_gamemode", value.GameMode, "-maxplayers", strconv.Itoa(value.MaxPlayers)}
    if value.SourceTVPort != 0 {
        args = append(args, "+tv_enable", "1", "+tv_port", strconv.Itoa(value.SourceTVPort))
    }
    extra, err := ParseExtraArgs(value.ExtraArgs)
    if err != nil { return nil, err }
    return append(args, extra...), nil
}
```

Change `docker.BuildContainerSpec` to return `(ContainerSpec, error)`, marshal only the tokens after the managed prefix into `SRCDS_EXTRA_ARGS_JSON`, keep `SRCDS_EXTRA_ARGS`, and propagate errors before `Engine.Create`.

Update Python:

```python
def extra_args():
    raw = os.getenv('SRCDS_EXTRA_ARGS_JSON', '').strip()
    if raw:
        value = json.loads(raw)
        if not isinstance(value, list) or not all(isinstance(item, str) for item in value):
            raise ValueError('SRCDS_EXTRA_ARGS_JSON must be a string array')
        return value
    return shlex.split(os.getenv('SRCDS_EXTRA_ARGS', ''))
```

Use `args.extend(extra_args())` in `srcds_command`.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run: `go test -count=1 ./internal/srcds ./internal/docker ./internal/lifecycle ./runtime`

Expected: PASS with exact command order and JSON transport.

- [ ] **Step 5: Commit the slice**

```bash
git add go.mod go.sum internal/srcds internal/docker/lifecycle.go internal/docker/lifecycle_test.go internal/docker/client_test.go internal/lifecycle/service.go runtime/supervisor.py runtime/dockerfile_test.go
git commit -m "feat(srcds): validate and transport startup arguments"
```

### Task 3: Provision Game and Package Before First Start

**Files:**
- Create: `internal/provisioning/service.go`
- Create: `internal/provisioning/service_test.go`
- Modify: `internal/docker/client.go:160`
- Modify: `internal/docker/client_test.go:91`
- Modify: `internal/lifecycle/service.go:104`
- Modify: `internal/lifecycle/service_test.go`
- Modify: `runtime/supervisor.py:126`
- Modify: `runtime/dockerfile_test.go`
- Modify: `cmd/panel/main.go:98`

**Why this task exists:**
- A selected plugin package must be deployed before the first SRCDS child exists.
- Runtime-owned installation can start a vanilla server and bypass the Panel's package transaction.

**Impact / Compatibility:**
- Reuse restricted maintenance containers, Steam credentials, update Pipeline and persistent directories.
- Game containers become run-only; the old Supervisor SteamCMD bootstrap retires after maintenance installation is wired.

**Verification:** `go test -count=1 ./internal/provisioning ./internal/docker ./internal/lifecycle ./runtime ./cmd/panel`

**Repair Track:**
- Root cause: first install currently occurs inside the game container immediately before SRCDS.
- Canonical owner: lifecycle-injected `provisioning.Service` using Docker maintenance and Pipeline owners.
- Smallest change: `InstallGame`, `Provisioner.Prepare`, lifecycle hook and dependency wiring.
- Compatibility: same anonymous Windows/Linux bootstrap sequence and persistent game bind.
- Verification: ordered event test proves install, deploy and container create sequence.

**Retirement Track:**
- Old owner: `runtime.ensure_game()` and `steamcmd_install_command()`.
- Active status: removed once Panel maintenance installation is exercised.
- Keep reason: none after production wiring; leaving it would permit vanilla startup.
- Convergence trigger: lifecycle always invokes provisioner when selected and applied packages differ with no container.
- Removal verification: runtime test asserts a missing `srcds_run` fails instead of downloading.

- [ ] **Step 1: Write failing install/provisioning/lifecycle-order tests**

Add an ordered provisioner test:

```go
type fakeRepo struct { instance domain.Instance }
func (r *fakeRepo) Instance(context.Context, string) (domain.Instance, error) { return r.instance, nil }
func (r *fakeRepo) UpdateInstance(_ context.Context, value domain.Instance) error { r.instance = value; return nil }

type fakeInstaller struct { events *[]string }
func (f fakeInstaller) InstallGame(context.Context, string, domain.Instance) error {
    *f.events = append(*f.events, "install")
    return nil
}

type fakePackages struct { item content.PackageVersion }
func (f fakePackages) Get(string) (content.PackageVersion, error) { return f.item, nil }

type fakeDeployer struct { events *[]string }
func (f fakeDeployer) Apply(context.Context, string, string, string, updates.Mode) error {
    *f.events = append(*f.events, "deploy")
    return nil
}

func TestPrepareInstallsThenDeploysSelectedPackage(t *testing.T) {
    events := []string{}
    repo := &fakeRepo{instance: domain.Instance{ID: "one", SelectedPackageID: "pkg"}}
    service := Service{
        Root: "/data", Instances: repo,
        Installer: fakeInstaller{events: &events},
        Packages: fakePackages{item: content.PackageVersion{ID: "pkg", ArchivePath: "pkg.zip", Version: "v1"}},
        Deployer: fakeDeployer{events: &events},
    }
    if err := service.Prepare(context.Background(), repo.instance); err != nil { t.Fatal(err) }
    if strings.Join(events, ",") != "install,deploy" { t.Fatalf("events=%v", events) }
    if repo.instance.PackageVersion != "pkg" { t.Fatalf("instance=%#v", repo.instance) }
}
```

Extend lifecycle test fakes with a `Prepare` event and assert `prepare,create,start`, with `PackageVersion` reloaded before `BuildContainerSpec`.

Add Docker test asserting the first-install command contains Windows bootstrap followed by Linux validate, and runtime test asserting missing game files fail fast.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test -count=1 ./internal/provisioning ./internal/docker ./internal/lifecycle ./runtime ./cmd/panel`

Expected: missing provisioning package, missing `InstallGame`, and lifecycle creating the game container before preparation.

- [ ] **Step 3: Implement maintenance installation and provisioner**

Refactor Docker maintenance execution behind:

```go
func (e *Engine) InstallGame(ctx context.Context, dataRoot string, instance domain.Instance) error {
    command := []string{"steamcmd", "+@sSteamCmdForcePlatformType", "windows", "+force_install_dir", "/opt/l4d2/game"}
    command = append(command, e.steamLoginArgs()...)
    command = append(command, "+app_update", "222860", "+@sSteamCmdForcePlatformType", "linux", "+app_update", "222860", "validate", "+quit")
    return e.runMaintenance(ctx, dataRoot, instance, command)
}
```

For licensed credentials, preserve the existing Linux-only verified path. Keep `UpdateGame` on Linux validate and share only container creation/adoption/wait/removal mechanics.

Implement `provisioning.Service.Prepare` to validate `SelectedPackageID`, call `InstallGame`, call `Pipeline.Apply(..., updates.Full)`, then reload and persist `PackageVersion` only after commit.

Use these explicit boundaries:

```go
type Installer interface {
    InstallGame(context.Context, string, domain.Instance) error
}
type PackageSource interface {
    Get(string) (content.PackageVersion, error)
}
type Deployer interface {
    Apply(context.Context, string, string, string, updates.Mode) error
}
type InstanceRepository interface {
    Instance(context.Context, string) (domain.Instance, error)
    UpdateInstance(context.Context, domain.Instance) error
}
type Service struct {
    Root string
    Installer Installer
    Packages PackageSource
    Deployer Deployer
    Instances InstanceRepository
}
```

Inject this interface into lifecycle:

```go
type Provisioner interface { Prepare(context.Context, domain.Instance) error }
func WithProvisioner(value Provisioner) Option { return func(s *Service) { s.provisioner = value } }
```

When `ContainerID == "" && PackageVersion != SelectedPackageID`, set `installing`, call `Prepare`, reload the instance, then build/create/start the game container. On error call `fault` and do not create the container.

Replace runtime installation with:

```python
def require_game():
    if not os.path.isfile(os.path.join(GAME, 'srcds_run')):
        raise RuntimeError('game content was not provisioned by the Panel')
```

- [ ] **Step 4: Wire production dependencies and verify GREEN**

Create package manager and Pipeline before lifecycle in `cmd/panel/main.go`, instantiate `provisioning.Service`, and pass `lifecycle.WithProvisioner(provisioner)`.

Run: `go test -count=1 ./internal/provisioning ./internal/docker ./internal/lifecycle ./runtime ./cmd/panel`

Expected: PASS, with first-install order proven and runtime bootstrap absent.

- [ ] **Step 5: Commit the slice**

```bash
git add internal/provisioning internal/docker/client.go internal/docker/client_test.go internal/lifecycle/service.go internal/lifecycle/service_test.go runtime/supervisor.py runtime/dockerfile_test.go cmd/panel/main.go
git commit -m "feat(provisioning): deploy package before first srcds start"
```

### Task 4: Make Package Updates Preserve Instance Intent

**Files:**
- Modify: `internal/updates/coordinator.go`
- Modify: `internal/updates/coordinator_test.go`
- Modify: `internal/httpapi/server.go:618`
- Modify: `internal/httpapi/server_test.go`
- Modify: `cmd/panel/main.go:128`
- Modify: `cmd/e2e-fixture/main.go:197`

**Why this task exists:**
- Configuration-driven package changes must keep stopped instances stopped and update applied state only after commit.
- Manual and scheduled deployments currently have different persistence owners.

**Impact / Compatibility:**
- Hot/full endpoints and Pipeline rollback remain unchanged.
- `updates.Coordinator` becomes the single owner that records selected/applied package IDs after a successful commit.

**Verification:** `go test -count=1 ./internal/updates ./internal/httpapi ./internal/automation ./cmd/e2e-fixture`

**Repair Track:**
- Root cause: full update always stops/starts, and HTTP alone writes `PackageVersion`.
- Canonical owner: `updates.Coordinator` with an `Instances` repository.
- Smallest change: load running intent, conditionally stop/start, and mark package after commit.
- Compatibility: running instances still receive health-checked restart; stopped instances no longer start unexpectedly.
- Verification: running, stopped, rollback and scheduled persistence tests.

**Retirement Track:**
- Old owner: HTTP `updatePackage` post-deployment database write.
- Active status: remove after Coordinator persists state.
- Keep reason: none; duplicate writes risk drift.
- Convergence trigger: all callers construct Coordinator with `Instances`.
- Removal verification: `rg "PackageVersion = item.ID" internal/httpapi` returns no match.

- [ ] **Step 1: Write failing stopped/running and persistence tests**

Add tests equivalent to:

```go
type packageRepo struct { instance domain.Instance }
func (r *packageRepo) Instance(context.Context, string) (domain.Instance, error) { return r.instance, nil }
func (r *packageRepo) UpdateInstance(_ context.Context, value domain.Instance) error { r.instance = value; return nil }

func TestFullCoordinatorKeepsStoppedInstanceStopped(t *testing.T) {
    repo := &packageRepo{instance: domain.Instance{ID: "one", DesiredState: domain.StateStopped, ActualState: domain.StateStopped, SelectedPackageID: "pkg"}}
    life := &fakeLifecycle{}
    coordinator := Coordinator{Instances: repo, Lifecycle: life, Deployer: fakeDeployer{life: life}}
    if err := coordinator.ApplyPackage(context.Background(), "one", content.PackageVersion{ID: "pkg", ArchivePath: "pkg.zip", Version: "v1"}, Full); err != nil { t.Fatal(err) }
    if strings.Join(life.events, ",") != "deploy,commit" { t.Fatalf("events=%v", life.events) }
    if repo.instance.PackageVersion != "pkg" { t.Fatalf("instance=%#v", repo.instance) }
}
```

Keep running order `stop,deploy,start,commit`; assert failure leaves `PackageVersion` unchanged.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test -count=1 ./internal/updates ./internal/httpapi ./internal/automation`

Expected: stopped test observes unwanted `stop,start`, and persistence remains owned only by HTTP.

- [ ] **Step 3: Move intent and state persistence into Coordinator**

Add:

```go
type PackageInstanceRepository interface {
    Instance(context.Context, string) (domain.Instance, error)
    UpdateInstance(context.Context, domain.Instance) error
}
```

In full mode, load the instance, derive `resume`, stop only when active, deploy, start only when `resume`, commit, then reload and set both `SelectedPackageID` and `PackageVersion` to `item.ID`. Hot mode commits then marks the same fields. Rollback restarts only when the original instance was running.

Remove the duplicate HTTP write and wire `Instances: db` in production and fixture coordinators.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run: `go test -count=1 ./internal/updates ./internal/httpapi ./internal/automation ./cmd/e2e-fixture`

Expected: PASS; stopped instances stay stopped and all deployment callers persist applied state.

- [ ] **Step 5: Commit the slice**

```bash
git add internal/updates/coordinator.go internal/updates/coordinator_test.go internal/httpapi/server.go internal/httpapi/server_test.go internal/automation cmd/panel/main.go cmd/e2e-fixture/main.go
git commit -m "fix(updates): preserve instance state during package deploy"
```

### Task 5: Extend Instance Create and Reconfiguration APIs

**Files:**
- Modify: `internal/httpapi/server.go:360`
- Modify: `internal/httpapi/server_test.go:82`
- Modify: `cmd/e2e-fixture/main.go:39`

**Why this task exists:**
- New instances need a package and startup arguments at creation.
- Existing instances need one serialized reconfiguration Job rather than unrelated concurrent package/rebuild actions.

**Impact / Compatibility:**
- Strict JSON decoding remains active.
- Uninstalled/containerless edits return `200`; installed runtime/package changes return one `202` Job.
- Display-name-only changes do not rebuild.

**Verification:** `go test -count=1 ./internal/httpapi ./cmd/e2e-fixture`

**Repair Track:**
- Root cause: create omits `extra_args`, update omits package selection, and update rebuilds every installed instance even for name-only edits.
- Canonical owner: shared `instanceInput` validation/diff helper in HTTP API.
- Smallest change: extend request, validate package/args, compare old/new runtime fields, queue one operation.
- Compatibility: existing route paths and Job polling remain unchanged.
- Verification: 201/200/202, invalid package, invalid args, name-only and combined reconfiguration tests.

**Retirement Track:**
- Old owner: unconditional installed-instance rebuild in `updateInstance`.
- Active status: replaced by field-diff planning.
- Keep reason: none.
- Convergence trigger: tests prove name-only update returns `200`.
- Removal verification: no unconditional `instance.ContainerID != ""` rebuild branch remains.

- [ ] **Step 1: Write failing HTTP contract tests**

Add package-seeding test helpers and tests equivalent to:

```go
func addTestPackage(t *testing.T, manager *content.PackageManager, name, version string) string {
    t.Helper()
    var raw bytes.Buffer
    writer := zip.NewWriter(&raw)
    file, err := writer.Create("cfg/plugin.cfg")
    if err != nil { t.Fatal(err) }
    if _, err := file.Write([]byte("sm_cvar fixture 1")); err != nil { t.Fatal(err) }
    if err := writer.Close(); err != nil { t.Fatal(err) }
    item, err := manager.AddUpload(name, version, bytes.NewReader(raw.Bytes()), int64(raw.Len()))
    if err != nil { t.Fatal(err) }
    return item.ID
}

func authenticatedJSON(t *testing.T, server *Server, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
    t.Helper()
    request := httptest.NewRequest(method, path, strings.NewReader(body))
    request.AddCookie(cookie)
    response := httptest.NewRecorder()
    server.Handler().ServeHTTP(response, request)
    return response
}

func TestCreatePersistsPackageAndExtraArgs(t *testing.T) {
    s, db := testServer(t)
    defer db.Close()
    packageID := addTestPackage(t, s.packages, "plugins-a.zip", "v1")
    cookie := loginCookie(t, s)
    body := fmt.Sprintf(`{"name":"One","game_port":27015,"start_map":"c2m1_highway","game_mode":"coop","tickrate":100,"max_players":8,"extra_args":"-strictportbind","package_id":%q}`, packageID)
    response := authenticatedJSON(t, s, cookie, http.MethodPost, "/api/instances", body)
    if response.Code != http.StatusCreated { t.Fatalf("status=%d body=%s", response.Code, response.Body.String()) }
    items, _ := db.Instances(context.Background())
    if items[0].SelectedPackageID != packageID || items[0].ExtraArgs != "-strictportbind" { t.Fatalf("instance=%#v", items[0]) }
}
```

Configure `testServer` with a real temporary `PackageManager`, update every existing create payload to include an ID returned by `addTestPackage`, and add tests for missing package `422`, reserved args `422`, name-only installed update `200`, runtime update `202 rebuild`, package update `202 reconfigure`, and combined update producing one Job.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test -count=1 ./internal/httpapi ./cmd/e2e-fixture`

Expected: create rejects new fields as unknown; update cannot select a package; installed name-only edit returns `202`.

- [ ] **Step 3: Implement shared input validation and diff planning**

Define one request type:

```go
type instanceInput struct {
    Name string `json:"name"`
    GamePort int `json:"game_port"`
    SourceTVPort int `json:"sourcetv_port"`
    PluginPorts []int `json:"plugin_ports"`
    StartMap string `json:"start_map"`
    GameMode string `json:"game_mode"`
    Tickrate int `json:"tickrate"`
    MaxPlayers int `json:"max_players"`
    ExtraArgs string `json:"extra_args"`
    PackageID string `json:"package_id"`
}
```

Validation must call `srcds.ParseExtraArgs`, `validateDeclaredPorts`, and `s.packages.Get(input.PackageID)`. Creation stores `SelectedPackageID` but leaves `PackageVersion` empty.

For update, capture `runtimeChanged` and `packageChanged` before saving. If no runtime work is required, return the instance. Otherwise queue one `reconfigure` Job:

```go
if packageChanged {
    reporter.Progress("package", 20, "deploying selected package")
    if err := s.updateCoordinator.ApplyPackage(ctx, id, item, updates.Full); err != nil { return err }
}
if runtimeChanged {
    reporter.Progress("container", 70, "rebuilding game container")
    return s.lifecycle.Rebuild(ctx, id)
}
return nil
```

Jobs already serialize per instance; do not add a second lock.

- [ ] **Step 4: Update fixture start semantics and verify GREEN**

In `fixtureLifecycle.Start`, when selected and applied packages differ, copy the selected ID into `PackageVersion` before marking running. This keeps browser tests deterministic while production order is covered by provisioning unit tests.

Run: `go test -count=1 ./internal/httpapi ./cmd/e2e-fixture`

Expected: PASS for create, edit, validation and one-Job reconfiguration contracts.

- [ ] **Step 5: Commit the slice**

```bash
git add internal/httpapi/server.go internal/httpapi/server_test.go cmd/e2e-fixture/main.go
git commit -m "feat(api): configure startup and package per instance"
```

### Task 6: Build the Shared React Configuration Experience

**Files:**
- Create: `web/src/app/InstanceConfigModal.tsx`
- Create: `web/src/app/InstanceConfigModal.test.tsx`
- Modify: `web/src/api/client.ts:36`
- Modify: `web/src/app/App.tsx:32`
- Modify: `web/src/app/App.test.tsx`
- Modify: `web/src/styles/app.css:412`

**Why this task exists:**
- Administrators need the same editable defaults, package choice and command preview for create and edit.
- Existing instances currently have no configuration action despite an update API.

**Impact / Compatibility:**
- Preserve overview card actions, Job strip/polling, content uploads and mobile navigation.
- Use Lucide icons, accessible labels, stable modal dimensions and wrapping command text.

**Verification:** `cd web && npm test -- --run && npm run build`

**Repair Track:**
- Root cause: creation is an isolated uncontrolled form and the card chevron has no behavior.
- Canonical owner: controlled `InstanceConfigModal` reused by create/edit.
- Smallest change: extract only instance configuration; leave unrelated App sections intact.
- Compatibility: existing action callbacks and content page remain operational.
- Verification: component payload, preview, edit refill, Job response and viewport tests.

**Retirement Track:**
- Old owner: `CreateInstance` inside `App.tsx` and inert chevron button.
- Active status: remove after shared modal is wired.
- Keep reason: none.
- Convergence trigger: both create and edit render `InstanceConfigModal`.
- Removal verification: `rg "function CreateInstance" web/src/app/App.tsx` returns no match.

- [ ] **Step 1: Write failing modal and App integration tests**

Test the pure preview and both submit modes:

```tsx
it("renders the managed command and editable extra arguments", async () => {
  render(<InstanceConfigModal mode="create" packages={[packageA]} onClose={vi.fn()} onSubmit={vi.fn()} />);
  await userEvent.clear(screen.getByLabelText("额外 SRCDS 启动项"));
  await userEvent.type(screen.getByLabelText("额外 SRCDS 启动项"), '-strictportbind +hostname "Night Coop"');
  expect(screen.getByLabelText("启动指令预览")).toHaveTextContent('./srcds_run -game left4dead2 -console -port 27015');
  expect(screen.getByLabelText("启动指令预览")).toHaveTextContent('-strictportbind +hostname "Night Coop"');
});

it("submits an existing instance package and startup edit", async () => {
  const submit = vi.fn();
  render(<InstanceConfigModal mode="edit" instance={instance} packages={[packageA, packageB]} onClose={vi.fn()} onSubmit={submit} />);
  await userEvent.selectOptions(screen.getByLabelText("插件包"), packageB.id);
  await userEvent.click(screen.getByRole("button", { name: "保存并应用" }));
  expect(submit).toHaveBeenCalledWith(expect.objectContaining({ package_id: packageB.id, extra_args: instance.extra_args }));
});
```

Add App tests for the card configuration button, package label/pending state, `PUT /api/instances/:id`, and `202` Job polling.

- [ ] **Step 2: Run tests and verify RED**

Run: `cd web && npm test -- --run`

Expected: missing modal module, missing normalized fields and inert configuration button.

- [ ] **Step 3: Implement normalized types and controlled modal**

Add normalized fields:

```ts
extra_args: value.extra_args ?? value.ExtraArgs ?? "",
tickrate: value.tickrate ?? value.Tickrate ?? 100,
package_id: value.package_id ?? value.SelectedPackageID ?? "",
applied_package_id: value.applied_package_id ?? value.PackageVersion ?? "",
```

Create `InstanceConfigModal` with controlled values for every field, a comma-separated plugin-port adapter, package select, and:

```ts
export function buildLaunchPreview(value: InstanceConfigValues) {
  const parts = [
    "./srcds_run", "-game", "left4dead2", "-console",
    "-port", String(value.game_port), "-tickrate", String(value.tickrate),
    "+map", value.start_map, "+mp_gamemode", value.game_mode,
    "-maxplayers", String(value.max_players),
  ];
  if (value.sourcetv_port) parts.push("+tv_enable", "1", "+tv_port", String(value.sourcetv_port));
  return `${parts.join(" ")}${value.extra_args.trim() ? ` ${value.extra_args.trim()}` : ""}`;
}
```

The prefix is visible but not editable. Disable submission when packages are empty. Use an error region for failed requests rather than dismissing the modal.

- [ ] **Step 4: Wire App state, create/edit and Job handling**

Load packages into App-level state, pass them to Overview and ContentPage, replace the inert chevron with a Lucide settings control named `配置 <instance>`, and route modal submit to POST or PUT. Detect a Job response by `Status` and hand it to the existing `setJob`/`pollJob` path; otherwise reload instances immediately.

Style `.instance-config-modal` with bounded width/height, scrollable form content and a monospace `.command-preview` using `white-space: pre-wrap; overflow-wrap: anywhere;`. Keep cards at the existing radius and ensure controls wrap on mobile.

- [ ] **Step 5: Run frontend tests/build and commit**

Run: `cd web && npm test -- --run && npm run build`

Expected: all component tests PASS and TypeScript/Vite build succeeds.

```bash
git add web/src/api/client.ts web/src/app/App.tsx web/src/app/App.test.tsx web/src/app/InstanceConfigModal.tsx web/src/app/InstanceConfigModal.test.tsx web/src/styles/app.css
git commit -m "feat(web): edit and preview instance startup configuration"
```

### Task 7: Verify the Real Browser Journey

**Files:**
- Modify: `web/e2e/control-panel.spec.ts`
- Modify: `cmd/e2e-fixture/main.go`
- Modify: `README.md`
- Modify: `docs/aegis/work/2026-07-15-instance-startup-packages/40-atomic-tasks.md`
- Modify: `docs/aegis/work/2026-07-15-instance-startup-packages/50-evidence.md`

**Why this task exists:**
- The main user value spans upload, creation, preview, first start, edit, Job recovery and refresh.
- Desktop/mobile layout must remain usable with the larger configuration modal and long command text.

**Impact / Compatibility:**
- Preserve the existing console, player, VPK, private file, schedule and Job recovery journey.
- Reorder package upload before instance creation because package selection is required.

**Verification:** Full commands listed below, plus Playwright desktop/mobile screenshots.

- [ ] **Step 1: Write the failing E2E journey**

Update the test to:

1. Log in and open Content Repository.
2. Upload `package-a.zip` and `package-b.zip`.
3. Return to Overview and create the instance with package A plus `-strictportbind +hostname "E2E <suffix>"`.
4. Assert the live preview before submit.
5. Start and wait for the first-start Job; refresh and assert package A is applied.
6. Open configuration, switch to package B, change extra args, submit, wait for the one reconfigure Job, refresh and assert package B plus the new preview values.
7. Continue the existing console/player/VPK/private/schedule/Job assertions.

Add an API assertion from the browser:

```ts
const saved = await page.evaluate(async (name) => {
  const response = await fetch("/api/instances");
  const items = await response.json();
  return items.find((item: any) => (item.name ?? item.Name) === name);
}, instanceName);
expect(saved.extra_args ?? saved.ExtraArgs).toContain("+hostname");
expect(saved.package_id ?? saved.SelectedPackageID).toBe(saved.applied_package_id ?? saved.PackageVersion);
```

- [ ] **Step 2: Run E2E and verify RED**

Run: `cd web && npm run e2e`

Expected: failure because package selection/configuration preview and first-start applied state are not yet represented in the fixture/browser flow.

- [ ] **Step 3: Complete fixture behavior and run GREEN**

Ensure fixture start copies selected to applied state and fixture update uses the production Coordinator state owner. Keep external Docker/SRCDS boundaries fake; do not duplicate production package Pipeline behavior.

Run: `cd web && npm run e2e`

Expected: desktop and mobile projects PASS, screenshots show no clipped modal, command text, cards or navigation.

- [ ] **Step 4: Run full regression and operational checks**

Run:

```text
go test -count=1 ./...
go vet ./...
cd web
npm test -- --run
npm run build
npm run e2e
cd ..
docker compose --env-file .env.example config --quiet
```

Expected: every command exits zero; test output contains no unexpected warnings or failures.

- [ ] **Step 5: Update docs/evidence and commit**

Update README instance creation/first-start notes, mark atomic tasks with actual status, and append exact red/green/regression evidence plus screenshot paths to `50-evidence.md`.

```bash
git add README.md web/e2e/control-panel.spec.ts cmd/e2e-fixture/main.go docs/aegis/work/2026-07-15-instance-startup-packages/40-atomic-tasks.md docs/aegis/work/2026-07-15-instance-startup-packages/50-evidence.md
git commit -m "test(e2e): verify per-instance startup and package flow"
```

### Task 8: Review and Completion Audit

**Files:**
- Modify only files required by concrete review findings.
- Verify: all files changed on `feature/instance-startup-packages`.

**Why this task exists:**
- The change crosses persistence, lifecycle, package rollback, runtime execution and a user-visible workflow.
- Completion requires fresh evidence and a review of contract drift, not only passing focused tests.

**Impact / Compatibility:**
- No unrelated refactor or metadata churn is accepted during review fixes.
- Every fix must keep the approved spec and regression commands green.

**Verification:** `git diff --check`, full regression matrix, status/diff review, and Aegis verification-before-completion checklist.

- [ ] **Step 1: Run spec coverage and placeholder audit**

Map design sections 3-11 to Tasks 1-7. Search changed code/docs for placeholder markers, incomplete branches, duplicate package-state owners and direct raw extra-argument execution.

- [ ] **Step 2: Perform code review**

Review migration idempotence, stale instance overwrites, first-start ordering, stopped-instance behavior, rollback/applied-state timing, reserved-flag validation, Job serialization, React response discrimination, accessibility and mobile overflow.

- [ ] **Step 3: Fix concrete findings with regression tests first**

For each finding, add a failing test in the owning package, observe the expected failure, make the smallest repair, and rerun the owning plus dependent test sets.

- [ ] **Step 4: Run final verification**

Run the full Task 7 matrix again, `git diff --check`, `git status --short`, and inspect `git diff --stat main...HEAD` plus `git log --oneline main..HEAD`.

- [ ] **Step 5: Record final evidence**

Append final commands, results, residual risks and any unexecuted real-Docker acceptance to `docs/aegis/work/2026-07-15-instance-startup-packages/50-evidence.md`, then commit only if the evidence file changed:

```bash
git add docs/aegis/work/2026-07-15-instance-startup-packages/50-evidence.md
git commit -m "docs(evidence): record startup package verification"
```

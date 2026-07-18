package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/not0721here/l4d2-control-panel/internal/docker"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/maintenance"
	"github.com/not0721here/l4d2-control-panel/internal/store"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStartWaitsForGlobalMaintenanceLease(t *testing.T) {
	gate := maintenance.NewGate()
	release, err := gate.Exclusive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	service := New(nil, nil, nil, t.TempDir(), WithMaintenanceGate(gate))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := service.Start(ctx, "abc"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
}

type fakeEngine struct {
	created, started, stopped bool
	execOperation             string
	containers                []docker.Container
	removed                   bool
	events                    *[]string
	createdSpec               docker.ContainerSpec
}

func (f *fakeEngine) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	f.created = true
	f.createdSpec = spec
	if f.events != nil {
		*f.events = append(*f.events, "create")
	}
	return "container-1", nil
}
func (f *fakeEngine) Start(context.Context, string) error {
	f.started = true
	if f.events != nil {
		*f.events = append(*f.events, "start")
	}
	return nil
}
func (f *fakeEngine) RunSupervisor(_ context.Context, _ string, op string) error {
	f.execOperation = op
	return nil
}
func (f *fakeEngine) Stop(context.Context, string, int) error {
	f.stopped = true
	if f.events != nil {
		*f.events = append(*f.events, "stop")
	}
	return nil
}
func (f *fakeEngine) ListManaged(context.Context) ([]docker.Container, error) {
	return f.containers, nil
}
func (f *fakeEngine) Remove(context.Context, string) error {
	f.removed = true
	if f.events != nil {
		*f.events = append(*f.events, "remove")
	}
	return nil
}

type freePorts struct{}

func (freePorts) Available(context.Context, string, []int) error { return nil }

type recordingPorts struct{ checked []int }

func (p *recordingPorts) Available(_ context.Context, _ string, ports []int) error {
	p.checked = append(p.checked, ports...)
	return nil
}

type fixedSpace uint64

func (s fixedSpace) Available(string) (uint64, error) { return uint64(s), nil }

type controlledHealth struct {
	mu      sync.Mutex
	calls   []string
	release chan struct{}
	called  chan string
}

func (h *controlledHealth) Wait(_ context.Context, instance domain.Instance) error {
	h.mu.Lock()
	h.calls = append(h.calls, instance.ContainerID)
	h.mu.Unlock()
	if h.called != nil {
		h.called <- instance.ContainerID
	}
	if instance.ContainerID == "old" {
		<-h.release
	}
	return nil
}

func (h *controlledHealth) recorded() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.calls...)
}

type fakeProvisioner struct {
	repo   *store.Store
	events *[]string
	err    error
}

type fakeLogPreparer struct {
	events *[]string
	err    error
	check  func() error
}

func (p fakeLogPreparer) Prepare(_ context.Context, instanceID string) error {
	*p.events = append(*p.events, "logs:"+instanceID)
	if p.check != nil {
		return p.check()
	}
	return p.err
}

func (p fakeProvisioner) Prepare(ctx context.Context, value domain.Instance) error {
	*p.events = append(*p.events, "prepare")
	if p.err != nil {
		return p.err
	}
	value.PackageVersion = value.SelectedPackageID
	return p.repo.UpdateInstance(ctx, value)
}

func TestStartPreparesSelectedPackageBeforeCreatingContainer(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	value := domain.Instance{ID: "prepared", NodeID: "local", Name: "prepared", GamePort: 27015, StartMap: "map", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", SelectedPackageID: "package-a", ActualState: domain.StateUninstalled}
	if err := db.CreateInstance(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	events := []string{}
	engine := &fakeEngine{events: &events}
	service := New(db, engine, freePorts{}, root, WithProvisioner(fakeProvisioner{repo: db, events: &events}), WithLogPreparer(fakeLogPreparer{events: &events}))
	if err := service.Start(context.Background(), value.ID); err != nil {
		t.Fatal(err)
	}
	if strings.Join(events, ",") != "prepare,logs:prepared,create,start" {
		t.Fatalf("events=%v", events)
	}
	got, err := db.Instance(context.Background(), value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PackageVersion != "package-a" || got.ActualState != domain.StateRunning {
		t.Fatalf("instance=%#v", got)
	}
}

func TestStartDoesNotCreateContainerWhenLogPreparationFails(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	value := domain.Instance{ID: "failed-logs", NodeID: "local", Name: "failed logs", GamePort: 27015, StartMap: "map", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", ActualState: domain.StateStopped}
	if err := db.CreateInstance(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	events := []string{}
	engine := &fakeEngine{events: &events}
	service := New(db, engine, freePorts{}, root, WithLogPreparer(fakeLogPreparer{events: &events, err: errors.New("migration failed")}))
	if err := service.Start(context.Background(), value.ID); err == nil {
		t.Fatal("expected log preparation failure")
	}
	got, err := db.Instance(context.Background(), value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if engine.created || got.ActualState != domain.StateFaulted || strings.Join(events, ",") != "logs:failed-logs" {
		t.Fatalf("engine=%#v instance=%#v events=%v", engine, got, events)
	}
}

func TestStartLeavesLogDirectoryOwnershipToLogPreparer(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	value := domain.Instance{ID: "log-owner", NodeID: "local", Name: "log owner", GamePort: 27015, StartMap: "map", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", ActualState: domain.StateStopped}
	if err := db.CreateInstance(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	events := []string{}
	base := filepath.Join(root, "instances", value.ID)
	preparer := fakeLogPreparer{events: &events, check: func() error {
		if _, err := os.Lstat(filepath.Join(base, "logs")); !os.IsNotExist(err) {
			return errors.New("lifecycle created logs before preparer")
		}
		return os.MkdirAll(filepath.Join(base, "logs", "game"), 0o750)
	}}
	if err := New(db, &fakeEngine{}, freePorts{}, root, WithLogPreparer(preparer)).Start(context.Background(), value.ID); err != nil {
		t.Fatal(err)
	}
}

func TestStartProvisionsUninstalledInstanceWhenPackageIsAlreadyMarkedApplied(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	value := domain.Instance{ID: "preapplied", NodeID: "local", Name: "preapplied", GamePort: 27015, StartMap: "map", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", SelectedPackageID: "package-a", PackageVersion: "package-a", ActualState: domain.StateUninstalled}
	if err := db.CreateInstance(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	events := []string{}
	engine := &fakeEngine{events: &events}
	service := New(db, engine, freePorts{}, root, WithProvisioner(fakeProvisioner{repo: db, events: &events}))
	if err := service.Start(context.Background(), value.ID); err != nil {
		t.Fatal(err)
	}
	if strings.Join(events, ",") != "prepare,create,start" {
		t.Fatalf("events=%v", events)
	}
}

func TestStartDoesNotCreateContainerWhenProvisioningFails(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	value := domain.Instance{ID: "failed-prepare", NodeID: "local", Name: "failed prepare", GamePort: 27015, StartMap: "map", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", SelectedPackageID: "package-a", ActualState: domain.StateUninstalled}
	if err := db.CreateInstance(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	events := []string{}
	engine := &fakeEngine{events: &events}
	service := New(db, engine, freePorts{}, root, WithProvisioner(fakeProvisioner{repo: db, events: &events, err: errors.New("install failed")}))
	if err := service.Start(context.Background(), value.ID); err == nil {
		t.Fatal("expected provisioning failure")
	}
	got, err := db.Instance(context.Background(), value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if engine.created || got.ActualState != domain.StateFaulted {
		t.Fatalf("engine=%#v instance=%#v", engine, got)
	}
}
func TestStartCreatesContainerPersistsIDAndStarts(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	v := domain.Instance{ID: "abc", NodeID: "local", Name: "one", GamePort: 27015, StartMap: "c2m1_highway", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime:v1", DesiredState: domain.StateStopped, ActualState: domain.StateUninstalled}
	if err := db.CreateInstance(context.Background(), v); err != nil {
		t.Fatal(err)
	}
	engine := &fakeEngine{}
	svc := New(db, engine, freePorts{}, root)
	if err := svc.Start(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
	got, _ := db.Instance(context.Background(), "abc")
	if !engine.created || !engine.started || got.ContainerID != "container-1" || got.ActualState != domain.StateRunning {
		t.Fatalf("engine=%#v instance=%#v", engine, got)
	}
}

func TestStartChecksEveryDeclaredPort(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	v := domain.Instance{ID: "ports", NodeID: "local", Name: "ports", GamePort: 27015, SourceTVPort: 27020, PluginPorts: []int{27021, 27022}, StartMap: "map", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", ActualState: domain.StateStopped}
	if err := db.CreateInstance(context.Background(), v); err != nil {
		t.Fatal(err)
	}
	checker := &recordingPorts{}
	if err := New(db, &fakeEngine{}, checker, root).Start(context.Background(), v.ID); err != nil {
		t.Fatal(err)
	}
	if len(checker.checked) != 4 || checker.checked[0] != 27015 || checker.checked[1] != 27020 || checker.checked[2] != 27021 || checker.checked[3] != 27022 {
		t.Fatalf("checked=%v", checker.checked)
	}
}
func TestStartRejectsInstallWhenDiskSpaceIsLow(t *testing.T) {
	root := t.TempDir()
	db, _ := store.Open(filepath.Join(root, "panel.db"))
	defer db.Close()
	v := domain.Instance{ID: "abc", NodeID: "local", Name: "one", GamePort: 27015, StartMap: "map", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", ActualState: domain.StateUninstalled}
	_ = db.CreateInstance(context.Background(), v)
	engine := &fakeEngine{}
	err := New(db, engine, freePorts{}, root, WithSpace(fixedSpace(10), 100)).Start(context.Background(), "abc")
	if err == nil || engine.created {
		t.Fatalf("err=%v engine=%#v", err, engine)
	}
}
func TestStopUsesSupervisorBeforeDocker(t *testing.T) {
	root := t.TempDir()
	db, _ := store.Open(filepath.Join(root, "panel.db"))
	defer db.Close()
	v := domain.Instance{ID: "abc", NodeID: "local", Name: "one", ContainerID: "container-1", GamePort: 27015, StartMap: "map", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime:v1", DesiredState: domain.StateRunning, ActualState: domain.StateRunning}
	_ = db.CreateInstance(context.Background(), v)
	engine := &fakeEngine{}
	if err := New(db, engine, freePorts{}, root).Stop(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
	got, _ := db.Instance(context.Background(), "abc")
	if engine.execOperation != "stop" || !engine.stopped || got.ActualState != domain.StateStopped {
		t.Fatalf("engine=%#v instance=%#v", engine, got)
	}
}

func TestRestartContinuesWhenSupervisorAlreadyStoppedContainer(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	instance := domain.Instance{ID: "abc", NodeID: "local", Name: "one", ContainerID: "container-1", GamePort: 27015, StartMap: "map", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime:v1", DesiredState: domain.StateRunning, ActualState: domain.StateRunning}
	if err := db.CreateInstance(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	started := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1.44/containers/json":
			_ = json.NewEncoder(w).Encode([]docker.Container{})
		case r.Method == http.MethodPost && r.URL.Path == "/v1.44/containers/container-1/exec":
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": "exec-stop"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1.44/exec/exec-stop/start":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/v1.44/containers/container-1/stop":
			w.WriteHeader(http.StatusNotModified)
		case r.Method == http.MethodPost && r.URL.Path == "/v1.44/containers/container-1/start":
			started = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	service := New(db, docker.NewEngine(server.URL), freePorts{}, root)
	if err := service.Restart(context.Background(), instance.ID); err != nil {
		t.Fatal(err)
	}
	got, err := db.Instance(context.Background(), instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !started || got.DesiredState != domain.StateRunning || got.ActualState != domain.StateRunning {
		t.Fatalf("started=%v instance=%#v", started, got)
	}
}

func TestReconcileMarksMissingAndReturnsUnknownContainers(t *testing.T) {
	root := t.TempDir()
	db, _ := store.Open(filepath.Join(root, "panel.db"))
	defer db.Close()
	missing := domain.Instance{ID: "missing", NodeID: "local", Name: "missing", GamePort: 27015, StartMap: "map", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", DesiredState: domain.StateRunning, ActualState: domain.StateRunning}
	known := missing
	known.ID = "known"
	known.Name = "known"
	known.GamePort = 27016
	_ = db.CreateInstance(context.Background(), missing)
	_ = db.CreateInstance(context.Background(), known)
	engine := &fakeEngine{containers: []docker.Container{{ID: "known-container", State: "running", Labels: map[string]string{docker.ManagedLabel: "true", docker.InstanceLabel: "known", docker.RoleLabel: "game", docker.GameLogMountsLabel: docker.GameLogMountsVersion}}, {ID: "unknown-container", State: "running", Labels: map[string]string{docker.ManagedLabel: "true", docker.InstanceLabel: "unknown", docker.RoleLabel: "game"}}}}
	unknown, err := New(db, engine, freePorts{}, root).Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	gotMissing, _ := db.Instance(context.Background(), "missing")
	gotKnown, _ := db.Instance(context.Background(), "known")
	if gotMissing.ActualState != domain.StateOrphaned || gotKnown.ContainerID != "known-container" || gotKnown.ActualState != domain.StateRunning || len(unknown) != 1 || unknown[0].ID != "unknown-container" {
		t.Fatalf("missing=%#v known=%#v unknown=%#v", gotMissing, gotKnown, unknown)
	}
}

func TestReconcileRebuildsLegacyGameContainersWithPersistentLogMounts(t *testing.T) {
	for _, tc := range []struct {
		name, containerState, mountVersion string
		desired, actual                    domain.InstanceState
		wantEvents                         string
	}{
		{name: "running", containerState: "running", desired: domain.StateRunning, actual: domain.StateRunning, wantEvents: "logs:abc,stop,remove,logs:abc,create,start"},
		{name: "stopped", containerState: "exited", desired: domain.StateStopped, actual: domain.StateStopped, wantEvents: "logs:abc,remove,logs:abc,create"},
		{name: "v0", containerState: "exited", mountVersion: "v0", desired: domain.StateStopped, actual: domain.StateStopped, wantEvents: "logs:abc,remove,logs:abc,create"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			db, _ := store.Open(filepath.Join(root, "panel.db"))
			defer db.Close()
			instance := domain.Instance{ID: "abc", NodeID: "local", Name: "one", ContainerID: "old", GamePort: 27015, StartMap: "map", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", DesiredState: tc.desired, ActualState: tc.actual}
			if err := db.CreateInstance(context.Background(), instance); err != nil {
				t.Fatal(err)
			}
			events := []string{}
			engine := &fakeEngine{events: &events, containers: []docker.Container{{ID: "old", State: tc.containerState, Labels: map[string]string{docker.ManagedLabel: "true", docker.InstanceLabel: "abc", docker.RoleLabel: "game", docker.GameLogMountsLabel: tc.mountVersion}}}}
			service := New(db, engine, freePorts{}, root, WithLogPreparer(fakeLogPreparer{events: &events}))
			if _, err := service.Reconcile(context.Background()); err != nil {
				t.Fatal(err)
			}
			got, _ := db.Instance(context.Background(), "abc")
			if strings.Join(events, ",") != tc.wantEvents || got.DesiredState != tc.desired || got.ActualState != tc.actual || got.ContainerID != "container-1" {
				t.Fatalf("events=%v instance=%#v", events, got)
			}
			if engine.createdSpec.Labels[docker.GameLogMountsLabel] != docker.GameLogMountsVersion || !strings.Contains(strings.Join(engine.createdSpec.Mounts, "|"), filepath.Join(root, "instances", "abc", "logs", "game")) {
				t.Fatalf("created spec=%#v", engine.createdSpec)
			}
		})
	}
}

func TestReconcileLegacyRunningContainerChecksHealthOnlyAfterRebuild(t *testing.T) {
	root := t.TempDir()
	db, _ := store.Open(filepath.Join(root, "panel.db"))
	defer db.Close()
	instance := domain.Instance{ID: "abc", NodeID: "local", Name: "one", ContainerID: "old", GamePort: 27015, RuntimeImage: "runtime", DesiredState: domain.StateRunning, ActualState: domain.StateRunning}
	_ = db.CreateInstance(context.Background(), instance)
	engine := &fakeEngine{containers: []docker.Container{{ID: "old", State: "running", Labels: map[string]string{docker.ManagedLabel: "true", docker.InstanceLabel: "abc", docker.RoleLabel: "game"}}}}
	health := &controlledHealth{release: make(chan struct{})}
	defer close(health.release)
	if _, err := New(db, engine, freePorts{}, root, WithHealth(health)).Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, _ := db.Instance(context.Background(), "abc")
	if calls := health.recorded(); len(calls) != 1 || calls[0] != "container-1" || got.ContainerID != "container-1" || got.ActualState != domain.StateRunning {
		t.Fatalf("health calls=%v instance=%#v", calls, got)
	}
}

func TestReconcileLegacyUpgradePreparationFailureKeepsContainerRetryable(t *testing.T) {
	root := t.TempDir()
	db, _ := store.Open(filepath.Join(root, "panel.db"))
	defer db.Close()
	instance := domain.Instance{ID: "abc", NodeID: "local", Name: "one", ContainerID: "old", GamePort: 27015, RuntimeImage: "runtime", DesiredState: domain.StateRunning, ActualState: domain.StateRunning}
	_ = db.CreateInstance(context.Background(), instance)
	events := []string{}
	engine := &fakeEngine{events: &events, containers: []docker.Container{{ID: "old", State: "running", Labels: map[string]string{docker.ManagedLabel: "true", docker.InstanceLabel: "abc", docker.RoleLabel: "game"}}}}
	_, err := New(db, engine, freePorts{}, root, WithLogPreparer(fakeLogPreparer{events: &events, err: errors.New("migration failed")})).Reconcile(context.Background())
	got, _ := db.Instance(context.Background(), "abc")
	if err == nil || !strings.Contains(err.Error(), "upgrade legacy game container abc") || engine.removed || got.ContainerID != "old" || got.DesiredState != domain.StateRunning {
		t.Fatalf("err=%v engine=%#v instance=%#v", err, engine, got)
	}
}

func TestReconcileLeavesCurrentGameContainerUntouched(t *testing.T) {
	root := t.TempDir()
	db, _ := store.Open(filepath.Join(root, "panel.db"))
	defer db.Close()
	instance := domain.Instance{ID: "abc", NodeID: "local", Name: "one", GamePort: 27015, RuntimeImage: "runtime", DesiredState: domain.StateStopped, ActualState: domain.StateStopped}
	_ = db.CreateInstance(context.Background(), instance)
	engine := &fakeEngine{containers: []docker.Container{{ID: "current", State: "exited", Labels: map[string]string{docker.ManagedLabel: "true", docker.InstanceLabel: "abc", docker.RoleLabel: "game", docker.GameLogMountsLabel: docker.GameLogMountsVersion}}}}
	if _, err := New(db, engine, freePorts{}, root).Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if engine.removed || engine.created {
		t.Fatalf("current container was rebuilt: %#v", engine)
	}
}

func TestReconcileCurrentRunningContainerKeepsAsynchronousHealthCheck(t *testing.T) {
	root := t.TempDir()
	db, _ := store.Open(filepath.Join(root, "panel.db"))
	defer db.Close()
	instance := domain.Instance{ID: "abc", NodeID: "local", Name: "one", GamePort: 27015, RuntimeImage: "runtime", DesiredState: domain.StateRunning, ActualState: domain.StateRunning}
	_ = db.CreateInstance(context.Background(), instance)
	engine := &fakeEngine{containers: []docker.Container{{ID: "current", State: "running", Labels: map[string]string{docker.ManagedLabel: "true", docker.InstanceLabel: "abc", docker.RoleLabel: "game", docker.GameLogMountsLabel: docker.GameLogMountsVersion}}}}
	called := make(chan string, 1)
	health := &controlledHealth{release: make(chan struct{}), called: called}
	if _, err := New(db, engine, freePorts{}, root, WithHealth(health)).Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-called:
		if id != "current" {
			t.Fatalf("health checked %q", id)
		}
	case <-time.After(time.Second):
		t.Fatal("current container health check did not run asynchronously")
	}
	if engine.removed || engine.created {
		t.Fatalf("current container was rebuilt: %#v", engine)
	}
}

func TestReconcileDoesNotAdoptContainerWithoutValidRole(t *testing.T) {
	root := t.TempDir()
	db, _ := store.Open(filepath.Join(root, "panel.db"))
	defer db.Close()
	instance := domain.Instance{ID: "abc", NodeID: "local", Name: "one", ContainerID: "trusted-game", GamePort: 27015, StartMap: "map", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", DesiredState: domain.StateStopped, ActualState: domain.StateStopped}
	_ = db.CreateInstance(context.Background(), instance)
	invalid := docker.Container{ID: "missing-role", State: "running", Labels: map[string]string{docker.ManagedLabel: "true", docker.InstanceLabel: "abc"}}
	unknown, err := New(db, &fakeEngine{containers: []docker.Container{invalid}}, freePorts{}, root).Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got, _ := db.Instance(context.Background(), "abc")
	if got.ContainerID != "trusted-game" || len(unknown) != 1 || unknown[0].ID != invalid.ID {
		t.Fatalf("instance=%#v unknown=%#v", got, unknown)
	}
}

func TestReconcileSeparatesMaintenanceWriterFromGameContainer(t *testing.T) {
	root := t.TempDir()
	db, _ := store.Open(filepath.Join(root, "panel.db"))
	defer db.Close()
	instance := domain.Instance{ID: "abc", NodeID: "local", Name: "one", ContainerID: "old-game", GamePort: 27015, StartMap: "map", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", DesiredState: domain.StateRunning, ActualState: domain.StateRunning}
	_ = db.CreateInstance(context.Background(), instance)
	engine := &fakeEngine{containers: []docker.Container{
		{ID: "game", State: "exited", Labels: map[string]string{docker.ManagedLabel: "true", docker.InstanceLabel: "abc", docker.RoleLabel: "game"}},
		{ID: "maintenance", State: "running", Labels: map[string]string{docker.ManagedLabel: "true", docker.InstanceLabel: "abc", docker.RoleLabel: "maintenance"}},
	}}
	unknown, err := New(db, engine, freePorts{}, root).Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got, _ := db.Instance(context.Background(), "abc")
	if got.ContainerID != "game" || got.ActualState != domain.StateUpdating || got.DesiredState != domain.StateRunning || len(unknown) != 0 {
		t.Fatalf("instance=%#v unknown=%#v", got, unknown)
	}
}

func TestLifecycleMutationsRejectSameInstanceMaintenanceWriter(t *testing.T) {
	operations := map[string]func(*Service, context.Context, string) error{
		"start":   func(s *Service, ctx context.Context, id string) error { return s.Start(ctx, id) },
		"stop":    func(s *Service, ctx context.Context, id string) error { return s.Stop(ctx, id) },
		"rebuild": func(s *Service, ctx context.Context, id string) error { return s.Rebuild(ctx, id) },
		"delete":  func(s *Service, ctx context.Context, id string) error { return s.Delete(ctx, id, false) },
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			db, _ := store.Open(filepath.Join(root, "panel.db"))
			defer db.Close()
			instance := domain.Instance{ID: "abc", NodeID: "local", Name: "one", ContainerID: "game", GamePort: 27015, StartMap: "map", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", DesiredState: domain.StateRunning, ActualState: domain.StateRunning}
			_ = db.CreateInstance(context.Background(), instance)
			engine := &fakeEngine{containers: []docker.Container{{ID: "writer", State: "running", Labels: map[string]string{docker.ManagedLabel: "true", docker.InstanceLabel: "abc", docker.RoleLabel: "maintenance"}}}}
			err := operation(New(db, engine, freePorts{}, root), context.Background(), "abc")
			if err == nil || !errors.Is(err, ErrMaintenanceActive) {
				t.Fatalf("err=%v", err)
			}
			if engine.started || engine.stopped || engine.removed {
				t.Fatalf("mutation reached Docker despite writer: %#v", engine)
			}
		})
	}
}

func TestRebuildReplacesContainerButKeepsPersistentData(t *testing.T) {
	root := t.TempDir()
	db, _ := store.Open(filepath.Join(root, "panel.db"))
	defer db.Close()
	v := domain.Instance{ID: "abc", NodeID: "local", Name: "one", ContainerID: "old", GamePort: 27015, StartMap: "map", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", DesiredState: domain.StateRunning, ActualState: domain.StateStarting}
	_ = db.CreateInstance(context.Background(), v)
	game := filepath.Join(root, "instances", "abc", "game")
	_ = os.MkdirAll(game, 0750)
	_ = os.WriteFile(filepath.Join(game, "keep"), []byte("data"), 0640)
	engine := &fakeEngine{}
	if err := New(db, engine, freePorts{}, root).Rebuild(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
	got, _ := db.Instance(context.Background(), "abc")
	if !engine.removed || got.ContainerID != "container-1" || !engine.started {
		t.Fatalf("engine=%#v got=%#v", engine, got)
	}
	if _, err := os.Stat(filepath.Join(game, "keep")); err != nil {
		t.Fatal("persistent game data removed")
	}
}

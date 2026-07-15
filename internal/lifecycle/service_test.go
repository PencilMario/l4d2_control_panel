package lifecycle

import (
	"context"
	"errors"
	"github.com/not0721here/l4d2-control-panel/internal/docker"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/store"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeEngine struct {
	created, started, stopped bool
	execOperation             string
	containers                []docker.Container
	removed                   bool
	events                    *[]string
}

func (f *fakeEngine) Create(context.Context, docker.ContainerSpec) (string, error) {
	f.created = true
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
func (f *fakeEngine) Stop(context.Context, string, int) error { f.stopped = true; return nil }
func (f *fakeEngine) ListManaged(context.Context) ([]docker.Container, error) {
	return f.containers, nil
}
func (f *fakeEngine) Remove(context.Context, string) error { f.removed = true; return nil }

type freePorts struct{}

func (freePorts) Available(context.Context, string, []int) error { return nil }

type recordingPorts struct{ checked []int }

func (p *recordingPorts) Available(_ context.Context, _ string, ports []int) error {
	p.checked = append(p.checked, ports...)
	return nil
}

type fixedSpace uint64

func (s fixedSpace) Available(string) (uint64, error) { return uint64(s), nil }

type fakeProvisioner struct {
	repo   *store.Store
	events *[]string
	err    error
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
	service := New(db, engine, freePorts{}, root, WithProvisioner(fakeProvisioner{repo: db, events: &events}))
	if err := service.Start(context.Background(), value.ID); err != nil {
		t.Fatal(err)
	}
	if strings.Join(events, ",") != "prepare,create,start" {
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
	engine := &fakeEngine{containers: []docker.Container{{ID: "known-container", State: "running", Labels: map[string]string{docker.ManagedLabel: "true", docker.InstanceLabel: "known", docker.RoleLabel: "game"}}, {ID: "unknown-container", State: "running", Labels: map[string]string{docker.ManagedLabel: "true", docker.InstanceLabel: "unknown", docker.RoleLabel: "game"}}}}
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

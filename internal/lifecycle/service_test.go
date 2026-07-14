package lifecycle

import (
	"context"
	"github.com/not0721here/l4d2-control-panel/internal/docker"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/store"
	"os"
	"path/filepath"
	"testing"
)

type fakeEngine struct {
	created, started, stopped bool
	execOperation             string
	containers                []docker.Container
	removed                   bool
}

func (f *fakeEngine) Create(context.Context, docker.ContainerSpec) (string, error) {
	f.created = true
	return "container-1", nil
}
func (f *fakeEngine) Start(context.Context, string) error { f.started = true; return nil }
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

func (freePorts) Available(int) error { return nil }

type fixedSpace uint64

func (s fixedSpace) Available(string) (uint64, error) { return uint64(s), nil }
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
	engine := &fakeEngine{containers: []docker.Container{{ID: "known-container", State: "running", Labels: map[string]string{docker.ManagedLabel: "true", docker.InstanceLabel: "known"}}, {ID: "unknown-container", State: "running", Labels: map[string]string{docker.ManagedLabel: "true", docker.InstanceLabel: "unknown"}}}}
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

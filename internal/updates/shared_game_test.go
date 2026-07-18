package updates

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/maintenance"
)

type sharedGameRepo struct {
	instances []domain.Instance
	state     domain.SharedGameState
}

func (r *sharedGameRepo) Instances(context.Context) ([]domain.Instance, error) {
	return r.instances, nil
}
func (r *sharedGameRepo) SaveSharedGameState(_ context.Context, state domain.SharedGameState) error {
	r.state = state
	return nil
}
func (r *sharedGameRepo) SharedGameState(context.Context) (domain.SharedGameState, error) {
	return r.state, nil
}

type sharedPlayers struct{ online map[string]int }

func (p sharedPlayers) PlayerCount(_ context.Context, id string) (int, error) {
	return p.online[id], nil
}

type sharedInstaller struct {
	events   *[]string
	target   string
	validate bool
}

func (i *sharedInstaller) InstallSharedGame(_ context.Context, _ string, target string, _ domain.Instance, validate bool) error {
	*i.events = append(*i.events, "install")
	i.target = target
	i.validate = validate
	return nil
}

type sharedReconciler struct {
	events *[]string
	fail   string
}

func (r sharedReconciler) Unmount(_ context.Context, instance domain.Instance, _ string) error {
	*r.events = append(*r.events, "unmount:"+instance.ID)
	return nil
}

func (r sharedReconciler) Switch(_ context.Context, instance domain.Instance, _, _ string) error {
	*r.events = append(*r.events, "switch:"+instance.ID)
	if r.fail == instance.ID {
		return errors.New("switch failed")
	}
	return nil
}

type sharedLife struct{ events *[]string }

func (l sharedLife) Stop(_ context.Context, id string) error {
	*l.events = append(*l.events, "stop:"+id)
	return nil
}
func (l sharedLife) Start(_ context.Context, id string) error {
	*l.events = append(*l.events, "start:"+id)
	return nil
}

func TestSharedGameUpdateStopsAndSwitchesEveryInstance(t *testing.T) {
	events := []string{}
	installer := &sharedInstaller{events: &events}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "game", "releases", "old"), 0o755); err != nil {
		t.Fatal(err)
	}
	repo := &sharedGameRepo{instances: []domain.Instance{{ID: "a", RuntimeImage: "runtime", DesiredState: domain.StateRunning, ActualState: domain.StateRunning}, {ID: "b", RuntimeImage: "runtime", DesiredState: domain.StateStopped, ActualState: domain.StateStopped}}, state: domain.SharedGameState{ActiveReleaseID: "old", MigrationState: "ready"}}
	coordinator := SharedGameCoordinator{Root: root, Instances: repo, Players: sharedPlayers{online: map[string]int{}}, Installer: installer, Reconciler: sharedReconciler{events: &events}, Lifecycle: sharedLife{&events}, Gate: maintenance.NewGate()}
	if err := coordinator.Update(context.Background(), "force"); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(events, ",")
	if got != "stop:a,unmount:a,unmount:b,install,switch:a,switch:b,start:a" {
		t.Fatalf("events=%s", got)
	}
	if repo.state.ActiveReleaseID != "old" || repo.state.PreviousReleaseID != "" || repo.state.OperationStage != "" {
		t.Fatalf("state=%#v", repo.state)
	}
	if !installer.validate || installer.target != filepath.Join(coordinator.Root, "game", "releases", "old") {
		t.Fatalf("installer target=%q validate=%v", installer.target, installer.validate)
	}
}

func TestSharedGameUpdateSkipBlocksWhenAnyInstanceOnline(t *testing.T) {
	events := []string{}
	repo := &sharedGameRepo{instances: []domain.Instance{{ID: "a", ActualState: domain.StateRunning}, {ID: "b", ActualState: domain.StateRunning}}, state: domain.SharedGameState{ActiveReleaseID: "old", MigrationState: "ready"}}
	coordinator := SharedGameCoordinator{Root: t.TempDir(), Instances: repo, Players: sharedPlayers{online: map[string]int{"b": 1}}, Installer: &sharedInstaller{events: &events}, Reconciler: sharedReconciler{events: &events}, Lifecycle: sharedLife{&events}, Gate: maintenance.NewGate()}
	if err := coordinator.Update(context.Background(), "skip"); err == nil {
		t.Fatal("online instance did not block")
	}
	if len(events) != 0 {
		t.Fatalf("events=%v", events)
	}
}

func TestSharedGameUpdateRollsSwitchedInstancesBack(t *testing.T) {
	events := []string{}
	repo := &sharedGameRepo{instances: []domain.Instance{{ID: "a", ActualState: domain.StateStopped}, {ID: "b", ActualState: domain.StateStopped}}, state: domain.SharedGameState{ActiveReleaseID: "old", MigrationState: "ready"}}
	coordinator := SharedGameCoordinator{Root: t.TempDir(), Instances: repo, Players: sharedPlayers{}, Installer: &sharedInstaller{events: &events}, Reconciler: sharedReconciler{events: &events, fail: "b"}, Lifecycle: sharedLife{&events}, Gate: maintenance.NewGate()}
	if err := coordinator.Update(context.Background(), "force"); err == nil {
		t.Fatal("switch failure accepted")
	}
	if !strings.HasSuffix(strings.Join(events, ","), "switch:a,switch:b") {
		t.Fatalf("events=%v", events)
	}
	if repo.state.MigrationState != "failed" || repo.state.OperationStage != "failed" {
		t.Fatalf("state=%#v", repo.state)
	}
}

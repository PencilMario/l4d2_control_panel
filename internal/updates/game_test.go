package updates

import (
	"context"
	"errors"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"strings"
	"testing"
)

type gameRepo struct{ instance domain.Instance }

func (r *gameRepo) Instance(context.Context, string) (domain.Instance, error) { return r.instance, nil }
func (r *gameRepo) UpdateInstance(_ context.Context, v domain.Instance) error {
	r.instance = v
	return nil
}

type gameUpdater struct {
	events      *[]string
	maintenance bool
	err         error
	beforeError func()
}

func (u gameUpdater) HasMaintenance(context.Context, string) (bool, error) {
	return u.maintenance, nil
}

func (u gameUpdater) UpdateGame(context.Context, string, domain.Instance) error {
	*u.events = append(*u.events, "steamcmd")
	if u.beforeError != nil {
		u.beforeError()
	}
	return u.err
}

type privateApplier struct{ events *[]string }

func (p privateApplier) Apply(context.Context, string) error {
	*p.events = append(*p.events, "private")
	return nil
}

type orderedLife struct{ events *[]string }

func (l orderedLife) Stop(context.Context, string) error {
	*l.events = append(*l.events, "stop")
	return nil
}
func (l orderedLife) Start(context.Context, string) error {
	*l.events = append(*l.events, "start")
	return nil
}

type gamePackages struct {
	item content.PackageVersion
	err  error
}

func (p gamePackages) Get(string) (content.PackageVersion, error) { return p.item, p.err }

type gameDeployment struct {
	events    *[]string
	commitErr error
}

func (d gameDeployment) Commit() error {
	*d.events = append(*d.events, "commit")
	return d.commitErr
}
func (d gameDeployment) Rollback() error {
	*d.events = append(*d.events, "rollback")
	return nil
}

type gameDeployer struct {
	events    *[]string
	commitErr error
}

func (d gameDeployer) Begin(_ context.Context, _ string, _ string, _ string, mode Mode) (Deployment, error) {
	*d.events = append(*d.events, "deploy:"+string(mode))
	return gameDeployment{events: d.events, commitErr: d.commitErr}, nil
}

func TestGameReinstallPackageOnlyForcesFullDeployment(t *testing.T) {
	events := []string{}
	repo := &gameRepo{instance: domain.Instance{ID: "abc", SelectedPackageID: "pkg", PackageVersion: "pkg", DesiredState: domain.StateStopped, ActualState: domain.StateStopped}}
	coordinator := GameCoordinator{
		Instances: repo,
		Lifecycle: orderedLife{&events},
		Private:   privateApplier{&events},
		Packages:  gamePackages{item: content.PackageVersion{ID: "pkg", ArchivePath: "package.zip", Version: "v1"}},
		Deployer:  gameDeployer{events: &events},
	}
	if err := coordinator.Reinstall(context.Background(), "abc", ReinstallOptions{Package: true}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(events, ","); got != "deploy:full,commit" {
		t.Fatalf("events=%s", got)
	}
}

func TestGameReinstallCombinedStopsAndStartsOnce(t *testing.T) {
	events := []string{}
	repo := &gameRepo{instance: domain.Instance{ID: "abc", SelectedPackageID: "pkg", PackageVersion: "pkg", DesiredState: domain.StateRunning, ActualState: domain.StateRunning}}
	coordinator := GameCoordinator{
		Root:      "/data",
		Instances: repo,
		Lifecycle: orderedLife{&events},
		Updater:   gameUpdater{events: &events},
		Private:   privateApplier{&events},
		Packages:  gamePackages{item: content.PackageVersion{ID: "pkg", ArchivePath: "package.zip", Version: "v1"}},
		Deployer:  gameDeployer{events: &events},
	}
	if err := coordinator.Reinstall(context.Background(), "abc", ReinstallOptions{Game: true, Package: true}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(events, ","); got != "stop,steamcmd,deploy:full,start,commit" {
		t.Fatalf("events=%s", got)
	}
}

func TestGameReinstallRollsBackCommittedPackageFailureWhileStopped(t *testing.T) {
	events := []string{}
	want := errors.New("commit failed")
	repo := &gameRepo{instance: domain.Instance{ID: "abc", SelectedPackageID: "pkg", PackageVersion: "pkg", DesiredState: domain.StateRunning, ActualState: domain.StateRunning}}
	coordinator := GameCoordinator{
		Instances: repo,
		Lifecycle: orderedLife{&events},
		Private:   privateApplier{&events},
		Packages:  gamePackages{item: content.PackageVersion{ID: "pkg", ArchivePath: "package.zip", Version: "v1"}},
		Deployer:  gameDeployer{events: &events, commitErr: want},
	}
	err := coordinator.Reinstall(context.Background(), "abc", ReinstallOptions{Package: true})
	if !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
	if got := strings.Join(events, ","); got != "stop,deploy:full,start,commit,stop,rollback,start" {
		t.Fatalf("events=%s", got)
	}
}

func TestGameReinstallPackageRequiresSelectedPackage(t *testing.T) {
	events := []string{}
	want := errors.New("package missing")
	repo := &gameRepo{instance: domain.Instance{ID: "abc", DesiredState: domain.StateStopped, ActualState: domain.StateStopped}}
	coordinator := GameCoordinator{Instances: repo, Lifecycle: orderedLife{&events}, Private: privateApplier{&events}, Packages: gamePackages{err: want}, Deployer: gameDeployer{events: &events}}
	err := coordinator.Reinstall(context.Background(), "abc", ReinstallOptions{Package: true})
	if !errors.Is(err, want) || repo.instance.ActualState != domain.StateFaulted {
		t.Fatalf("err=%v instance=%#v", err, repo.instance)
	}
}
func TestGameUpdateStopsValidatesReappliesAndStarts(t *testing.T) {
	events := []string{}
	repo := &gameRepo{instance: domain.Instance{ID: "abc", DesiredState: domain.StateRunning, ActualState: domain.StateRunning}}
	coordinator := GameCoordinator{Root: "/data", Instances: repo, Lifecycle: orderedLife{&events}, Updater: gameUpdater{events: &events}, Private: privateApplier{&events}}
	if err := coordinator.Update(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(events, ","); got != "stop,steamcmd,private,start" {
		t.Fatalf("events=%s", got)
	}
}

func TestGameUpdateAdoptsMaintenanceWithoutStoppingAgain(t *testing.T) {
	events := []string{}
	repo := &gameRepo{instance: domain.Instance{ID: "abc", DesiredState: domain.StateRunning, ActualState: domain.StateUpdating}}
	coordinator := GameCoordinator{Root: "/data", Instances: repo, Lifecycle: orderedLife{&events}, Updater: gameUpdater{events: &events, maintenance: true}, Private: privateApplier{&events}}
	if err := coordinator.Update(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(events, ","); got != "steamcmd,private,start" {
		t.Fatalf("events=%s", got)
	}
}

func TestGameUpdatePersistsRunningIntentBeforeSteamCMD(t *testing.T) {
	events := []string{}
	repo := &gameRepo{instance: domain.Instance{ID: "abc", ContainerID: "game", DesiredState: domain.StateRunning, ActualState: domain.StateRunning}}
	updater := gameUpdater{events: &events, beforeError: func() {
		if repo.instance.DesiredState != domain.StateRunning || repo.instance.ActualState != domain.StateUpdating || repo.instance.ContainerID != "game" {
			t.Fatalf("instance was not checkpointed before SteamCMD: %#v", repo.instance)
		}
	}}
	coordinator := GameCoordinator{Root: "/data", Instances: repo, Lifecycle: orderedLife{&events}, Updater: updater, Private: privateApplier{&events}}
	if err := coordinator.Update(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
}

func TestGameUpdateLeavesDesiredStoppedInstanceStopped(t *testing.T) {
	events := []string{}
	repo := &gameRepo{instance: domain.Instance{ID: "abc", ContainerID: "game", DesiredState: domain.StateStopped, ActualState: domain.StateStopped}}
	coordinator := GameCoordinator{Root: "/data", Instances: repo, Lifecycle: orderedLife{&events}, Updater: gameUpdater{events: &events}, Private: privateApplier{&events}}
	if err := coordinator.Update(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(events, ","); got != "steamcmd,private" {
		t.Fatalf("events=%s", got)
	}
	if repo.instance.DesiredState != domain.StateStopped || repo.instance.ActualState != domain.StateStopped {
		t.Fatalf("instance=%#v", repo.instance)
	}
}

func TestGameUpdateFaultPreservesLatestInstanceFields(t *testing.T) {
	events := []string{}
	repo := &gameRepo{instance: domain.Instance{ID: "abc", ContainerID: "old", DesiredState: domain.StateRunning, ActualState: domain.StateUpdating}}
	want := errors.New("steam failed")
	updater := gameUpdater{events: &events, maintenance: true, err: want, beforeError: func() {
		repo.instance.ContainerID = "new"
		repo.instance.DesiredState = domain.StateStopped
	}}
	coordinator := GameCoordinator{Root: "/data", Instances: repo, Lifecycle: orderedLife{&events}, Updater: updater, Private: privateApplier{&events}}
	err := coordinator.Update(context.Background(), "abc")
	if !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
	if repo.instance.ContainerID != "new" || repo.instance.DesiredState != domain.StateStopped || repo.instance.ActualState != domain.StateFaulted {
		t.Fatalf("stale fault overwrote latest instance: %#v", repo.instance)
	}
}

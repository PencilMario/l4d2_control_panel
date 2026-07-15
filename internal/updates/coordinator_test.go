package updates

import (
	"context"
	"errors"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"strings"
	"testing"
)

type packageRepo struct {
	instance domain.Instance
}

func (r *packageRepo) Instance(context.Context, string) (domain.Instance, error) {
	return r.instance, nil
}

func (r *packageRepo) UpdateInstance(_ context.Context, value domain.Instance) error {
	r.instance = value
	return nil
}

type fakeLifecycle struct {
	events        []string
	startFailures int
}

func (f *fakeLifecycle) Start(context.Context, string) error {
	f.events = append(f.events, "start")
	if f.startFailures > 0 {
		f.startFailures--
		return errors.New("health check failed")
	}
	return nil
}
func (f *fakeLifecycle) Stop(context.Context, string) error {
	f.events = append(f.events, "stop")
	return nil
}

type fakeDeployer struct {
	life       *fakeLifecycle
	fail       bool
	commitFail bool
}

type fakeDeployment struct {
	life       *fakeLifecycle
	commitFail bool
}

func (f fakeDeployment) Commit() error {
	f.life.events = append(f.life.events, "commit")
	if f.commitFail {
		return errors.New("commit failed")
	}
	return nil
}
func (f fakeDeployment) Rollback() error {
	f.life.events = append(f.life.events, "rollback")
	return nil
}

func (f fakeDeployer) Begin(context.Context, string, string, string, Mode) (Deployment, error) {
	f.life.events = append(f.life.events, "deploy")
	if f.fail {
		return nil, errors.New("deploy failed")
	}
	return fakeDeployment{life: f.life, commitFail: f.commitFail}, nil
}
func TestFullCoordinatorStopsDeploysAndStarts(t *testing.T) {
	life := &fakeLifecycle{}
	repo := &packageRepo{instance: domain.Instance{ID: "abc", DesiredState: domain.StateRunning, ActualState: domain.StateRunning}}
	coordinator := Coordinator{Instances: repo, Lifecycle: life, Deployer: fakeDeployer{life: life}}
	err := coordinator.ApplyPackage(context.Background(), "abc", content.PackageVersion{ID: "pkg-v1", ArchivePath: "p.zip", Version: "v1"}, Full)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(life.events, ","); got != "stop,deploy,start,commit" {
		t.Fatalf("events=%s", got)
	}
	if repo.instance.SelectedPackageID != "pkg-v1" || repo.instance.PackageVersion != "pkg-v1" {
		t.Fatalf("instance=%#v", repo.instance)
	}
}

func TestFullCoordinatorKeepsStoppedInstanceStopped(t *testing.T) {
	life := &fakeLifecycle{}
	repo := &packageRepo{instance: domain.Instance{ID: "abc", DesiredState: domain.StateStopped, ActualState: domain.StateStopped, SelectedPackageID: "pkg-v2", PackageVersion: "pkg-v1"}}
	coordinator := Coordinator{Instances: repo, Lifecycle: life, Deployer: fakeDeployer{life: life}}
	if err := coordinator.ApplyPackage(context.Background(), "abc", content.PackageVersion{ID: "pkg-v2", ArchivePath: "p.zip", Version: "v2"}, Full); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(life.events, ","); got != "deploy,commit" {
		t.Fatalf("events=%s", got)
	}
	if repo.instance.PackageVersion != "pkg-v2" {
		t.Fatalf("instance=%#v", repo.instance)
	}
}

func TestHotCoordinatorMarksPackageAfterCommit(t *testing.T) {
	life := &fakeLifecycle{}
	repo := &packageRepo{instance: domain.Instance{ID: "abc", SelectedPackageID: "pkg-v1", PackageVersion: "pkg-v1"}}
	coordinator := Coordinator{Instances: repo, Lifecycle: life, Deployer: fakeDeployer{life: life}}
	if err := coordinator.ApplyPackage(context.Background(), "abc", content.PackageVersion{ID: "pkg-v2", ArchivePath: "p.zip", Version: "v2"}, Hot); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(life.events, ","); got != "deploy,commit" {
		t.Fatalf("events=%s", got)
	}
	if repo.instance.SelectedPackageID != "pkg-v2" || repo.instance.PackageVersion != "pkg-v2" {
		t.Fatalf("instance=%#v", repo.instance)
	}
}

func TestFullCoordinatorRollsBackWhenRestartHealthFails(t *testing.T) {
	life := &fakeLifecycle{startFailures: 1}
	repo := &packageRepo{instance: domain.Instance{ID: "abc", DesiredState: domain.StateRunning, ActualState: domain.StateRunning, SelectedPackageID: "pkg-v2", PackageVersion: "pkg-v1"}}
	coordinator := Coordinator{Instances: repo, Lifecycle: life, Deployer: fakeDeployer{life: life}}
	if err := coordinator.ApplyPackage(context.Background(), "abc", content.PackageVersion{ID: "pkg-v2", ArchivePath: "p.zip", Version: "v2"}, Full); err == nil {
		t.Fatal("expected health failure")
	}
	if got := strings.Join(life.events, ","); got != "stop,deploy,start,stop,rollback,start" {
		t.Fatalf("events=%s", got)
	}
	if repo.instance.PackageVersion != "pkg-v1" {
		t.Fatalf("instance=%#v", repo.instance)
	}
}
func TestFullCoordinatorRollsBackWhenJournalCommitFails(t *testing.T) {
	life := &fakeLifecycle{}
	repo := &packageRepo{instance: domain.Instance{ID: "abc", DesiredState: domain.StateRunning, ActualState: domain.StateRunning, SelectedPackageID: "pkg-v2", PackageVersion: "pkg-v1"}}
	coordinator := Coordinator{Instances: repo, Lifecycle: life, Deployer: fakeDeployer{life: life, commitFail: true}}
	if err := coordinator.ApplyPackage(context.Background(), "abc", content.PackageVersion{ID: "pkg-v2", ArchivePath: "p.zip", Version: "v2"}, Full); err == nil {
		t.Fatal("expected commit failure")
	}
	if got := strings.Join(life.events, ","); got != "stop,deploy,start,commit,stop,rollback,start" {
		t.Fatalf("events=%s", got)
	}
}
func TestFailedFullUpdateRestartsRolledBackInstance(t *testing.T) {
	life := &fakeLifecycle{}
	repo := &packageRepo{instance: domain.Instance{ID: "abc", DesiredState: domain.StateRunning, ActualState: domain.StateRunning, SelectedPackageID: "pkg-v2", PackageVersion: "pkg-v1"}}
	coordinator := Coordinator{Instances: repo, Lifecycle: life, Deployer: fakeDeployer{life: life, fail: true}}
	if err := coordinator.ApplyPackage(context.Background(), "abc", content.PackageVersion{ID: "pkg-v2", ArchivePath: "p.zip", Version: "v2"}, Full); err == nil {
		t.Fatal("expected failure")
	}
	if got := strings.Join(life.events, ","); got != "stop,deploy,start" {
		t.Fatalf("events=%s", got)
	}
}

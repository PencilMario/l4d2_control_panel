package updates

import (
	"context"
	"errors"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"strings"
	"testing"
)

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
	coordinator := Coordinator{Lifecycle: life, Deployer: fakeDeployer{life: life}}
	err := coordinator.ApplyPackage(context.Background(), "abc", content.PackageVersion{ArchivePath: "p.zip", Version: "v1"}, Full)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(life.events, ","); got != "stop,deploy,start,commit" {
		t.Fatalf("events=%s", got)
	}
}

func TestFullCoordinatorRollsBackWhenRestartHealthFails(t *testing.T) {
	life := &fakeLifecycle{startFailures: 1}
	coordinator := Coordinator{Lifecycle: life, Deployer: fakeDeployer{life: life}}
	if err := coordinator.ApplyPackage(context.Background(), "abc", content.PackageVersion{ArchivePath: "p.zip", Version: "v2"}, Full); err == nil {
		t.Fatal("expected health failure")
	}
	if got := strings.Join(life.events, ","); got != "stop,deploy,start,stop,rollback,start" {
		t.Fatalf("events=%s", got)
	}
}
func TestFullCoordinatorRollsBackWhenJournalCommitFails(t *testing.T) {
	life := &fakeLifecycle{}
	coordinator := Coordinator{Lifecycle: life, Deployer: fakeDeployer{life: life, commitFail: true}}
	if err := coordinator.ApplyPackage(context.Background(), "abc", content.PackageVersion{ArchivePath: "p.zip", Version: "v2"}, Full); err == nil {
		t.Fatal("expected commit failure")
	}
	if got := strings.Join(life.events, ","); got != "stop,deploy,start,commit,stop,rollback,start" {
		t.Fatalf("events=%s", got)
	}
}
func TestFailedFullUpdateRestartsRolledBackInstance(t *testing.T) {
	life := &fakeLifecycle{}
	coordinator := Coordinator{Lifecycle: life, Deployer: fakeDeployer{life: life, fail: true}}
	if err := coordinator.ApplyPackage(context.Background(), "abc", content.PackageVersion{ArchivePath: "p.zip", Version: "v1"}, Full); err == nil {
		t.Fatal("expected failure")
	}
	if got := strings.Join(life.events, ","); got != "stop,deploy,start" {
		t.Fatalf("events=%s", got)
	}
}

package updates

import (
	"context"
	"errors"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"strings"
	"testing"
)

type fakeLifecycle struct{ events []string }

func (f *fakeLifecycle) Start(context.Context, string) error {
	f.events = append(f.events, "start")
	return nil
}
func (f *fakeLifecycle) Stop(context.Context, string) error {
	f.events = append(f.events, "stop")
	return nil
}

type fakeDeployer struct {
	life *fakeLifecycle
	fail bool
}

func (f fakeDeployer) Apply(context.Context, string, string, string, Mode) error {
	f.life.events = append(f.life.events, "deploy")
	if f.fail {
		return errors.New("deploy failed")
	}
	return nil
}
func TestFullCoordinatorStopsDeploysAndStarts(t *testing.T) {
	life := &fakeLifecycle{}
	coordinator := Coordinator{Lifecycle: life, Deployer: fakeDeployer{life: life}}
	err := coordinator.ApplyPackage(context.Background(), "abc", content.PackageVersion{ArchivePath: "p.zip", Version: "v1"}, Full)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(life.events, ","); got != "stop,deploy,start" {
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

package provisioning

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/updates"
)

type fakeRepo struct {
	instance domain.Instance
}

func (r *fakeRepo) Instance(context.Context, string) (domain.Instance, error) {
	return r.instance, nil
}

func (r *fakeRepo) UpdateInstance(_ context.Context, value domain.Instance) error {
	r.instance = value
	return nil
}

type fakePackages struct {
	item content.PackageVersion
	err  error
}

func (f fakePackages) Get(string) (content.PackageVersion, error) {
	return f.item, f.err
}

type fakeDeployer struct {
	events *[]string
	err    error
}

type sharedStateRepo struct{ state domain.SharedGameState }

func (r sharedStateRepo) SharedGameState(context.Context) (domain.SharedGameState, error) {
	return r.state, nil
}

type fakeOverlay struct{ events *[]string }

func (o fakeOverlay) Ensure(_ context.Context, id, release string) error {
	*o.events = append(*o.events, "ensure:"+id+":"+release)
	return nil
}

func (f fakeDeployer) Apply(context.Context, string, string, string, updates.Mode) error {
	*f.events = append(*f.events, "deploy")
	return f.err
}

func TestPrepareDeploysSelectedPackage(t *testing.T) {
	events := []string{}
	repo := &fakeRepo{instance: domain.Instance{ID: "one", SelectedPackageID: "pkg"}}
	service := Service{
		Root:        t.TempDir(),
		Instances:   repo,
		SharedState: sharedStateRepo{state: domain.SharedGameState{ActiveReleaseID: "release-1", MigrationState: "ready"}},
		Overlay:     fakeOverlay{events: &events},
		Packages:    fakePackages{item: content.PackageVersion{ID: "pkg", ArchivePath: "pkg.zip", Version: "v1"}},
		Deployer:    fakeDeployer{events: &events},
	}
	if err := service.Prepare(context.Background(), repo.instance); err != nil {
		t.Fatal(err)
	}
	if strings.Join(events, ",") != "ensure:one:release-1,deploy" {
		t.Fatalf("events=%v", events)
	}
	if repo.instance.PackageVersion != "pkg" {
		t.Fatalf("instance=%#v", repo.instance)
	}
}

func TestPrepareDoesNotMarkPackageWhenDeploymentFails(t *testing.T) {
	events := []string{}
	repo := &fakeRepo{instance: domain.Instance{ID: "one", SelectedPackageID: "pkg"}}
	service := Service{
		Root:        t.TempDir(),
		Instances:   repo,
		SharedState: sharedStateRepo{state: domain.SharedGameState{ActiveReleaseID: "release-1", MigrationState: "ready"}},
		Overlay:     fakeOverlay{events: &events},
		Packages:    fakePackages{item: content.PackageVersion{ID: "pkg", ArchivePath: "pkg.zip", Version: "v1"}},
		Deployer:    fakeDeployer{events: &events, err: errors.New("deploy failed")},
	}
	if err := service.Prepare(context.Background(), repo.instance); err == nil {
		t.Fatal("expected deployment failure")
	}
	if repo.instance.PackageVersion != "" {
		t.Fatalf("instance=%#v", repo.instance)
	}
}

func TestPrepareRequiresSelectedPackage(t *testing.T) {
	service := Service{Instances: &fakeRepo{instance: domain.Instance{ID: "one"}}}
	if err := service.Prepare(context.Background(), domain.Instance{ID: "one"}); err == nil {
		t.Fatal("expected package selection failure")
	}
}

func TestPrepareUsesSharedGameWithoutInstallingPerInstance(t *testing.T) {
	events := []string{}
	repo := &fakeRepo{instance: domain.Instance{ID: "one", SelectedPackageID: "pkg"}}
	service := Service{Root: t.TempDir(), Instances: repo, SharedState: sharedStateRepo{state: domain.SharedGameState{ActiveReleaseID: "release-1", MigrationState: "ready"}}, Overlay: fakeOverlay{events: &events}, Packages: fakePackages{item: content.PackageVersion{ID: "pkg", ArchivePath: "pkg.zip", Version: "v1"}}, Deployer: fakeDeployer{events: &events}}
	if err := service.Prepare(context.Background(), repo.instance); err != nil {
		t.Fatal(err)
	}
	if strings.Join(events, ",") != "ensure:one:release-1,deploy" {
		t.Fatalf("events=%v", events)
	}
}

func TestPrepareRejectsMissingSharedGameWithoutInstallingPerInstance(t *testing.T) {
	events := []string{}
	repo := &fakeRepo{instance: domain.Instance{ID: "one", SelectedPackageID: "pkg"}}
	service := Service{Root: t.TempDir(), Instances: repo, SharedState: sharedStateRepo{}, Overlay: fakeOverlay{events: &events}, Packages: fakePackages{item: content.PackageVersion{ID: "pkg", ArchivePath: "pkg.zip", Version: "v1"}}, Deployer: fakeDeployer{events: &events}}
	if err := service.Prepare(context.Background(), repo.instance); err == nil {
		t.Fatal("expected missing shared game error")
	}
	if len(events) != 0 {
		t.Fatalf("events=%v", events)
	}
}

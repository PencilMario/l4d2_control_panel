package provisioning

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/updates"
)

type fakeRepo struct {
	instance  domain.Instance
	instances []domain.Instance
}

func (r *fakeRepo) Instance(context.Context, string) (domain.Instance, error) {
	return r.instance, nil
}

func (r *fakeRepo) UpdateInstance(_ context.Context, value domain.Instance) error {
	r.instance = value
	return nil
}

func (r *fakeRepo) Instances(context.Context) ([]domain.Instance, error) {
	if r.instances != nil {
		return r.instances, nil
	}
	return []domain.Instance{r.instance}, nil
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

type fakeAccelerator struct {
	events *[]string
	err    error
}

func (a fakeAccelerator) Ensure(_ context.Context, instance domain.Instance) error {
	*a.events = append(*a.events, "accelerator:"+instance.ID)
	return a.err
}

func (a fakeAccelerator) Reinstall(_ context.Context, instance domain.Instance) error {
	*a.events = append(*a.events, "accelerator-reinstall:"+instance.ID)
	return a.err
}

type sharedStateRepo struct{ state domain.SharedGameState }

func (r sharedStateRepo) SharedGameState(context.Context) (domain.SharedGameState, error) {
	return r.state, nil
}

type fakeOverlay struct {
	events *[]string
	failID string
}

func (o fakeOverlay) Ensure(_ context.Context, id, release string) error {
	*o.events = append(*o.events, "ensure:"+id+":"+release)
	if id == o.failID {
		return errors.New("mount failed")
	}
	return nil
}

func TestRecoverOverlaysEnsuresEveryInstanceUsesActiveRelease(t *testing.T) {
	events := []string{}
	repo := &fakeRepo{instances: []domain.Instance{{ID: "one"}, {ID: "two"}}}
	service := Service{
		Instances:   repo,
		SharedState: sharedStateRepo{state: domain.SharedGameState{ActiveReleaseID: "release-1", MigrationState: "ready"}},
		Overlay:     fakeOverlay{events: &events},
	}
	if err := service.RecoverOverlays(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(events, ","); got != "ensure:one:release-1,ensure:two:release-1" {
		t.Fatalf("events=%v", events)
	}
}

func TestRecoverOverlaysUsesCanonicalCurrentReleaseWhenStateIsFailed(t *testing.T) {
	events := []string{}
	root := t.TempDir()
	release := filepath.Join(root, "game", "releases", "release-1")
	if err := os.MkdirAll(release, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("releases", "release-1"), filepath.Join(root, "game", "current")); err != nil {
		t.Fatal(err)
	}
	service := Service{Root: root, Instances: &fakeRepo{instances: []domain.Instance{{ID: "one"}}}, SharedState: sharedStateRepo{state: domain.SharedGameState{MigrationState: "failed"}}, Overlay: fakeOverlay{events: &events}}
	if err := service.RecoverOverlays(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(events, ","); got != "ensure:one:release-1" {
		t.Fatalf("events=%v", events)
	}
}

func TestEnsureOverlayUsesCanonicalCurrentReleaseWhenStateIsFailed(t *testing.T) {
	events := []string{}
	root := t.TempDir()
	release := filepath.Join(root, "game", "releases", "release-1")
	if err := os.MkdirAll(release, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("releases", "release-1"), filepath.Join(root, "game", "current")); err != nil {
		t.Fatal(err)
	}
	service := Service{Root: root, SharedState: sharedStateRepo{state: domain.SharedGameState{MigrationState: "failed"}}, Overlay: fakeOverlay{events: &events}}
	if err := service.EnsureOverlay(context.Background(), domain.Instance{ID: "one"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(events, ","); got != "ensure:one:release-1" {
		t.Fatalf("events=%v", events)
	}
}

func TestRecoverOverlaysRejectsMissingStateWithoutCanonicalCurrentRelease(t *testing.T) {
	events := []string{}
	service := Service{Root: t.TempDir(), Instances: &fakeRepo{instances: []domain.Instance{{ID: "one"}}}, SharedState: sharedStateRepo{}, Overlay: fakeOverlay{events: &events}}
	if err := service.RecoverOverlays(context.Background()); err == nil || !strings.Contains(err.Error(), "shared game is not ready") {
		t.Fatalf("err=%v", err)
	}
}

func TestRecoverOverlaysReportsFailingInstance(t *testing.T) {
	events := []string{}
	repo := &fakeRepo{instances: []domain.Instance{{ID: "one"}, {ID: "two"}, {ID: "three"}}}
	service := Service{Instances: repo, SharedState: sharedStateRepo{state: domain.SharedGameState{ActiveReleaseID: "release-1", MigrationState: "ready"}}, Overlay: fakeOverlay{events: &events, failID: "two"}}
	if err := service.RecoverOverlays(context.Background()); err == nil || !strings.Contains(err.Error(), "two") {
		t.Fatalf("err=%v", err)
	}
	if got := strings.Join(events, ","); got != "ensure:one:release-1,ensure:two:release-1" {
		t.Fatalf("events=%v", events)
	}
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
		Accelerator: fakeAccelerator{events: &events},
	}
	if err := service.Prepare(context.Background(), repo.instance); err != nil {
		t.Fatal(err)
	}
	if strings.Join(events, ",") != "ensure:one:release-1,deploy,accelerator-reinstall:one" {
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

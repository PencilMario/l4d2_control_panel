package migration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

type migrationRepo struct {
	instances []domain.Instance
	state     domain.SharedGameState
}

func (r *migrationRepo) Instances(context.Context) ([]domain.Instance, error) {
	return r.instances, nil
}
func (r *migrationRepo) SaveSharedGameState(_ context.Context, s domain.SharedGameState) error {
	r.state = s
	return nil
}

type migrationInstaller struct {
	events   *[]string
	validate *bool
}

func (i migrationInstaller) InstallSharedGame(_ context.Context, _ string, _ string, _ domain.Instance, validate bool) error {
	*i.events = append(*i.events, "install")
	if i.validate != nil {
		*i.validate = validate
	}
	return nil
}

type migrationPublisher struct{ events *[]string }

func (p migrationPublisher) StagePath(_, id string) string { return "stage-" + id }
func (p migrationPublisher) Publish(_ context.Context, id string) error {
	*p.events = append(*p.events, "publish:"+id)
	return nil
}
func (p migrationPublisher) Activate(_ context.Context, id string) error {
	*p.events = append(*p.events, "activate:"+id)
	return nil
}

type migrationLayout struct{ events *[]string }

func (l migrationLayout) Prepare(_ context.Context, id, migrationID string) error {
	*l.events = append(*l.events, "prepare:"+id)
	return nil
}
func (l migrationLayout) Rollback(_ context.Context, id, migrationID string) error {
	*l.events = append(*l.events, "rollback:"+id)
	return nil
}

type migrationRebuilder struct {
	events *[]string
	fail   string
}

func (r migrationRebuilder) Switch(_ context.Context, v domain.Instance, _, release string) error {
	*r.events = append(*r.events, "switch:"+v.ID)
	if v.ID == r.fail {
		return errors.New("failed")
	}
	return nil
}

func TestSharedGameMigrationBuildsFreshReleaseAndMigratesAllStoppedInstances(t *testing.T) {
	events := []string{}
	validate := true
	repo := &migrationRepo{instances: []domain.Instance{{ID: "a", RuntimeImage: "runtime", ActualState: domain.StateStopped}, {ID: "b", RuntimeImage: "runtime", ActualState: domain.StateStopped}}}
	service := SharedGameService{Root: t.TempDir(), Instances: repo, Installer: migrationInstaller{events: &events, validate: &validate}, Publisher: migrationPublisher{&events}, Layout: migrationLayout{&events}, Reconciler: migrationRebuilder{events: &events}}
	if err := service.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if validate {
		t.Fatal("fresh shared game install must not validate")
	}
	if got := strings.Join(events, ","); !strings.HasPrefix(got, "install,publish:") || !strings.Contains(got, ",prepare:a,switch:a,prepare:b,switch:b,activate:") {
		t.Fatalf("events=%s", got)
	}
	if repo.state.MigrationState != "ready" || repo.state.ActiveReleaseID == "" || repo.state.OperationID != "" {
		t.Fatalf("state=%#v", repo.state)
	}
}

func TestSharedGameMigrationRejectsActiveInstance(t *testing.T) {
	repo := &migrationRepo{instances: []domain.Instance{{ID: "a", ActualState: domain.StateRunning}}}
	service := SharedGameService{Instances: repo}
	if err := service.Migrate(context.Background()); err == nil {
		t.Fatal("active instance accepted")
	}
}

func TestSharedGameMigrationRollsPreparedInstancesBack(t *testing.T) {
	events := []string{}
	repo := &migrationRepo{instances: []domain.Instance{{ID: "a", RuntimeImage: "runtime", ActualState: domain.StateStopped}, {ID: "b", RuntimeImage: "runtime", ActualState: domain.StateStopped}}}
	service := SharedGameService{Root: t.TempDir(), Instances: repo, Installer: migrationInstaller{events: &events}, Publisher: migrationPublisher{&events}, Layout: migrationLayout{&events}, Reconciler: migrationRebuilder{events: &events, fail: "b"}}
	if err := service.Migrate(context.Background()); err == nil {
		t.Fatal("migration failure accepted")
	}
	if got := strings.Join(events, ","); !strings.HasSuffix(got, "switch:b,rollback:b,rollback:a") {
		t.Fatalf("events=%s", got)
	}
	if repo.state.MigrationState != "failed" {
		t.Fatalf("state=%#v", repo.state)
	}
}

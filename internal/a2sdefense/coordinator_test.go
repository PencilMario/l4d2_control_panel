package a2sdefense

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

type fakeRepository struct {
	instances []domain.Instance
	settings  domain.A2SDefenseSettings
	saves     []domain.A2SDefenseSettings
	err       error
}

func (f *fakeRepository) Instances(context.Context) ([]domain.Instance, error) {
	return append([]domain.Instance(nil), f.instances...), f.err
}
func (f *fakeRepository) A2SDefenseSettings(context.Context) (domain.A2SDefenseSettings, error) {
	return f.settings, f.err
}
func (f *fakeRepository) SaveA2SDefenseSettings(_ context.Context, value domain.A2SDefenseSettings) error {
	if f.err != nil {
		return f.err
	}
	f.settings = value
	f.saves = append(f.saves, value)
	return nil
}

type recordingFirewall struct {
	configs []Config
	status  Status
	err     error
}

func (f *recordingFirewall) Apply(_ context.Context, config Config) (Status, error) {
	f.configs = append(f.configs, config)
	if f.err != nil {
		return Status{}, f.err
	}
	f.status = Status{Compatible: true, Enabled: config.Enabled, Revision: config.Revision, PolicyVersion: PolicyVersion, Ports: append([]int(nil), config.Ports...), AppliedAt: "2026-08-05T02:03:04Z"}
	return f.status, nil
}
func (f *recordingFirewall) Status(context.Context) (Status, error) { return f.status, f.err }

func TestCoordinatorEnableAppliesAllGameAndSourceTVPortsBeforePersisting(t *testing.T) {
	repository := &fakeRepository{instances: []domain.Instance{
		{GamePort: 27016, SourceTVPort: 27020, PluginPorts: []int{27030}},
		{GamePort: 27015, SourceTVPort: 27020},
	}}
	firewall := &recordingFirewall{}
	coordinator := NewCoordinator(repository, firewall, time.Minute, func() time.Time { return time.Date(2026, 8, 5, 2, 3, 4, 0, time.UTC) })
	status, err := coordinator.SetEnabled(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firewall.configs[0].Ports, []int{27015, 27016, 27020}) {
		t.Fatalf("ports=%v", firewall.configs[0].Ports)
	}
	if status.Revision != 1 || !repository.settings.Enabled || repository.settings.Pending || repository.settings.Revision != 1 || repository.settings.LastSyncedAt.IsZero() {
		t.Fatalf("status=%+v settings=%+v", status, repository.settings)
	}
}

func TestCoordinatorFailedEnableKeepsDesiredStateAndRecordsError(t *testing.T) {
	repository := &fakeRepository{}
	firewall := &recordingFirewall{err: errors.New("helper unavailable")}
	coordinator := NewCoordinator(repository, firewall, time.Minute, time.Now)
	if _, err := coordinator.SetEnabled(context.Background(), true); err == nil {
		t.Fatal("expected enable failure")
	}
	if repository.settings.Enabled || !repository.settings.Pending || repository.settings.LastError == "" {
		t.Fatalf("settings=%+v", repository.settings)
	}
}

func TestCoordinatorReconcileAdvancesRevisionWhenPortsChange(t *testing.T) {
	repository := &fakeRepository{
		instances: []domain.Instance{{GamePort: 27016}},
		settings:  domain.A2SDefenseSettings{Enabled: true, Revision: 4},
	}
	firewall := &recordingFirewall{status: Status{Compatible: true, Enabled: true, Revision: 4, Ports: []int{27015}}}
	coordinator := NewCoordinator(repository, firewall, time.Minute, time.Now)
	if err := coordinator.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(firewall.configs) != 1 || firewall.configs[0].Revision != 5 || !reflect.DeepEqual(firewall.configs[0].Ports, []int{27016}) {
		t.Fatalf("configs=%+v", firewall.configs)
	}
	if repository.settings.Revision != 5 || repository.settings.Pending {
		t.Fatalf("settings=%+v", repository.settings)
	}
}

func TestCoordinatorReconcileMarksPendingAndPeriodicLoopStops(t *testing.T) {
	repository := &fakeRepository{instances: []domain.Instance{{GamePort: 27015}}, settings: domain.A2SDefenseSettings{Enabled: true, Revision: 1}}
	firewall := &recordingFirewall{err: errors.New("offline")}
	coordinator := NewCoordinator(repository, firewall, 5*time.Millisecond, time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	coordinator.Start(ctx)
	time.Sleep(20 * time.Millisecond)
	cancel()
	coordinator.Stop()
	if !repository.settings.Pending || repository.settings.LastError == "" {
		t.Fatalf("settings=%+v", repository.settings)
	}
}

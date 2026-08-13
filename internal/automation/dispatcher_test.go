package automation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/a2s"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/crashreports"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/players"
	"github.com/not0721here/l4d2-control-panel/internal/releases"
	"github.com/not0721here/l4d2-control-panel/internal/updates"
)

type recordingMaintenanceCleaner struct {
	days int
	err  error
}

func (*recordingMaintenanceCleaner) Backup(context.Context, string) (string, error) {
	return "", nil
}

func (c *recordingMaintenanceCleaner) Cleanup(_ context.Context, retention time.Duration) (int, error) {
	c.days = int(retention / (24 * time.Hour))
	return 0, c.err
}

type recordingGameLogCleaner struct {
	days int
	err  error
}

func (c *recordingGameLogCleaner) Cleanup(_ context.Context, retentionDays int) error {
	c.days = retentionDays
	return c.err
}

type recordingCrashReportCleaner struct {
	days int
	err  error
}

func (c *recordingCrashReportCleaner) Cleanup(_ context.Context, retentionDays int) (crashreports.CleanupResult, error) {
	c.days = retentionDays
	return crashreports.CleanupResult{}, c.err
}

func TestScheduledCleanupSharesRetentionAcrossAllCleanersAndAttemptsEachStage(t *testing.T) {
	maintenance := &recordingMaintenanceCleaner{err: errors.New("maintenance failed")}
	gameLogs := &recordingGameLogCleaner{err: errors.New("game logs failed")}
	crashReports := &recordingCrashReportCleaner{err: errors.New("crash reports failed")}
	d := Dispatcher{
		Maintenance:  maintenance,
		GameLogs:     gameLogs,
		CrashReports: crashReports,
	}

	err := d.run(context.Background(), domain.ScheduledTask{Type: "cleanup", Payload: `{"retention_days":45}`})
	for _, want := range []error{maintenance.err, gameLogs.err, crashReports.err} {
		if !errors.Is(err, want) {
			t.Errorf("error=%v does not contain %v", err, want)
		}
	}
	if maintenance.days != 45 || gameLogs.days != 45 || crashReports.days != 45 {
		t.Fatalf("retentions=%d/%d/%d", maintenance.days, gameLogs.days, crashReports.days)
	}
}

func TestScheduledCleanupDefaultsRetentionToThirtyDays(t *testing.T) {
	maintenance := &recordingMaintenanceCleaner{}
	gameLogs := &recordingGameLogCleaner{}
	crashReports := &recordingCrashReportCleaner{}
	d := Dispatcher{Maintenance: maintenance, GameLogs: gameLogs, CrashReports: crashReports}

	if err := d.run(context.Background(), domain.ScheduledTask{Type: "cleanup", Payload: `{}`}); err != nil {
		t.Fatal(err)
	}
	if maintenance.days != 30 || gameLogs.days != 30 || crashReports.days != 30 {
		t.Fatalf("retentions=%d/%d/%d", maintenance.days, gameLogs.days, crashReports.days)
	}
}

type missingSourceRepo struct{}

func (missingSourceRepo) GitHubSource(context.Context, string) (domain.GitHubSource, error) {
	return domain.GitHubSource{}, errors.New("not found")
}

type fakeInstanceRepo struct {
	instance domain.Instance
	err      error
}

type failedPlayerQuery struct{}

func (failedPlayerQuery) Info(string) (a2s.Info, error) {
	return a2s.Info{}, errors.New("instance is offline")
}

func (failedPlayerQuery) Players(string) ([]a2s.Player, error) {
	return nil, errors.New("instance is offline")
}

func (f fakeInstanceRepo) Instance(context.Context, string) (domain.Instance, error) {
	return f.instance, f.err
}

type fakeReleaseFetcher struct {
	result       releases.FetchResult
	repository   string
	assetPattern string
}

func (f *fakeReleaseFetcher) FetchLatest(_ context.Context, repository, assetPattern, _ string, _ *content.PackageManager) (releases.FetchResult, error) {
	f.repository = repository
	f.assetPattern = assetPattern
	return f.result, nil
}

func TestScheduledReleaseReportsDeletedSource(t *testing.T) {
	d := Dispatcher{Sources: missingSourceRepo{}, Packages: &content.PackageManager{}, ReleaseFetcher: &fakeReleaseFetcher{}, PackagesUpdate: &fakePackageUpdater{}}
	err := d.run(context.Background(), domain.ScheduledTask{Type: "release_check", Payload: `{"source_id":"deleted"}`})
	if err == nil || err.Error() != "GitHub source not found" {
		t.Fatalf("err=%v", err)
	}
}

func TestScheduledHotPackageUpdatesAreRetired(t *testing.T) {
	err := (Dispatcher{}).run(context.Background(), domain.ScheduledTask{InstanceID: "instance", Type: "package_hot"})
	if err == nil || err.Error() != "unsupported scheduled task type" {
		t.Fatalf("err=%v", err)
	}
}

type fakePackageUpdater struct {
	calls     int
	mode      updates.Mode
	packageID string
}

type fakeSharedGameUpdater struct{ policy string }

func (f *fakeSharedGameUpdater) Update(_ context.Context, policy string) error {
	f.policy = policy
	return nil
}

func TestScheduledGameUpdateUsesGlobalPolicy(t *testing.T) {
	updater := &fakeSharedGameUpdater{}
	d := Dispatcher{SharedGameUpdate: updater}
	task := domain.ScheduledTask{Type: "game_update", OnlinePolicy: "wait"}
	if err := d.run(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if updater.policy != "wait" {
		t.Fatalf("policy=%q", updater.policy)
	}
}

func TestWaitForPlayersForcesWaitPolicyAfterQueryFailure(t *testing.T) {
	instance := domain.Instance{ID: "instance", ActualState: domain.StateRunning, ContainerID: "container", GamePort: 27015}
	playerService := players.NewService(fakeInstanceRepo{instance: instance}, failedPlayerQuery{}, nil, "127.0.0.1")
	d := Dispatcher{Instances: fakeInstanceRepo{instance: instance}, Players: playerService}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := d.waitForPlayers(ctx, domain.ScheduledTask{InstanceID: instance.ID, OnlinePolicy: "wait"}); err != nil {
		t.Fatalf("query failure should force a waiting scheduled task to run: %v", err)
	}
}

func TestWaitForPlayersAllowsStoppedInstance(t *testing.T) {
	instance := domain.Instance{ID: "instance", ActualState: domain.StateStopped}
	playerService := players.NewService(fakeInstanceRepo{instance: instance}, failedPlayerQuery{}, nil, "127.0.0.1")
	d := Dispatcher{Instances: fakeInstanceRepo{instance: instance}, Players: playerService}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := d.waitForPlayers(ctx, domain.ScheduledTask{InstanceID: instance.ID, OnlinePolicy: "wait"}); err != nil {
		t.Fatalf("stopped instance should not wait for player queries: %v", err)
	}
}

func (f *fakePackageUpdater) ApplyPackage(_ context.Context, _ string, item content.PackageVersion, mode updates.Mode) error {
	f.calls++
	f.packageID = item.ID
	f.mode = mode
	return nil
}

func TestScheduledHotReleaseUpdatesAreRetired(t *testing.T) {
	err := (Dispatcher{}).run(context.Background(), domain.ScheduledTask{InstanceID: "instance", Type: "release_hot"})
	if err == nil || err.Error() != "unsupported scheduled task type" {
		t.Fatalf("err=%v", err)
	}
}

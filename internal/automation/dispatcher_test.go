package automation

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/releases"
	"github.com/not0721here/l4d2-control-panel/internal/updates"
)

type missingSourceRepo struct{}

func (missingSourceRepo) GitHubSource(context.Context, string) (domain.GitHubSource, error) {
	return domain.GitHubSource{}, errors.New("not found")
}

type fakeInstanceRepo struct {
	instance domain.Instance
	err      error
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

func TestScheduledPackageUpdateUsesInstanceSelectedPackage(t *testing.T) {
	manager, err := content.NewPackageManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	selected := content.PackageVersion{ID: uuid.NewString(), Filename: "selected.zip", Version: "v-selected", HotCompatible: true}
	if err := manager.UpdateMetadata(selected); err != nil {
		t.Fatal(err)
	}
	updater := &fakePackageUpdater{}
	d := Dispatcher{
		Instances:      fakeInstanceRepo{instance: domain.Instance{ID: "instance", SelectedPackageID: selected.ID}},
		Packages:       manager,
		PackagesUpdate: updater,
	}
	if err := d.run(context.Background(), domain.ScheduledTask{InstanceID: "instance", Type: "package_hot", Payload: `{"package_id":"wrong"}`}); err != nil {
		t.Fatal(err)
	}
	if updater.packageID != selected.ID {
		t.Fatalf("package=%q want %q", updater.packageID, selected.ID)
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

func (f *fakePackageUpdater) ApplyPackage(_ context.Context, _ string, item content.PackageVersion, mode updates.Mode) error {
	f.calls++
	f.packageID = item.ID
	f.mode = mode
	return nil
}

func TestScheduledReleaseUpdateAppliesOnlyNewRelease(t *testing.T) {
	for _, tc := range []struct {
		name, kind string
		mode       updates.Mode
	}{
		{"hot", "release_hot", updates.Hot}, {"full", "release_full", updates.Full},
	} {
		t.Run(tc.name, func(t *testing.T) {
			updater := &fakePackageUpdater{}
			fetcher := &fakeReleaseFetcher{result: releases.FetchResult{Package: content.PackageVersion{ID: "new"}, Updated: true}}
			manager, err := content.NewPackageManager(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			selected := content.PackageVersion{ID: uuid.NewString(), Filename: "plugins.zip", SourceRepository: "owner/repo"}
			if err := manager.UpdateMetadata(selected); err != nil {
				t.Fatal(err)
			}
			d := Dispatcher{Instances: fakeInstanceRepo{instance: domain.Instance{ID: "instance", SelectedPackageID: selected.ID}}, Packages: manager, PackagesUpdate: updater, ReleaseFetcher: fetcher}
			task := domain.ScheduledTask{InstanceID: "instance", Type: tc.kind, Payload: `{"repository":"wrong/repo","asset_pattern":"wrong"}`}
			if err := d.run(context.Background(), task); err != nil {
				t.Fatal(err)
			}
			if updater.calls != 1 {
				t.Fatalf("calls=%d", updater.calls)
			}
			if fetcher.repository != "owner/repo" || fetcher.assetPattern != `^plugins\.zip$` {
				t.Fatalf("fetch source=%q pattern=%q", fetcher.repository, fetcher.assetPattern)
			}
			fetcher.result = releases.FetchResult{Updated: false}
			if err := d.run(context.Background(), task); err != nil {
				t.Fatal(err)
			}
			if updater.calls != 1 {
				t.Fatalf("unchanged release applied: calls=%d", updater.calls)
			}
		})
	}
}

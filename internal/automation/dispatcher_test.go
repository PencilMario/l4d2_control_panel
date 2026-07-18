package automation

import (
	"context"
	"errors"
	"testing"

	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/releases"
	"github.com/not0721here/l4d2-control-panel/internal/updates"
)

type missingSourceRepo struct{}

func (missingSourceRepo) GitHubSource(context.Context, string) (domain.GitHubSource, error) {
	return domain.GitHubSource{}, errors.New("not found")
}

type fakeReleaseFetcher struct{ result releases.FetchResult }

func (f fakeReleaseFetcher) FetchLatest(context.Context, string, string, string, *content.PackageManager) (releases.FetchResult, error) {
	return f.result, nil
}

func TestScheduledReleaseReportsDeletedSource(t *testing.T) {
	d := Dispatcher{Sources: missingSourceRepo{}, Packages: &content.PackageManager{}, PackagesUpdate: &fakePackageUpdater{}}
	err := d.run(context.Background(), domain.ScheduledTask{Type: "release_hot", Payload: `{"source_id":"deleted"}`})
	if err == nil || err.Error() != "GitHub source not found" {
		t.Fatalf("err=%v", err)
	}
}

type fakePackageUpdater struct {
	calls int
	mode  updates.Mode
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

func (f *fakePackageUpdater) ApplyPackage(context.Context, string, content.PackageVersion, updates.Mode) error {
	f.calls++
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
			d := Dispatcher{Packages: &content.PackageManager{}, PackagesUpdate: updater, ReleaseFetcher: fakeReleaseFetcher{result: releases.FetchResult{Package: content.PackageVersion{ID: "new"}, Updated: true}}}
			task := domain.ScheduledTask{InstanceID: "instance", Type: tc.kind, Payload: `{"repository":"owner/repo","asset_pattern":"^plugins\\.zip$"}`}
			if err := d.run(context.Background(), task); err != nil {
				t.Fatal(err)
			}
			if updater.calls != 1 {
				t.Fatalf("calls=%d", updater.calls)
			}
			d.ReleaseFetcher = fakeReleaseFetcher{result: releases.FetchResult{Updated: false}}
			if err := d.run(context.Background(), task); err != nil {
				t.Fatal(err)
			}
			if updater.calls != 1 {
				t.Fatalf("unchanged release applied: calls=%d", updater.calls)
			}
		})
	}
}

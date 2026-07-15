package automation

import (
	"context"
	"testing"

	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/releases"
	"github.com/not0721here/l4d2-control-panel/internal/updates"
)

type fakeReleaseFetcher struct{ result releases.FetchResult }

func (f fakeReleaseFetcher) FetchLatest(context.Context, string, string, string, *content.PackageManager) (releases.FetchResult, error) {
	return f.result, nil
}

type fakePackageUpdater struct {
	calls int
	mode  updates.Mode
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

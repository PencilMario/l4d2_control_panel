package updates

import (
	"context"
	"strings"
	"testing"

	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

type rebuildOverlay struct{ events *[]string }

func (o rebuildOverlay) ResetUpper(_ context.Context, instanceID, releaseID string) error {
	*o.events = append(*o.events, "reset:"+instanceID+":"+releaseID)
	return nil
}

type rebuildPackages struct{}

func (rebuildPackages) Get(string) (content.PackageVersion, error) {
	return content.PackageVersion{ID: "pkg", ArchivePath: "pkg.zip", Version: "v1"}, nil
}

type rebuildDeployer struct{ events *[]string }

func (d rebuildDeployer) Apply(_ context.Context, id, _, _ string, mode Mode) error {
	*d.events = append(*d.events, "package:"+id+":"+string(mode))
	return nil
}

type rebuildPrivate struct{ events *[]string }

func (p rebuildPrivate) Apply(_ context.Context, id string) error {
	*p.events = append(*p.events, "private:"+id)
	return nil
}

type rebuildAccelerator struct{ events *[]string }

func (a rebuildAccelerator) Ensure(_ context.Context, instance domain.Instance) error {
	*a.events = append(*a.events, "accelerator:"+instance.ID)
	return nil
}

func (a rebuildAccelerator) Reinstall(_ context.Context, instance domain.Instance) error {
	*a.events = append(*a.events, "accelerator-reinstall:"+instance.ID)
	return nil
}

func TestSharedGameRebuilderRecreatesManagedLayers(t *testing.T) {
	events := []string{}
	r := SharedGameRebuilder{Overlay: rebuildOverlay{&events}, Packages: rebuildPackages{}, Deployer: rebuildDeployer{&events}, Private: rebuildPrivate{&events}, Accelerator: rebuildAccelerator{&events}}
	if err := r.Switch(context.Background(), domain.Instance{ID: "abc", SelectedPackageID: "pkg"}, "old", "new"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(events, ","); got != "reset:abc:new,package:abc:full,private:abc,accelerator-reinstall:abc" {
		t.Fatalf("events=%s", got)
	}
}

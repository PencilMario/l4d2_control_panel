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

func TestSharedGameRebuilderRecreatesManagedLayers(t *testing.T) {
	events := []string{}
	r := SharedGameRebuilder{Overlay: rebuildOverlay{&events}, Packages: rebuildPackages{}, Deployer: rebuildDeployer{&events}, Private: rebuildPrivate{&events}}
	if err := r.Switch(context.Background(), domain.Instance{ID: "abc", SelectedPackageID: "pkg"}, "old", "new"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(events, ","); got != "reset:abc:new,package:abc:full,private:abc" {
		t.Fatalf("events=%s", got)
	}
}

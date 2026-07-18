//go:build linux

package overlayfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSystemMounterIntegration(t *testing.T) {
	if os.Getenv("OVERLAYFS_INTEGRATION") != "1" {
		t.Skip("set OVERLAYFS_INTEGRATION=1 as root")
	}
	root := t.TempDir()
	mount, err := (Paths{Root: root}).Mount("instance-1", "release-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(mount.Lower, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mount.Lower, "baseline.txt"), []byte("baseline"), 0o640); err != nil {
		t.Fatal(err)
	}
	mounter := SystemMounter{}
	ctx := context.Background()
	if err := mounter.Ensure(ctx, mount); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mounter.Unmount(ctx, mount) })
	if raw, err := os.ReadFile(filepath.Join(mount.Merged, "baseline.txt")); err != nil || string(raw) != "baseline" {
		t.Fatalf("merged baseline = %q, err=%v", raw, err)
	}
	if err := os.WriteFile(filepath.Join(mount.Merged, "runtime.txt"), []byte("instance"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(mount.Upper, "runtime.txt")); err != nil {
		t.Fatalf("runtime write missing from upper: %v", err)
	}
	if err := mounter.ResetManagedPaths(ctx, mount, []string{"runtime.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(mount.Merged, "runtime.txt")); !os.IsNotExist(err) {
		t.Fatalf("managed runtime path still visible: %v", err)
	}
}

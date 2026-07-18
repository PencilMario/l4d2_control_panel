package metrics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryStorageCountsOverlayUpperInsteadOfMerged(t *testing.T) {
	root := t.TempDir()
	instanceRoot := filepath.Join(root, "instances", "instance-1")
	upper := filepath.Join(instanceRoot, "overlay", "upper")
	merged := filepath.Join(instanceRoot, "overlay", "merged")
	if err := os.MkdirAll(upper, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(merged, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(upper, "private.cfg"), []byte("upper"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(merged, "shared.vpk"), []byte("shared lower"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("overlay", "merged"), filepath.Join(instanceRoot, "game")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}

	usage, err := (DirectoryStorage{Root: root}).InstanceStorage(context.Background(), "instance-1")
	if err != nil {
		t.Fatal(err)
	}
	if usage.Game != uint64(len("upper")) {
		t.Fatalf("game usage = %d, want %d", usage.Game, len("upper"))
	}
}

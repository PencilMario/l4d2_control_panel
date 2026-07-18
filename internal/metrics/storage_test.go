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

func TestDirectoryStorageCountsPersistentInstanceDirectories(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "instances", "instance-1")
	files := map[string]string{
		"private/server.cfg":        "private",
		"backups/nightly.tar.gz":    "backup",
		"console/session.log":       "console",
		"logs/game/server.log":      "game-log",
		"logs/sourcemod/errors.log": "sourcemod-log",
	}
	for name, contents := range files {
		path := filepath.Join(base, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	usage, err := (DirectoryStorage{Root: root}).InstanceStorage(context.Background(), "instance-1")
	if err != nil {
		t.Fatal(err)
	}
	if usage.Private != uint64(len("private")) {
		t.Fatalf("private usage = %d, want %d", usage.Private, len("private"))
	}
	if usage.Backups != uint64(len("backup")) {
		t.Fatalf("backups usage = %d, want %d", usage.Backups, len("backup"))
	}
	wantLogs := uint64(len("console") + len("game-log") + len("sourcemod-log"))
	if usage.Console != wantLogs {
		t.Fatalf("log usage = %d, want %d", usage.Console, wantLogs)
	}
}

package gamelogs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPrepareCreatesPersistentLogRoots(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(root, Options{})
	if err := manager.Prepare(context.Background(), "instance-1"); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"logs/game", "logs/sourcemod"} {
		info, err := os.Stat(filepath.Join(root, "instances", "instance-1", filepath.FromSlash(rel)))
		if err != nil || !info.IsDir() {
			t.Fatalf("%s was not prepared as a directory: info=%v err=%v", rel, info, err)
		}
	}
}

func TestPrepareMigratesMergedGameAndNestedSourceModLogs(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "instances", "instance-1")
	writeFile(t, filepath.Join(base, "overlay", "merged", "left4dead2", "logs", "game.log"), "game")
	writeFile(t, filepath.Join(base, "overlay", "merged", "left4dead2", "addons", "sourcemod", "logs", "errors", "error.log"), "sm")

	if err := NewManager(root, Options{}).Prepare(context.Background(), "instance-1"); err != nil {
		t.Fatal(err)
	}
	assertFile(t, filepath.Join(base, "logs", "game", "game.log"), "game")
	assertFile(t, filepath.Join(base, "logs", "sourcemod", "errors", "error.log"), "sm")
	assertFile(t, filepath.Join(base, "overlay", "merged", "left4dead2", "logs", "game.log"), "game")
}

func TestPrepareMigratesOverlayUpperLogsWhenMergedIsUnavailable(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "instances", "instance-1")
	writeFile(t, filepath.Join(base, "overlay", "upper", "left4dead2", "logs", "upper.log"), "upper-game")
	writeFile(t, filepath.Join(base, "overlay", "upper", "left4dead2", "addons", "sourcemod", "logs", "nested", "upper-sm.log"), "upper-sm")

	if err := NewManager(root, Options{}).Prepare(context.Background(), "instance-1"); err != nil {
		t.Fatal(err)
	}
	assertFile(t, filepath.Join(base, "logs", "game", "upper.log"), "upper-game")
	assertFile(t, filepath.Join(base, "logs", "sourcemod", "nested", "upper-sm.log"), "upper-sm")
}

func TestPrepareIsIdempotentAndPreservesConflictingContent(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "instances", "instance-1")
	source := filepath.Join(base, "overlay", "merged", "left4dead2", "logs", "server.log")
	destination := filepath.Join(base, "logs", "game", "server.log")
	writeFile(t, source, "old")
	writeFile(t, destination, "new")
	now := time.Date(2026, 7, 18, 12, 34, 56, 0, time.UTC)
	manager := NewManager(root, Options{Now: func() time.Time { return now }})

	if err := manager.Prepare(context.Background(), "instance-1"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Prepare(context.Background(), "instance-1"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(destination))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("files=%v, want original plus exactly one migrated conflict", entries)
	}
	assertFile(t, destination, "new")
	assertFile(t, filepath.Join(filepath.Dir(destination), "server.migrated-20260718T123456Z-001.log"), "old")
}

func TestPrepareSkipsIdenticalDestination(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "instances", "instance-1")
	writeFile(t, filepath.Join(base, "overlay", "merged", "left4dead2", "logs", "same.log"), "same")
	destination := filepath.Join(base, "logs", "game", "same.log")
	writeFile(t, destination, "same")
	if err := NewManager(root, Options{}).Prepare(context.Background(), "instance-1"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(destination))
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries=%v err=%v", entries, err)
	}
}

func TestPrepareRejectsSymlinkInMigrationTree(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "instances", "instance-1")
	logs := filepath.Join(base, "overlay", "merged", "left4dead2", "logs")
	if err := os.MkdirAll(logs, 0o750); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside.log")
	writeFile(t, outside, "secret")
	if err := os.Symlink(outside, filepath.Join(logs, "linked.log")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := NewManager(root, Options{}).Prepare(context.Background(), "instance-1"); err == nil {
		t.Fatal("expected symlink rejection")
	}
	if _, err := os.Stat(filepath.Join(base, "logs", "game", "linked.log")); !os.IsNotExist(err) {
		t.Fatalf("symlink target was migrated: %v", err)
	}
}

func TestPrepareRejectsSymlinkPersistentRoot(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "instances", "instance-1")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(filepath.Join(base, "logs"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "logs", "game")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := NewManager(root, Options{}).Prepare(context.Background(), "instance-1"); err == nil {
		t.Fatal("expected persistent root symlink rejection")
	}
}

func TestPrepareRejectsSymlinkPersistentSubdirectory(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "instances", "instance-1")
	writeFile(t, filepath.Join(base, "overlay", "merged", "left4dead2", "logs", "nested", "server.log"), "log")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(filepath.Join(base, "logs", "game"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "logs", "game", "nested")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := NewManager(root, Options{}).Prepare(context.Background(), "instance-1"); err == nil {
		t.Fatal("expected persistent subdirectory symlink rejection")
	}
	if _, err := os.Stat(filepath.Join(outside, "server.log")); !os.IsNotExist(err) {
		t.Fatalf("migration escaped persistent root: %v", err)
	}
}

func TestPrepareRejectsNonDirectoryPersistentComponent(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "instances", "instance-1")
	writeFile(t, filepath.Join(base, "overlay", "merged", "left4dead2", "logs", "nested", "server.log"), "log")
	writeFile(t, filepath.Join(base, "logs", "game", "nested"), "not a directory")
	if err := NewManager(root, Options{}).Prepare(context.Background(), "instance-1"); err == nil {
		t.Fatal("expected non-directory persistent component rejection")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("ReadFile(%s)=%q, %v; want %q", path, got, err, want)
	}
}

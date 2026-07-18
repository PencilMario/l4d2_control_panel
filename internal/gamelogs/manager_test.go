package gamelogs

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestTreeListsNestedLogsWithStableKindPathOrder(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "instances", "instance-1", "logs")
	writeFile(t, filepath.Join(base, "sourcemod", "z", "error.log"), "err")
	writeFile(t, filepath.Join(base, "game", "z.log"), "game-z")
	writeFile(t, filepath.Join(base, "game", "a.log"), "a")
	writeFile(t, filepath.Join(base, "sourcemod", "a.log"), "sm-a")
	wantTime := time.Date(2026, 7, 18, 2, 3, 4, 0, time.FixedZone("test", 8*60*60))
	if err := os.Chtimes(filepath.Join(base, "game", "a.log"), wantTime, wantTime); err != nil {
		t.Fatal(err)
	}

	entries, err := NewManager(root, Options{}).Tree(context.Background(), "instance-1")
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(entries))
	for i, entry := range entries {
		got[i] = entry.Kind + ":" + entry.Path
	}
	if want := []string{"game:a.log", "game:z.log", "sourcemod:a.log", "sourcemod:z/error.log"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("entries=%v, want %v", got, want)
	}
	if entries[0].Size != 1 || !entries[0].ModifiedAt.Equal(wantTime) || entries[0].ModifiedAt.Location() != time.UTC {
		t.Fatalf("metadata=%+v", entries[0])
	}
}

func TestPreviewReturnsTailMetadataAndReplacementText(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(root, Options{})
	base := filepath.Join(root, "instances", "i", "logs")
	path := filepath.Join(base, "sourcemod", "nested", "x.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte{'a', 'b', 0xff, 'c', 'd'}, 0o640); err != nil {
		t.Fatal(err)
	}
	mtime := time.Date(2026, 7, 18, 1, 2, 3, 0, time.UTC)
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	preview, err := manager.Preview(context.Background(), "i", "sourcemod", "nested/x.log", 4)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Text != "b\ufffdcd" || !preview.Truncated || preview.Size != 5 || !preview.ModifiedAt.Equal(mtime) {
		t.Fatalf("preview=%+v", preview)
	}
	full, err := manager.Preview(context.Background(), "i", "sourcemod", "nested/x.log", 10)
	if err != nil || full.Text != "ab\ufffdcd" || full.Truncated || full.Size != 5 {
		t.Fatalf("full=%+v err=%v", full, err)
	}
	original, _ := os.ReadFile(path)
	if !reflect.DeepEqual(original, []byte{'a', 'b', 0xff, 'c', 'd'}) {
		t.Fatalf("original changed: %v", original)
	}
}

func TestReadAPIsRejectUnsafeAndMissingPaths(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(root, Options{})
	base := filepath.Join(root, "instances", "i", "logs", "game")
	writeFile(t, filepath.Join(base, "ok.log"), "ok")
	for _, tc := range []struct {
		name, kind, path string
		limit            int64
	}{
		{"kind", "other", "ok.log", 1}, {"traversal", "game", "../ok.log", 1}, {"absolute", "game", filepath.Join(root, "outside.log"), 1}, {"nul", "game", "bad\x00.log", 1}, {"zero limit", "game", "ok.log", 0}, {"missing", "game", "missing.log", 1}, {"directory", "game", ".", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := manager.Preview(context.Background(), "i", tc.kind, tc.path, tc.limit); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestReadAPIsRejectIntermediateAndLeafSymlinks(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(root, Options{})
	base := filepath.Join(root, "instances", "i", "logs", "game")
	outside := filepath.Join(root, "outside")
	writeFile(t, filepath.Join(outside, "secret.log"), "secret")
	if err := os.MkdirAll(base, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.log"), filepath.Join(base, "leaf.log")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"linked/secret.log", "leaf.log"} {
		if _, err := manager.Preview(context.Background(), "i", "game", path, 10); err == nil {
			t.Fatalf("Preview(%q) accepted symlink", path)
		}
	}
	if _, err := manager.Tree(context.Background(), "i"); err == nil {
		t.Fatal("Tree accepted symlink")
	}
}

func TestResolveDownloadReturnsValidatedOpenRegularFile(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(root, Options{})
	path := filepath.Join(root, "instances", "i", "logs", "game", "server.log")
	writeFile(t, path, "download")
	file, info, err := manager.ResolveDownload("i", "game", "server.log")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if !info.Mode().IsRegular() || info.Size() != 8 {
		t.Fatalf("info=%v", info)
	}
	_ = os.Remove(path) // succeeds on Unix; Windows keeps the validated handle usable while open
	content := make([]byte, 8)
	if _, err := file.Read(content); err != nil {
		t.Fatal(err)
	}
	if string(content) != "download" {
		t.Fatalf("content=%q", content)
	}
}

func TestPreviewOpenFileSafelyHandlesShorteningAfterStat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rotating.log")
	writeFile(t, path, "0123456789")
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, 3); err != nil {
		t.Fatal(err)
	}
	current, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}

	preview, err := previewOpenFile(file, info, 4)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Text != "012" || preview.Truncated || preview.Size != 3 || !preview.ModifiedAt.Equal(current.ModTime().UTC()) {
		t.Fatalf("preview after shortening=%+v", preview)
	}
}

func TestPreviewOpenFileUsesActualTailAfterShorteningAboveLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rotating.log")
	writeFile(t, path, "0123456789")
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	initial, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, 7); err != nil {
		t.Fatal(err)
	}

	preview, err := previewOpenFile(file, initial, 4)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Text != "3456" || !preview.Truncated || preview.Size != 7 {
		t.Fatalf("preview after shortening=%+v", preview)
	}
}

func TestResolveDownloadRejectsDirectoryLeaf(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "instances", "i", "logs", "game", "directory")
	if err := os.MkdirAll(path, 0o750); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(root, Options{})
	if _, _, err := manager.ResolveDownload("i", "game", "directory"); err == nil {
		t.Fatal("ResolveDownload accepted directory leaf")
	}
	if _, err := manager.Preview(context.Background(), "i", "game", "directory", 10); err == nil {
		t.Fatal("Preview accepted directory leaf")
	}
}

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

func TestPrepareRejectsSymlinkInLegacyParentPath(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "instances", "instance-1")
	outside := filepath.Join(root, "outside")
	writeFile(t, filepath.Join(outside, "left4dead2", "logs", "escaped.log"), "secret")
	if err := os.MkdirAll(filepath.Join(base, "overlay"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "overlay", "merged")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := NewManager(root, Options{}).Prepare(context.Background(), "instance-1"); err == nil {
		t.Fatal("expected legacy parent symlink rejection")
	}
	if _, err := os.Stat(filepath.Join(base, "logs", "game", "escaped.log")); !os.IsNotExist(err) {
		t.Fatalf("legacy parent symlink escaped: %v", err)
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

func TestPrepareRejectsPersistentLeafSymlinkRegardlessOfContent(t *testing.T) {
	for _, test := range []struct {
		name, source, outside string
	}{
		{name: "same content", source: "same", outside: "same"},
		{name: "different content", source: "legacy", outside: "outside"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			base := filepath.Join(root, "instances", "instance-1")
			source := filepath.Join(base, "overlay", "merged", "left4dead2", "logs", "server.log")
			destination := filepath.Join(base, "logs", "game", "server.log")
			outside := filepath.Join(root, "outside.log")
			writeFile(t, source, test.source)
			writeFile(t, outside, test.outside)
			if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, destination); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}

			if err := NewManager(root, Options{}).Prepare(context.Background(), "instance-1"); err == nil {
				t.Fatal("expected persistent leaf symlink rejection")
			}
			assertFile(t, outside, test.outside)
			matches, err := filepath.Glob(filepath.Join(filepath.Dir(destination), "server.migrated-*.log"))
			if err != nil || len(matches) != 0 {
				t.Fatalf("conflict copies=%v err=%v", matches, err)
			}
		})
	}
}

func TestPrepareRejectsDirectoryAtPersistentFileLeaf(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "instances", "instance-1")
	source := filepath.Join(base, "overlay", "merged", "left4dead2", "logs", "server.log")
	destination := filepath.Join(base, "logs", "game", "server.log")
	writeFile(t, source, "legacy")
	if err := os.MkdirAll(destination, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := NewManager(root, Options{}).Prepare(context.Background(), "instance-1"); err == nil {
		t.Fatal("expected directory leaf rejection")
	}
	info, err := os.Stat(destination)
	if err != nil || !info.IsDir() {
		t.Fatalf("destination directory changed: info=%v err=%v", info, err)
	}
}

func TestPrepareSerializesConcurrentMigrationWithoutLosingConflicts(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "instances", "instance-1")
	writeFile(t, filepath.Join(base, "overlay", "merged", "left4dead2", "logs", "server.log"), "merged")
	writeFile(t, filepath.Join(base, "overlay", "upper", "left4dead2", "logs", "server.log"), "upper")
	manager := NewManager(root, Options{Now: func() time.Time {
		return time.Date(2026, 7, 18, 12, 34, 56, 0, time.UTC)
	}})
	start := make(chan struct{})
	errs := make(chan error, 8)
	var workers sync.WaitGroup
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			errs <- manager.Prepare(context.Background(), "instance-1")
		}()
	}
	close(start)
	workers.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Prepare: %v", err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(base, "logs", "game"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("entries=%v err=%v; want both contents exactly once", entries, err)
	}
	contents := map[string]bool{}
	for _, entry := range entries {
		value, err := os.ReadFile(filepath.Join(base, "logs", "game", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		contents[string(value)] = true
	}
	if !contents["merged"] || !contents["upper"] {
		t.Fatalf("contents=%v", contents)
	}
}

func TestPrepareDeduplicatesLargeFile(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "instances", "instance-1")
	content := make([]byte, 12*1024*1024)
	for index := range content {
		content[index] = byte(index % 251)
	}
	source := filepath.Join(base, "overlay", "merged", "left4dead2", "logs", "large.log")
	destination := filepath.Join(base, "logs", "game", "large.log")
	if err := os.MkdirAll(filepath.Dir(source), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, content, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, content, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := NewManager(root, Options{}).Prepare(context.Background(), "instance-1"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(destination))
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries=%v err=%v", entries, err)
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

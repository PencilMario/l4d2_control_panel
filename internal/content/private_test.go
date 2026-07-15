package content

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPrivateWorkspaceCRUDAndDiff(t *testing.T) {
	root := t.TempDir()
	manager := NewPrivateManager(root, 1<<20)
	ctx := context.Background()

	if err := manager.MakeDir(ctx, "abc", "cfg/sourcemod"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Save(ctx, "abc", "cfg/server.cfg", []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := manager.Move(ctx, "abc", "cfg/server.cfg", "cfg/sourcemod/server.cfg", false); err != nil {
		t.Fatal(err)
	}

	tree, err := manager.Tree(ctx, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 3 {
		t.Fatalf("tree=%#v", tree)
	}
	wantPaths := []string{"cfg", "cfg/sourcemod", "cfg/sourcemod/server.cfg"}
	wantKinds := []string{"directory", "directory", "file"}
	for i := range tree {
		if tree[i].Path != wantPaths[i] || tree[i].Kind != wantKinds[i] {
			t.Fatalf("tree[%d]=%#v", i, tree[i])
		}
	}
	if tree[2].Size != 5 {
		t.Fatalf("file size=%d", tree[2].Size)
	}

	diff, err := manager.Diff(ctx, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if diff.Summary != (DiffSummary{Added: 1}) {
		t.Fatalf("summary=%#v", diff.Summary)
	}
	if len(diff.Changes) != 1 || diff.Changes[0].Path != "cfg/sourcemod/server.cfg" || diff.Changes[0].Kind != "added" {
		t.Fatalf("changes=%#v", diff.Changes)
	}
}

func TestPrivateApplyDeleteRestoresCapturedLowerLayer(t *testing.T) {
	root := t.TempDir()
	manager := NewPrivateManager(root, 1<<20)
	game := filepath.Join(root, "instances", "abc", "game", "left4dead2", "cfg", "server.cfg")
	if err := os.MkdirAll(filepath.Dir(game), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(game, []byte("package"), 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Save(context.Background(), "abc", "cfg/server.cfg", []byte("private")); err != nil {
		t.Fatal(err)
	}
	if err := manager.ApplyChanges(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Delete(context.Background(), "abc", "cfg/server.cfg"); err != nil {
		t.Fatal(err)
	}
	if err := manager.ApplyChanges(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(game); err != nil || string(raw) != "package" {
		t.Fatalf("game=%q err=%v", raw, err)
	}
}

func TestPrivateApplyDeleteWithoutLowerRemovesTarget(t *testing.T) {
	root := t.TempDir()
	manager := NewPrivateManager(root, 1<<20)
	if _, err := manager.Save(context.Background(), "abc", "cfg/private.cfg", []byte("private")); err != nil {
		t.Fatal(err)
	}
	if err := manager.ApplyChanges(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Delete(context.Background(), "abc", "cfg/private.cfg"); err != nil {
		t.Fatal(err)
	}
	if err := manager.ApplyChanges(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "instances", "abc", "game", "left4dead2", "cfg", "private.cfg")
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target remains: %v", err)
	}
}

func TestPrivateApplyCopiesEmptyDirectory(t *testing.T) {
	root := t.TempDir()
	manager := NewPrivateManager(root, 1<<20)
	if err := manager.MakeDir(context.Background(), "abc", "cfg/empty"); err != nil {
		t.Fatal(err)
	}
	if err := manager.ApplyChanges(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "instances", "abc", "game", "left4dead2", "cfg", "empty")
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		t.Fatalf("empty directory missing: info=%v err=%v", info, err)
	}
}

func TestPrivateApplyFailureRollsBackGameAndManifest(t *testing.T) {
	root := t.TempDir()
	manager := NewPrivateManager(root, 1<<20)
	ctx := context.Background()
	if _, err := manager.Save(ctx, "abc", "cfg/a.cfg", []byte("old-a")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Save(ctx, "abc", "cfg/b.cfg", []byte("old-b")); err != nil {
		t.Fatal(err)
	}
	if err := manager.ApplyChanges(ctx, "abc"); err != nil {
		t.Fatal(err)
	}
	manifestPath, _ := manager.manifestPath("abc")
	oldManifest, _ := os.ReadFile(manifestPath)
	if _, err := manager.Save(ctx, "abc", "cfg/a.cfg", []byte("new-a")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Save(ctx, "abc", "cfg/b.cfg", []byte("new-b")); err != nil {
		t.Fatal(err)
	}
	setPrivateApplyFailureHook(func(count int) error {
		if count == 1 {
			return errors.New("injected")
		}
		return nil
	})
	t.Cleanup(func() { setPrivateApplyFailureHook(nil) })
	if err := manager.ApplyChanges(ctx, "abc"); err == nil {
		t.Fatal("expected failure")
	}
	game := filepath.Join(root, "instances", "abc", "game", "left4dead2", "cfg")
	for name, want := range map[string]string{"a.cfg": "old-a", "b.cfg": "old-b"} {
		raw, err := os.ReadFile(filepath.Join(game, name))
		if err != nil || string(raw) != want {
			t.Fatalf("%s=%q err=%v", name, raw, err)
		}
	}
	if got, _ := os.ReadFile(manifestPath); string(got) != string(oldManifest) {
		t.Fatal("applied manifest changed")
	}
	if diff, err := manager.Diff(ctx, "abc"); err != nil || diff.Summary.Modified != 2 {
		t.Fatalf("diff=%#v err=%v", diff, err)
	}
}

func TestPrivateSnapshotsRetainAndRestoreWorkspace(t *testing.T) {
	root := t.TempDir()
	manager := NewPrivateManager(root, 1<<20)
	ctx := context.Background()
	if err := manager.MakeDir(ctx, "abc", "cfg/empty"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 21; i++ {
		if _, err := manager.Save(ctx, "abc", "cfg/value.cfg", []byte(strconv.Itoa(i))); err != nil {
			t.Fatal(err)
		}
		if err := manager.ApplyChanges(ctx, "abc"); err != nil {
			t.Fatal(err)
		}
	}
	snapshots, err := manager.Snapshots(ctx, "abc")
	if err != nil || len(snapshots) != 20 {
		t.Fatalf("snapshots=%d err=%v", len(snapshots), err)
	}
	selected := snapshots[len(snapshots)-1]
	game := filepath.Join(root, "instances", "abc", "game", "left4dead2", "cfg", "value.cfg")
	beforeGame, _ := os.ReadFile(game)
	manifestPath, _ := manager.manifestPath("abc")
	beforeManifest, _ := os.ReadFile(manifestPath)
	if err := manager.RestoreSnapshot(ctx, "abc", selected.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "instances", "abc", "private", "cfg", "empty")); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(game); string(raw) != string(beforeGame) {
		t.Fatal("restore changed game")
	}
	if raw, _ := os.ReadFile(manifestPath); string(raw) != string(beforeManifest) {
		t.Fatal("restore changed manifest")
	}
	if diff, err := manager.Diff(ctx, "abc"); err != nil || diff.Summary.Modified != 1 {
		t.Fatalf("diff=%#v err=%v", diff, err)
	}
	if err := manager.ApplyChanges(ctx, "abc"); err != nil {
		t.Fatal(err)
	}
	after, _ := manager.Snapshots(ctx, "abc")
	if after[0].ID == selected.ID {
		t.Fatal("apply rewrote snapshot history")
	}
}

func TestPrivatePathRejectsEscapeSymlinkAndOverwrite(t *testing.T) {
	root := t.TempDir()
	manager := NewPrivateManager(root, 1<<20)
	ctx := context.Background()

	if _, err := manager.Save(ctx, "abc", "../outside", []byte("x")); err == nil {
		t.Fatal("escape accepted")
	}
	if err := manager.MakeDir(ctx, "abc", "cfg"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Save(ctx, "abc", "cfg/a.cfg", []byte("a")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Save(ctx, "abc", "cfg/b.cfg", []byte("b")); err != nil {
		t.Fatal(err)
	}
	if err := manager.Move(ctx, "abc", "cfg/a.cfg", "cfg/b.cfg", false); err == nil || !strings.Contains(err.Error(), "exists") {
		t.Fatalf("move conflict err=%v", err)
	}

	privateRoot := filepath.Join(root, "instances", "abc", "private")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(outside, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(privateRoot, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := manager.Save(ctx, "abc", "linked/escape.cfg", []byte("x")); err == nil {
		t.Fatal("symlink parent accepted")
	}
	if _, err := manager.Tree(ctx, "abc"); err == nil {
		t.Fatal("tree accepted symlink")
	}
}

func TestPrivateMoveOverwriteAndFailurePreservesDestination(t *testing.T) {
	manager := NewPrivateManager(t.TempDir(), 1<<20)
	ctx := context.Background()
	if _, err := manager.Save(ctx, "abc", "cfg/source.cfg", []byte("new")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Save(ctx, "abc", "cfg/target.cfg", []byte("old")); err != nil {
		t.Fatal(err)
	}
	if err := manager.Move(ctx, "abc", "cfg/source.cfg", "cfg/target.cfg", true); err != nil {
		t.Fatal(err)
	}
	raw, err := manager.Read(ctx, "abc", "cfg/target.cfg")
	if err != nil || string(raw) != "new" {
		t.Fatalf("target=%q err=%v", raw, err)
	}

	if err := manager.MakeDir(ctx, "abc", "tree/destination"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Save(ctx, "abc", "tree/destination/keep.cfg", []byte("keep")); err != nil {
		t.Fatal(err)
	}
	if err := manager.Move(ctx, "abc", "tree", "tree/destination", true); err == nil {
		t.Fatal("moving a directory into itself succeeded")
	}
	raw, err = manager.Read(ctx, "abc", "tree/destination/keep.cfg")
	if err != nil || string(raw) != "keep" {
		t.Fatalf("failed move did not preserve destination: %q err=%v", raw, err)
	}
	matches, err := filepath.Glob(filepath.Join(manager.root, "instances", "abc", "private", "**", ".private-replaced-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("replacement artifacts=%v err=%v", matches, err)
	}
}

func TestPrivateWorkspacePreservesPrivatePrefixedDotfiles(t *testing.T) {
	manager := NewPrivateManager(t.TempDir(), 1<<20)
	ctx := context.Background()
	if err := manager.MakeDir(ctx, "abc", ".private-directory"); err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string]string{
		".private-settings":    "root",
		"cfg/.private-example": "nested",
	} {
		if _, err := manager.Save(ctx, "abc", path, []byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	tree, err := manager.Tree(ctx, "abc")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, entry := range tree {
		seen[entry.Path] = true
	}
	for _, path := range []string{".private-directory", ".private-settings", "cfg/.private-example"} {
		if !seen[path] {
			t.Fatalf("tree hid legitimate path %q: %#v", path, tree)
		}
	}
	diff, err := manager.Diff(ctx, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if diff.Summary.Added != 3 {
		t.Fatalf("diff hid legitimate resources: %#v", diff)
	}
	listed, err := manager.List(ctx, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Fatalf("list=%#v", listed)
	}
	temporaryEntries, err := filepath.Glob(filepath.Join(manager.root, "instances", "abc", "private", "**", ".private-save-*"))
	if err != nil || len(temporaryEntries) != 0 {
		t.Fatalf("save temporary artifacts=%v err=%v", temporaryEntries, err)
	}
}

func TestPrivateManagersShareInstanceLock(t *testing.T) {
	root := t.TempDir()
	a := NewPrivateManager(root, 1<<20)
	b := NewPrivateManager(filepath.Join(root, "."), 1<<20)
	if a.instanceLock("abc") != b.instanceLock("abc") {
		t.Fatal("managers for the same canonical root do not share a lock")
	}
}

func TestPrivateSaveAndMoveHideIntermediateStateAcrossManagers(t *testing.T) {
	root := t.TempDir()
	a := NewPrivateManager(root, 1<<20)
	b := NewPrivateManager(root, 1<<20)
	ctx := context.Background()
	if _, err := a.Save(ctx, "abc", "cfg/target.cfg", []byte("old")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Save(ctx, "abc", "cfg/source.cfg", []byte("new")); err != nil {
		t.Fatal(err)
	}

	for _, phase := range []string{"save-temp-created", "move-destination-swapped"} {
		t.Run(phase, func(t *testing.T) {
			reached := make(chan string, 1)
			release := make(chan struct{})
			setPrivateOperationHook(func(gotPhase, path string) {
				if gotPhase == phase {
					if gotPhase == "save-temp-created" && filepath.Dir(path) != filepath.Join(root, "instances", "abc", "private", "cfg") {
						t.Errorf("temp directory=%q", filepath.Dir(path))
					}
					reached <- path
					<-release
				}
			})
			t.Cleanup(func() { setPrivateOperationHook(nil) })
			done := make(chan error, 1)
			if phase == "save-temp-created" {
				go func() { _, err := a.Save(ctx, "abc", "cfg/saved.cfg", []byte("saved")); done <- err }()
			} else {
				go func() { done <- a.Move(ctx, "abc", "cfg/source.cfg", "cfg/target.cfg", true) }()
			}
			<-reached
			observed := make(chan error, 1)
			go func() {
				if _, err := b.Tree(ctx, "abc"); err != nil {
					observed <- err
					return
				}
				if _, err := b.Diff(ctx, "abc"); err != nil {
					observed <- err
					return
				}
				observed <- b.Apply(ctx, "abc")
			}()
			select {
			case err := <-observed:
				t.Fatalf("observer did not block: %v", err)
			case <-time.After(25 * time.Millisecond):
			}
			close(release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if err := <-observed; err != nil {
				t.Fatal(err)
			}
			setPrivateOperationHook(nil)
		})
	}
}

func TestPrivateMoveIntoAbsentDescendantLeavesTreeUnchanged(t *testing.T) {
	manager := NewPrivateManager(t.TempDir(), 1<<20)
	ctx := context.Background()
	if _, err := manager.Save(ctx, "abc", "tree/keep.cfg", []byte("keep")); err != nil {
		t.Fatal(err)
	}
	before, err := manager.Tree(ctx, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Move(ctx, "abc", "tree", "tree/new/location", false); err == nil {
		t.Fatal("move into absent descendant succeeded")
	}
	after, err := manager.Tree(ctx, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("tree mutated: before=%#v after=%#v", before, after)
	}
	for i := range before {
		if before[i].Path != after[i].Path || before[i].Hash != after[i].Hash {
			t.Fatalf("tree mutated: before=%#v after=%#v", before, after)
		}
	}
}

func TestPrivateManifestRepeatedWrite(t *testing.T) {
	root := t.TempDir()
	manager := NewPrivateManager(root, 1<<20)
	firstHash := strings.Repeat("a", 64)
	secondHash := strings.Repeat("b", 64)
	first := privateManifest{Entries: map[string]manifestEntry{"first.cfg": {Kind: "file", Hash: firstHash, Size: 1}}}
	second := privateManifest{Entries: map[string]manifestEntry{"second.cfg": {Kind: "file", Hash: secondHash, Size: 2}}}
	if err := manager.writePrivateManifest("abc", first); err != nil {
		t.Fatal(err)
	}
	if err := manager.writePrivateManifest("abc", second); err != nil {
		t.Fatal(err)
	}
	loaded, err := manager.readPrivateManifest("abc")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Entries) != 1 || loaded.Entries["second.cfg"].Hash != secondHash {
		t.Fatalf("manifest=%#v", loaded)
	}
}

func TestPrivateManifestReplacementIsAtomicToReaders(t *testing.T) {
	manager := NewPrivateManager(t.TempDir(), 1<<20)
	manifest := func(hash string) privateManifest {
		return privateManifest{Entries: map[string]manifestEntry{"state.cfg": {Kind: "file", Hash: strings.Repeat(hash, 64), Size: 1}}}
	}
	if err := manager.writePrivateManifest("abc", manifest("a")); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	errorsSeen := make(chan error, 1)
	var readers sync.WaitGroup
	for range 8 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				loaded, err := manager.readPrivateManifest("abc")
				if err == nil {
					hash := loaded.Entries["state.cfg"].Hash
					if len(loaded.Entries) != 1 || (hash != strings.Repeat("a", 64) && hash != strings.Repeat("b", 64)) {
						err = errors.New("reader observed missing or partial manifest")
					}
				}
				if err != nil {
					select {
					case errorsSeen <- err:
					default:
					}
					return
				}
			}
		}()
	}
	for i := 0; i < 500; i++ {
		hash := "a"
		if i%2 == 0 {
			hash = "b"
		}
		if err := manager.writePrivateManifest("abc", manifest(hash)); err != nil {
			close(stop)
			readers.Wait()
			t.Fatal(err)
		}
	}
	close(stop)
	readers.Wait()
	select {
	case err := <-errorsSeen:
		t.Fatal(err)
	default:
	}
}

func TestPrivateManifestRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	manager := NewPrivateManager(root, 1<<20)
	manifest := privateManifest{Entries: map[string]manifestEntry{"first.cfg": {Kind: "file", Hash: strings.Repeat("a", 64), Size: 1}}}
	outside := filepath.Join(root, "outside-instance")
	if err := os.MkdirAll(outside, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "instances"), 0750); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(root, "instances", "linked")
	if err := os.Symlink(outside, linked); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := manager.writePrivateManifest("linked", manifest); err == nil {
		t.Fatal("manifest write accepted symlink instance")
	}
	if _, err := os.Stat(filepath.Join(outside, "private-applied.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("manifest escaped instance root: %v", err)
	}

	manifestInstance := filepath.Join(root, "instances", "manifest-link")
	if err := os.MkdirAll(manifestInstance, 0750); err != nil {
		t.Fatal(err)
	}
	outsideManifest := filepath.Join(root, "outside-manifest.json")
	if err := os.WriteFile(outsideManifest, []byte("outside"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideManifest, filepath.Join(manifestInstance, "private-applied.json")); err != nil {
		t.Fatal(err)
	}
	if err := manager.writePrivateManifest("manifest-link", manifest); err == nil {
		t.Fatal("manifest write accepted symlink destination")
	}
	if _, err := manager.readPrivateManifest("manifest-link"); err == nil {
		t.Fatal("manifest read accepted symlink destination")
	}
	raw, err := os.ReadFile(outsideManifest)
	if err != nil || string(raw) != "outside" {
		t.Fatalf("outside manifest changed: %q err=%v", raw, err)
	}
}

func TestPrivateManifestRejectsInvalidEntries(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		entry manifestEntry
	}{
		{name: "parent", path: "../escape.cfg", entry: manifestEntry{Kind: "file", Hash: strings.Repeat("a", 64)}},
		{name: "absolute", path: "C:/escape.cfg", entry: manifestEntry{Kind: "file", Hash: strings.Repeat("a", 64)}},
		{name: "backslash", path: `cfg\escape.cfg`, entry: manifestEntry{Kind: "file", Hash: strings.Repeat("a", 64)}},
		{name: "kind", path: "cfg/x", entry: manifestEntry{Kind: "link"}},
		{name: "directory hash", path: "cfg", entry: manifestEntry{Kind: "directory", Hash: "bad"}},
		{name: "directory size", path: "cfg", entry: manifestEntry{Kind: "directory", Size: 1}},
		{name: "file hash", path: "cfg/x", entry: manifestEntry{Kind: "file", Hash: "bad"}},
		{name: "negative size", path: "cfg/x", entry: manifestEntry{Kind: "file", Hash: strings.Repeat("a", 64), Size: -1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := NewPrivateManager(t.TempDir(), 1<<20)
			path, _ := manager.manifestPath("abc")
			if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(privateManifest{Version: privateManifestVersion, Entries: map[string]manifestEntry{test.path: test.entry}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0640); err != nil {
				t.Fatal(err)
			}
			if _, err := manager.Diff(context.Background(), "abc"); err == nil {
				t.Fatal("diff trusted invalid manifest")
			}
		})
	}
}

func TestPrivateFilesVersionAndApplyLast(t *testing.T) {
	root := t.TempDir()
	manager := NewPrivateManager(root, 1024)
	if _, err := manager.Save(context.Background(), "abc", "cfg/server.cfg", []byte("first")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Save(context.Background(), "abc", "cfg/server.cfg", []byte("private-final")); err != nil {
		t.Fatal(err)
	}
	game := filepath.Join(root, "instances", "abc", "game", "left4dead2")
	_ = os.MkdirAll(filepath.Join(game, "cfg"), 0750)
	_ = os.WriteFile(filepath.Join(game, "cfg", "server.cfg"), []byte("package"), 0640)
	if err := manager.Apply(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(game, "cfg", "server.cfg"))
	if string(raw) != "private-final" {
		t.Fatalf("got %q", raw)
	}
	history, err := manager.History(context.Background(), "abc", "cfg/server.cfg")
	if err != nil || len(history) != 1 {
		t.Fatalf("history=%#v err=%v", history, err)
	}
	if filepath.IsAbs(history[0].Path) || !strings.HasPrefix(history[0].Path, "cfg/server.cfg.") {
		t.Fatalf("history path must be private-root relative, got %q", history[0].Path)
	}
	items, err := manager.List(context.Background(), "abc")
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	loaded, err := manager.Read(context.Background(), "abc", "cfg/server.cfg")
	if err != nil || string(loaded) != "private-final" {
		t.Fatalf("loaded=%q err=%v", loaded, err)
	}
	if err := manager.Delete(context.Background(), "abc", "cfg/server.cfg"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Read(context.Background(), "abc", "cfg/server.cfg"); err == nil {
		t.Fatal("deleted file remains")
	}
}
func TestPrivateFilesRejectEscapeAndOversize(t *testing.T) {
	root := t.TempDir()
	manager := NewPrivateManager(root, 3)
	escaped := filepath.Join(root, "outside", "private", "cfg")
	if err := os.MkdirAll(escaped, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(escaped, "x"), []byte("bad"), 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Save(context.Background(), "abc", "../bad", []byte("x")); err == nil {
		t.Fatal("escape accepted")
	}
	if _, err := manager.Save(context.Background(), "abc", "cfg/x", []byte("long")); err == nil {
		t.Fatal("oversize accepted")
	}
	if _, err := manager.Read(context.Background(), "../outside", "cfg/x"); err == nil {
		t.Fatal("read accepted invalid instance id")
	}
	if _, err := manager.List(context.Background(), "../outside"); err == nil {
		t.Fatal("list accepted invalid instance id")
	}
	if _, err := manager.History(context.Background(), "../outside", "cfg/x"); err == nil {
		t.Fatal("history accepted invalid instance id")
	}
	if err := manager.Apply(context.Background(), "../outside"); err == nil {
		t.Fatal("apply accepted invalid instance id")
	}
}

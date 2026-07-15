package content

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

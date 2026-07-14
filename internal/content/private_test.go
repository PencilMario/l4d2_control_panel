package content

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

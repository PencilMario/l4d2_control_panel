package content

import (
	"context"
	"os"
	"path/filepath"
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
}
func TestPrivateFilesRejectEscapeAndOversize(t *testing.T) {
	manager := NewPrivateManager(t.TempDir(), 3)
	if _, err := manager.Save(context.Background(), "abc", "../bad", []byte("x")); err == nil {
		t.Fatal("escape accepted")
	}
	if _, err := manager.Save(context.Background(), "abc", "cfg/x", []byte("long")); err == nil {
		t.Fatal("oversize accepted")
	}
}

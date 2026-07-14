package content

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyPrivateWinsOverPackageAndShared(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "game")
	private := filepath.Join(root, "private")
	_ = os.MkdirAll(filepath.Join(game, "cfg"), 0750)
	_ = os.MkdirAll(filepath.Join(private, "cfg"), 0750)
	_ = os.WriteFile(filepath.Join(game, "cfg", "server.cfg"), []byte("package"), 0640)
	_ = os.WriteFile(filepath.Join(private, "cfg", "server.cfg"), []byte("private"), 0640)
	if err := ApplyTree(private, game); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(game, "cfg", "server.cfg"))
	if string(got) != "private" {
		t.Fatalf("got %q", got)
	}
}

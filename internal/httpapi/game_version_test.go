package httpapi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSharedGameVersion(t *testing.T) {
	current := t.TempDir()
	gameDir := filepath.Join(current, "left4dead2")
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "steam.inf"), []byte("ClientVersion=123\r\nPatchVersion = 2.2.4.3\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	version, err := readSharedGameVersion(current)
	if err != nil || version != "2.2.4.3" {
		t.Fatalf("version=%q err=%v", version, err)
	}
}

func TestReadSharedGameVersionRejectsMissingPatchVersion(t *testing.T) {
	current := t.TempDir()
	gameDir := filepath.Join(current, "left4dead2")
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gameDir, "steam.inf"), []byte("ClientVersion=123\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readSharedGameVersion(current); err == nil {
		t.Fatal("expected missing PatchVersion error")
	}
}

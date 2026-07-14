package maintenance

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupAndCleanupStayInsideInstanceData(t *testing.T) {
	root := t.TempDir()
	private := filepath.Join(root, "instances", "abc", "private", "cfg")
	_ = os.MkdirAll(private, 0750)
	_ = os.WriteFile(filepath.Join(private, "server.cfg"), []byte("x"), 0640)
	manager := New(root)
	archive, err := manager.Backup(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(archive) != filepath.Join(root, "instances", "abc", "backups") {
		t.Fatalf("archive=%s", archive)
	}
	old := filepath.Join(root, "instances", "abc", "backups", "old.tar.gz")
	_ = os.WriteFile(old, []byte("x"), 0640)
	past := time.Now().Add(-40 * 24 * time.Hour)
	_ = os.Chtimes(old, past, past)
	removed, err := manager.Cleanup(context.Background(), 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if removed < 1 {
		t.Fatalf("removed=%d", removed)
	}
	if _, err := os.Stat(archive); err != nil {
		t.Fatal("fresh backup removed")
	}
}

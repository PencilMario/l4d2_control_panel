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

func TestCanceledBackupDoesNotPublishPartialArchive(t *testing.T) {
	root := t.TempDir()
	private := filepath.Join(root, "instances", "abc", "private")
	if err := os.MkdirAll(private, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(private, "server.cfg"), []byte("data"), 0640); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New(root).Backup(ctx, "abc"); err == nil {
		t.Fatal("canceled backup unexpectedly succeeded")
	}
	backupDir := filepath.Join(root, "instances", "abc", "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("partial backup was published: %v", entries)
	}
}

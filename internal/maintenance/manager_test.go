package maintenance

import (
	"context"
	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type maintenanceLogSink struct {
	mu       sync.Mutex
	messages []string
}

func (s *maintenanceLogSink) Append(_ context.Context, _, _ string, _ joblogs.Level, message string) (joblogs.Record, error) {
	s.mu.Lock()
	s.messages = append(s.messages, message)
	s.mu.Unlock()
	return joblogs.Record{}, nil
}
func (*maintenanceLogSink) Finalize(context.Context, string) error { return nil }

func (s *maintenanceLogSink) joined() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.messages, "\n")
}

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

func TestBackupAndCleanupLogArchiveNamesSizesAndRemovedFiles(t *testing.T) {
	root := t.TempDir()
	private := filepath.Join(root, "instances", "abc", "private")
	if err := os.MkdirAll(private, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(private, "server.cfg"), []byte("configuration"), 0o640); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(root, "instances", "abc", "backups", "old.tar.gz")
	if err := os.MkdirAll(filepath.Dir(old), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-40 * 24 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}

	sink := &maintenanceLogSink{}
	jobManager := jobs.NewManager(jobs.WithLogSink(sink))
	manager := New(root)
	if _, err := jobManager.Start(context.Background(), "abc", "backup_cleanup", func(ctx context.Context, _ jobs.Reporter) error {
		if _, err := manager.Backup(ctx, "abc"); err != nil {
			return err
		}
		_, err := manager.Cleanup(ctx, 30*24*time.Hour)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := jobManager.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	logs := sink.joined()
	for _, want := range []string{"backup started instance=abc", "archive=backup-", "backup completed", "size=", "cleanup started retention=720h0m0s", "deleted file=instances/abc/backups/old.tar.gz", "released=3 bytes", "cleanup completed removed=1"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("logs=%q missing %q", logs, want)
		}
	}
	if strings.Contains(logs, root) {
		t.Fatalf("logs leaked root path %q: %q", root, logs)
	}
}

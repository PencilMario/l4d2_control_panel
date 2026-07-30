package maintenance

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type cleanupInstanceSource struct {
	items []domain.Instance
	err   error
}

func (s cleanupInstanceSource) Instances(context.Context) ([]domain.Instance, error) {
	return s.items, s.err
}

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

func TestCleanupPackagesProtectsSelectedAppliedAndLatest(t *testing.T) {
	root := t.TempDir()
	packages, err := content.NewPackageManager(root)
	if err != nil {
		t.Fatal(err)
	}
	raw := maintenancePackageZip(t)
	created := time.Date(2026, 7, 30, 1, 0, 0, 0, time.UTC)
	add := func(version string, at time.Time) content.PackageVersion {
		t.Helper()
		item, err := packages.AddUpload("plugins.zip", version, bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			t.Fatal(err)
		}
		item.SourceRepository = "owner/repo"
		item.CreatedAt = at
		if err := packages.UpdateMetadata(item); err != nil {
			t.Fatal(err)
		}
		return item
	}
	old := add("old", created)
	selected := add("selected", created.Add(time.Hour))
	applied := add("applied", created.Add(2*time.Hour))
	latest := add("latest", created.Add(3*time.Hour))
	instances := cleanupInstanceSource{items: []domain.Instance{
		{SelectedPackageID: selected.ID},
		{PackageVersion: applied.ID},
	}}
	manager := New(root, WithPackageCleanup(instances, packages))
	sink := &maintenanceLogSink{}
	jobManager := jobs.NewManager(jobs.WithLogSink(sink))
	if _, err := jobManager.Start(context.Background(), "", "cleanup", func(ctx context.Context, _ jobs.Reporter) error {
		_, err := manager.Cleanup(ctx, 30*24*time.Hour)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := jobManager.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := packages.Get(old.ID); !os.IsNotExist(err) {
		t.Fatalf("unreferenced old package retained: %v", err)
	}
	for _, kept := range []content.PackageVersion{selected, applied, latest} {
		if _, err := packages.Get(kept.ID); err != nil {
			t.Fatalf("protected package %s removed: %v", kept.ID, err)
		}
	}
	logs := sink.joined()
	for _, want := range []string{"deleted package package_id=" + old.ID, "repository=owner/repo", "package cleanup completed scanned=4", "kept_latest=1", "kept_referenced=2", "deleted=1"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("logs=%q missing %q", logs, want)
		}
	}
	if strings.Contains(logs, root) {
		t.Fatalf("logs leaked root path %q: %q", root, logs)
	}
}

func TestCleanupPackagesDoesNotDeleteWhenInstanceReadFails(t *testing.T) {
	root := t.TempDir()
	packages, err := content.NewPackageManager(root)
	if err != nil {
		t.Fatal(err)
	}
	raw := maintenancePackageZip(t)
	var old content.PackageVersion
	for index := 0; index < 2; index++ {
		item, err := packages.AddUpload("plugins.zip", string(rune('1'+index)), bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			t.Fatal(err)
		}
		item.SourceRepository = "owner/repo"
		item.CreatedAt = time.Date(2026, 7, 30, index+1, 0, 0, 0, time.UTC)
		if err := packages.UpdateMetadata(item); err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			old = item
		}
	}
	manager := New(root, WithPackageCleanup(cleanupInstanceSource{err: errors.New("read " + root + ": database unavailable")}, packages))
	sink := &maintenanceLogSink{}
	jobManager := jobs.NewManager(jobs.WithLogSink(sink))
	if _, err := jobManager.Start(context.Background(), "", "cleanup", func(ctx context.Context, _ jobs.Reporter) error {
		_, err := manager.Cleanup(ctx, 30*24*time.Hour)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := jobManager.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	logs := sink.joined()
	if !strings.Contains(logs, "database unavailable") || strings.Contains(logs, root) {
		t.Fatalf("unsafe failure logs=%q", logs)
	}
	if _, err := packages.Get(old.ID); err != nil {
		t.Fatalf("package deleted after instance read failure: %v", err)
	}
}

func maintenancePackageZip(t *testing.T) []byte {
	t.Helper()
	var raw bytes.Buffer
	writer := zip.NewWriter(&raw)
	entry, err := writer.Create("cfg/plugin.cfg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return raw.Bytes()
}

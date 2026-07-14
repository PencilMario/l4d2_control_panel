package jobs

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/store"
)

func TestManagerSerializesMutationPerInstance(t *testing.T) {
	m := NewManager()
	release := make(chan struct{})
	started := make(chan struct{})
	var overlap atomic.Bool
	m.Start(context.Background(), "a", "update", func(context.Context, Reporter) error { close(started); <-release; return nil })
	<-started
	job := m.Start(context.Background(), "a", "restart", func(context.Context, Reporter) error { overlap.Store(true); return nil })
	time.Sleep(20 * time.Millisecond)
	if overlap.Load() {
		t.Fatal("jobs overlapped")
	}
	close(release)
	wait(t, m, job.ID)
	if !overlap.Load() {
		t.Fatal("queued job never ran")
	}
}

func TestPersistentManagerReloadsCompletedJob(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	m := NewPersistentManager(db)
	job := m.Start(context.Background(), "a", "install", func(_ context.Context, r Reporter) error { r.Progress("steamcmd", 42, "downloading"); return nil })
	got := wait(t, m, job.ID)
	if got.Status != Succeeded {
		t.Fatalf("job=%#v", got)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	reloaded, ok := NewPersistentManager(db).Get(job.ID)
	if !ok || reloaded.Status != Succeeded || reloaded.Stage != "steamcmd" || reloaded.Percent != 100 {
		t.Fatalf("reloaded=%#v ok=%v", reloaded, ok)
	}
}
func TestReporterPersistsProgress(t *testing.T) {
	m := NewManager()
	job := m.Start(context.Background(), "a", "install", func(_ context.Context, r Reporter) error { r.Progress("steamcmd", 42, "downloading"); return nil })
	got := wait(t, m, job.ID)
	if got.Status != Succeeded || got.Stage != "steamcmd" || got.Percent != 100 {
		t.Fatalf("job=%#v", got)
	}
}
func wait(t *testing.T, m *Manager, id string) Job {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		j, _ := m.Get(id)
		if j.Status == Succeeded || j.Status == Failed {
			return j
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timeout")
	return Job{}
}

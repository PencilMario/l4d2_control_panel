package jobs

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/store"
)

func TestManagerSerializesMutationPerInstance(t *testing.T) {
	m := NewManager()
	release := make(chan struct{})
	started := make(chan struct{})
	var overlap atomic.Bool
	if _, err := m.Start(context.Background(), "a", "update", func(context.Context, Reporter) error { close(started); <-release; return nil }); err != nil {
		t.Fatal(err)
	}
	<-started
	job, err := m.Start(context.Background(), "a", "restart", func(context.Context, Reporter) error { overlap.Store(true); return nil })
	if err != nil {
		t.Fatal(err)
	}
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
	job, err := m.Start(context.Background(), "a", "install", func(_ context.Context, r Reporter) error { r.Progress("steamcmd", 42, "downloading"); return nil })
	if err != nil {
		t.Fatal(err)
	}
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

func TestPersistentManagerMarksStaleRunningJobInterrupted(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	record := domain.JobRecord{ID: "stale", InstanceID: "abc", Type: "install", Status: "running", CreatedAt: now, UpdatedAt: now}
	if err := db.SaveJob(record); err != nil {
		t.Fatal(err)
	}
	manager := NewPersistentManager(db)
	job, ok := manager.Get("stale")
	if !ok || job.Status != Interrupted || job.Error == "" {
		t.Fatalf("job=%#v ok=%v", job, ok)
	}
}
func TestReporterPersistsProgress(t *testing.T) {
	m := NewManager()
	job, err := m.Start(context.Background(), "a", "install", func(_ context.Context, r Reporter) error { r.Progress("steamcmd", 42, "downloading"); return nil })
	if err != nil {
		t.Fatal(err)
	}
	got := wait(t, m, job.ID)
	if got.Status != Succeeded || got.Stage != "steamcmd" || got.Percent != 100 {
		t.Fatalf("job=%#v", got)
	}
}

type failingRepository struct{ err error }

func (r failingRepository) SaveJob(domain.JobRecord) error { return r.err }
func (failingRepository) LoadJob(string) (domain.JobRecord, bool, error) {
	return domain.JobRecord{}, false, nil
}

func TestStartReturnsInitialPersistenceFailureWithoutRunning(t *testing.T) {
	want := errors.New("database unavailable")
	m := NewPersistentManager(failingRepository{err: want})
	ran := atomic.Bool{}
	if _, err := m.Start(context.Background(), "a", "install", func(context.Context, Reporter) error { ran.Store(true); return nil }); !errors.Is(err, want) {
		t.Fatalf("start error=%v", err)
	}
	if ran.Load() {
		t.Fatal("unpersisted job ran")
	}
}

func TestWaitIsBoundedByContextAndDrainsActiveJobs(t *testing.T) {
	m := NewManager()
	release := make(chan struct{})
	if _, err := m.Start(context.Background(), "a", "install", func(context.Context, Reporter) error { <-release; return nil }); err != nil {
		t.Fatal(err)
	}
	timed, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := m.Wait(timed); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bounded wait error=%v", err)
	}
	close(release)
	drained, cancelDrain := context.WithTimeout(context.Background(), time.Second)
	defer cancelDrain()
	if err := m.Wait(drained); err != nil {
		t.Fatal(err)
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

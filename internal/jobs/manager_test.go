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

func TestPersistentManagerRecordsLifecycleEvents(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	manager := NewPersistentManager(db)
	created, err := manager.Start(context.Background(), "a", "install", func(_ context.Context, reporter Reporter) error {
		reporter.Progress("download", 40, "downloading")
		return errors.New("download interrupted")
	})
	if err != nil {
		t.Fatal(err)
	}
	wait(t, manager, created.ID)
	job, events, ok, err := manager.Details(created.ID)
	if err != nil || !ok || job.StartedAt == nil || job.FinishedAt == nil || job.Error != "download interrupted" {
		t.Fatalf("job=%#v events=%#v ok=%v err=%v", job, events, ok, err)
	}
	assertEventKinds(t, events, "queued", "started", "progress", "failed")
	if events[2].Stage != "download" || events[2].Percent != 40 || events[2].Message != "downloading" {
		t.Fatalf("progress event=%#v", events[2])
	}
	if events[3].Message != "download interrupted" {
		t.Fatalf("failed event=%#v", events[3])
	}
}

func TestPersistentManagerPrunesSuccessfulJobsUsingGlobalLimit(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetSuccessfulJobLimit(1); err != nil {
		t.Fatal(err)
	}
	manager := NewPersistentManager(db)
	first, err := manager.Start(context.Background(), "a", "install", func(context.Context, Reporter) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	wait(t, manager, first.ID)
	second, err := manager.Start(context.Background(), "b", "install", func(context.Context, Reporter) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	wait(t, manager, second.ID)
	if err := manager.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, found := manager.Get(first.ID); found {
		t.Fatal("oldest successful job remained accessible")
	}
	if job, found := manager.Get(second.ID); !found || job.Status != Succeeded {
		t.Fatalf("newest job=%#v found=%v", job, found)
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

func (r failingRepository) SaveJobWithEvent(domain.JobRecord, domain.JobEvent) error { return r.err }
func (failingRepository) LoadJob(string) (domain.JobRecord, bool, error) {
	return domain.JobRecord{}, false, nil
}
func (failingRepository) JobEvents(string) ([]domain.JobEvent, error) { return nil, nil }

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

func assertEventKinds(t *testing.T, events []domain.JobEvent, wants ...string) {
	t.Helper()
	if len(events) != len(wants) {
		t.Fatalf("events=%#v wants=%#v", events, wants)
	}
	for index, want := range wants {
		if events[index].Kind != want {
			t.Fatalf("event %d kind=%q want=%q", index, events[index].Kind, want)
		}
	}
}

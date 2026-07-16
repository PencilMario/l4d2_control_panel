package jobs

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
	"github.com/not0721here/l4d2-control-panel/internal/store"
)

type recordedLogCall struct {
	kind, jobID, source, message string
	level                        joblogs.Level
}

type recordingLogSink struct {
	mu    sync.Mutex
	calls []recordedLogCall
}

func (s *recordingLogSink) Append(_ context.Context, jobID, source string, level joblogs.Level, message string) (joblogs.Record, error) {
	s.mu.Lock()
	s.calls = append(s.calls, recordedLogCall{kind: "append", jobID: jobID, source: source, level: level, message: message})
	s.mu.Unlock()
	return joblogs.Record{}, nil
}

func (s *recordingLogSink) Finalize(_ context.Context, jobID string) error {
	s.mu.Lock()
	s.calls = append(s.calls, recordedLogCall{kind: "finalize", jobID: jobID})
	s.mu.Unlock()
	return nil
}

func TestManagerWritesTaskLifecycleAndReporterLogs(t *testing.T) {
	sink := &recordingLogSink{}
	m := NewManager(WithLogSink(sink))
	job, err := m.Start(context.Background(), "a", "install", func(_ context.Context, reporter Reporter) error {
		reporter.Progress("download", 40, "downloading")
		reporter.Log("steamcmd", joblogs.Output, "raw output")
		return errors.New("download interrupted")
	})
	if err != nil {
		t.Fatal(err)
	}
	wait(t, m, job.ID)
	if err := m.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	wants := []recordedLogCall{
		{kind: "append", jobID: job.ID, source: "task", level: joblogs.Info, message: "Task queued"},
		{kind: "append", jobID: job.ID, source: "task", level: joblogs.Info, message: "Task started"},
		{kind: "append", jobID: job.ID, source: "task", level: joblogs.Info, message: "downloading"},
		{kind: "append", jobID: job.ID, source: "steamcmd", level: joblogs.Output, message: "raw output"},
		{kind: "append", jobID: job.ID, source: "task", level: joblogs.Error, message: "download interrupted"},
		{kind: "finalize", jobID: job.ID},
	}
	if !reflect.DeepEqual(sink.calls, wants) {
		t.Fatalf("calls=%#v want=%#v", sink.calls, wants)
	}
}

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
	if !ok || reloaded.Status != Succeeded || reloaded.Stage != "complete" || reloaded.Message != "Task completed" || reloaded.Percent != 100 {
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

func TestPersistentManagerPrunesTerminalJobsAfterFailureUsingGlobalLimit(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetCompletedJobLimit(1); err != nil {
		t.Fatal(err)
	}
	manager := NewPersistentManager(db)
	first, err := manager.Start(context.Background(), "a", "install", func(context.Context, Reporter) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	wait(t, manager, first.ID)
	second, err := manager.Start(context.Background(), "b", "install", func(context.Context, Reporter) error {
		return errors.New("install failed")
	})
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
	if job, found := manager.Get(second.ID); !found || job.Status != Failed {
		t.Fatalf("newest job=%#v found=%v", job, found)
	}
}

func TestPersistentManagerDoesNotExposeUnpersistedTerminalState(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := &failNthLifecycleWriteRepository{
		Store: db,
		at:    3,
		err:   errors.New("terminal write failed"),
	}
	manager := NewPersistentManager(repo)
	created, err := manager.Start(context.Background(), "a", "install", func(context.Context, Reporter) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}

	job, events, found, err := manager.Details(created.ID)
	if err != nil || !found || job.Status != Running {
		t.Fatalf("job=%#v events=%#v found=%v err=%v", job, events, found, err)
	}
	assertEventKinds(t, events, "queued", "started")
	stored, found, err := db.LoadJob(created.ID)
	if err != nil || !found || stored.Status != string(Running) {
		t.Fatalf("stored=%#v found=%v err=%v", stored, found, err)
	}
}

func TestPersistentManagerDoesNotRunWhenStartedStatePersistenceFails(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := &failNthLifecycleWriteRepository{
		Store: db,
		at:    2,
		err:   errors.New("started write failed"),
	}
	manager := NewPersistentManager(repo)
	ran := atomic.Bool{}
	created, err := manager.Start(context.Background(), "a", "install", func(context.Context, Reporter) error {
		ran.Store(true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ran.Load() {
		t.Fatal("operation ran without a durable started transition")
	}
	job, events, found, err := manager.Details(created.ID)
	if err != nil || !found || job.Status != Failed || job.Error == "" || job.StartedAt != nil {
		t.Fatalf("job=%#v events=%#v found=%v err=%v", job, events, found, err)
	}
	assertEventKinds(t, events, "queued", "failed")
}

func TestPersistentManagerWritesTerminalSummaryWithoutProgress(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	manager := NewPersistentManager(db)
	created, err := manager.Start(context.Background(), "a", "install", func(context.Context, Reporter) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	wait(t, manager, created.ID)
	job, found := manager.Get(created.ID)
	if !found || job.Status != Succeeded || job.Stage != "complete" || job.Message != "Task completed" {
		t.Fatalf("job=%#v found=%v", job, found)
	}
}

func TestPersistentManagerMarksStaleRunningJobInterrupted(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetCompletedJobLimit(1); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	olderFinished := now.Add(-time.Minute)
	if err := db.SaveJob(domain.JobRecord{
		ID: "older-failed", Status: "failed", Error: "boom", CreatedAt: olderFinished, UpdatedAt: olderFinished, FinishedAt: &olderFinished,
	}); err != nil {
		t.Fatal(err)
	}
	record := domain.JobRecord{ID: "stale", InstanceID: "abc", Type: "install", Status: "running", CreatedAt: now, UpdatedAt: now}
	if err := db.SaveJob(record); err != nil {
		t.Fatal(err)
	}
	manager := NewPersistentManager(db)
	job, ok := manager.Get("stale")
	if !ok || job.Status != Interrupted || job.Error == "" {
		t.Fatalf("job=%#v ok=%v", job, ok)
	}
	if _, ok := manager.Get("older-failed"); ok {
		t.Fatal("older terminal job remained after startup recovery")
	}
}
func TestReporterPersistsProgress(t *testing.T) {
	m := NewManager()
	job, err := m.Start(context.Background(), "a", "install", func(_ context.Context, r Reporter) error { r.Progress("steamcmd", 42, "downloading"); return nil })
	if err != nil {
		t.Fatal(err)
	}
	got := wait(t, m, job.ID)
	if got.Status != Succeeded || got.Stage != "complete" || got.Message != "Task completed" || got.Percent != 100 {
		t.Fatalf("job=%#v", got)
	}
	_, events, found, err := m.Details(job.ID)
	if err != nil || !found {
		t.Fatalf("events=%#v found=%v err=%v", events, found, err)
	}
	assertEventKinds(t, events, "queued", "started", "progress", "succeeded")
	if events[2].Stage != "steamcmd" || events[2].Message != "downloading" {
		t.Fatalf("progress event=%#v", events[2])
	}
}

type failingRepository struct{ err error }

func (r failingRepository) SaveJobWithEvent(domain.JobRecord, domain.JobEvent) error { return r.err }
func (failingRepository) LoadJob(string) (domain.JobRecord, bool, error) {
	return domain.JobRecord{}, false, nil
}
func (failingRepository) JobEvents(string) ([]domain.JobEvent, error) { return nil, nil }

type failNthLifecycleWriteRepository struct {
	*store.Store
	calls atomic.Int32
	at    int32
	err   error
}

func (r *failNthLifecycleWriteRepository) SaveJobWithEvent(record domain.JobRecord, event domain.JobEvent) error {
	if r.calls.Add(1) == r.at {
		return r.err
	}
	return r.Store.SaveJobWithEvent(record, event)
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

package gamelogs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
)

type schedulerRepo struct {
	mu        sync.Mutex
	instances []domain.Instance
	active    map[string]bool
	days      int
	fail      map[string]error
	records   map[string]domain.JobRecord
}

func (r *schedulerRepo) Instances(context.Context) ([]domain.Instance, error) {
	return r.instances, nil
}
func (r *schedulerRepo) GameLogRetentionDays() (int, error) { return r.days, nil }
func (r *schedulerRepo) HasActiveJob(_ context.Context, id, kind string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active[id+":"+kind], r.fail[id]
}
func (r *schedulerRepo) SaveJobWithEvent(v domain.JobRecord, _ domain.JobEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.records == nil {
		r.records = map[string]domain.JobRecord{}
	}
	r.records[v.ID] = v
	if v.Status == "pending" || v.Status == "running" {
		r.active[v.InstanceID+":"+v.Type] = true
	} else {
		delete(r.active, v.InstanceID+":"+v.Type)
	}
	return nil
}
func (r *schedulerRepo) LoadJob(id string) (domain.JobRecord, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.records[id]
	return v, ok, nil
}
func (r *schedulerRepo) JobEvents(string) ([]domain.JobEvent, error) { return nil, nil }

type memoryLogs struct {
	mu       sync.Mutex
	messages []string
}

func (l *memoryLogs) Append(_ context.Context, _, _ string, _ joblogs.Level, message string) (joblogs.Record, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.messages = append(l.messages, message)
	return joblogs.Record{}, nil
}
func (*memoryLogs) Finalize(context.Context, string) error { return nil }

func TestEnqueueAllDeduplicatesConcurrentPersistentJobsAndContinuesFailures(t *testing.T) {
	repo := &schedulerRepo{instances: []domain.Instance{{ID: "a"}, {ID: "b"}, {ID: "c"}}, active: map[string]bool{"a:" + CleanupJobType: true}, days: 14, fail: map[string]error{"c": errors.New("db unavailable")}}
	manager := jobs.NewPersistentManager(repo)
	s := NewScheduler(repo, manager, NewManager(t.TempDir(), Options{}))
	var wg sync.WaitGroup
	results := make(chan EnqueueResult, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- s.EnqueueAll(context.Background()) }()
	}
	wg.Wait()
	close(results)
	totalQueued := 0
	for result := range results {
		totalQueued += result.Queued
	}
	if totalQueued != 1 {
		t.Fatalf("queued=%d, want 1", totalQueued)
	}
	first := s.EnqueueAll(context.Background())
	if first.Deduplicated != 2 || first.Failed != 1 {
		t.Fatalf("result=%+v", first)
	}
	if err := manager.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupJobReadsCurrentRetentionAndReportsSummaryOnPartialFailure(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "instances", "i", "logs", "game", "old.log")
	bad := filepath.Join(root, "instances", "i", "logs", "game", "bad.log")
	writeFile(t, old, "123")
	writeFile(t, bad, "4567")
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	_ = os.Chtimes(old, now.Add(-20*24*time.Hour), now.Add(-20*24*time.Hour))
	_ = os.Chtimes(bad, now.Add(-20*24*time.Hour), now.Add(-20*24*time.Hour))
	repo := &schedulerRepo{instances: []domain.Instance{{ID: "i"}}, active: map[string]bool{}, days: 14}
	logs := &memoryLogs{}
	jm := jobs.NewPersistentManager(repo, jobs.WithLogSink(logs))
	cleaner := NewManager(root, Options{Now: func() time.Time { return now }, Remove: func(path string, _ os.FileInfo) error {
		if strings.HasSuffix(path, "bad.log") {
			return errors.New("denied")
		}
		return os.Remove(path)
	}})
	result := NewScheduler(repo, jm, cleaner).EnqueueAll(context.Background())
	if result.Queued != 1 {
		t.Fatalf("result=%+v", result)
	}
	repo.days = 7
	if err := jm.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	job, ok := jm.Get(result.JobIDs[0])
	if !ok || job.Status != jobs.Failed {
		t.Fatalf("job=%+v ok=%v", job, ok)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("successful deletion rolled back: %v", err)
	}
	joined := strings.Join(logs.messages, "\n")
	for _, want := range []string{"retention=7", "cutoff=2026-07-11T12:00:00Z", "Scanned=2", "Expired=2", "Deleted=1", "Skipped=0", "ReleasedBytes=3", "Failures=1"} {
		if !strings.Contains(joined, want) {
			t.Errorf("logs missing %q:\n%s", want, joined)
		}
	}
}

func TestSchedulerStartIsIdempotentAndStopWaits(t *testing.T) {
	repo := &schedulerRepo{active: map[string]bool{}, days: 14}
	s := NewScheduler(repo, jobs.NewManager(), NewManager(t.TempDir(), Options{}))
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	if got := len(s.cron.Entries()); got != 1 {
		t.Fatalf("cron entries=%d", got)
	}
	s.Stop()
}

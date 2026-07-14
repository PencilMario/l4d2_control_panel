package jobs

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
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

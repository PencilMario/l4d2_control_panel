package vpkrestart

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
	"github.com/not0721here/l4d2-control-panel/internal/players"
)

type memoryRepo struct {
	mu        sync.Mutex
	instances []domain.Instance
	items     map[string]domain.VPKRestart
}

func (r *memoryRepo) Instances(context.Context) ([]domain.Instance, error) { return r.instances, nil }
func (r *memoryRepo) Instance(_ context.Context, id string) (domain.Instance, error) {
	for _, item := range r.instances {
		if item.ID == id {
			return item, nil
		}
	}
	return domain.Instance{}, errors.New("not found")
}
func (r *memoryRepo) UpsertVPKRestart(_ context.Context, v domain.VPKRestart) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old, ok := r.items[v.InstanceID]; ok {
		old.PublicationID = v.PublicationID
		if old.JobID == "" {
			old.JobID = v.JobID
		}
		r.items[v.InstanceID] = old
	} else {
		v.Status = "waiting"
		r.items[v.InstanceID] = v
	}
	return nil
}
func (r *memoryRepo) PendingVPKRestarts(context.Context) ([]domain.VPKRestart, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []domain.VPKRestart{}
	for _, v := range r.items {
		if v.Status == "waiting" || v.Status == "retry" || v.Status == "queued" {
			out = append(out, v)
		}
	}
	return out, nil
}
func (r *memoryRepo) ClaimVPKRestart(_ context.Context, id string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v := r.items[id]
	if v.Status != "waiting" && v.Status != "retry" {
		return false, nil
	}
	v.Status = "queued"
	r.items[id] = v
	return true, nil
}
func (r *memoryRepo) UpdateVPKRestart(_ context.Context, id, status string, failures int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	v := r.items[id]
	v.Status, v.Failures = status, failures
	r.items[id] = v
	return nil
}

type fakePlayers struct {
	counts []int
	errors []error
	calls  int
}

func (p *fakePlayers) Online(context.Context, string) (players.Snapshot, error) {
	i := p.calls
	p.calls++
	if i < len(p.errors) && p.errors[i] != nil {
		return players.Snapshot{}, p.errors[i]
	}
	n := 0
	if i < len(p.counts) {
		n = p.counts[i]
	}
	return players.Snapshot{Players: make([]players.OnlinePlayer, n)}, nil
}

type fakeLife struct {
	restarts int
	err      error
}

func (l *fakeLife) Restart(context.Context, string) error { l.restarts++; return l.err }

type immediateJobs struct{ starts int }

func (j *immediateJobs) StartWithOptions(ctx context.Context, id, kind string, options jobs.StartOptions, fn func(context.Context, jobs.Reporter) error) (jobs.Job, error) {
	j.starts++
	if options.Preflight != nil {
		if err := options.Preflight(ctx, nil); err != nil {
			return jobs.Job{}, err
		}
	}
	return jobs.Job{ID: "job-1"}, fn(ctx, nil)
}

type restartLogSink struct {
	mu       sync.Mutex
	messages []string
}

func (s *restartLogSink) Append(_ context.Context, _, _ string, _ joblogs.Level, message string) (joblogs.Record, error) {
	s.mu.Lock()
	s.messages = append(s.messages, message)
	s.mu.Unlock()
	return joblogs.Record{}, nil
}
func (*restartLogSink) Finalize(context.Context, string) error { return nil }
func (s *restartLogSink) joined() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.messages, "\n")
}

func running(id, container string) domain.Instance {
	return domain.Instance{ID: id, ContainerID: container, DesiredState: domain.StateRunning, ActualState: domain.StateRunning}
}

func TestRegisterMergesRunningInstancesAndSkipsStopped(t *testing.T) {
	r := &memoryRepo{instances: []domain.Instance{running("a", "c1"), {ID: "b", DesiredState: domain.StateStopped, ActualState: domain.StateStopped}}, items: map[string]domain.VPKRestart{}}
	c := New(r, &fakePlayers{}, &fakeLife{}, &immediateJobs{})
	n, err := c.Register(context.Background(), "hash-1")
	if err != nil || n != 1 {
		t.Fatalf("count=%d err=%v", n, err)
	}
	_, _ = c.Register(context.Background(), "hash-2")
	if len(r.items) != 1 || r.items["a"].ContainerID != "c1" || r.items["a"].PublicationID != "hash-2" {
		t.Fatalf("items=%#v", r.items)
	}
}

func TestRegisterCreatesVisibleJobImmediately(t *testing.T) {
	r := &memoryRepo{instances: []domain.Instance{running("a", "c1")}, items: map[string]domain.VPKRestart{}}
	jobsStarted := &immediateJobs{}
	c := New(r, &fakePlayers{}, &fakeLife{}, jobsStarted)
	if count, err := c.Register(context.Background(), "hash"); err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if jobsStarted.starts != 1 || r.items["a"].JobID == "" {
		t.Fatalf("starts=%d item=%#v", jobsStarted.starts, r.items["a"])
	}
}

func TestVPKJobWaitsForPlayersThenRestarts(t *testing.T) {
	r := &memoryRepo{instances: []domain.Instance{running("a", "c1")}, items: map[string]domain.VPKRestart{}}
	life := &fakeLife{}
	c := New(r, &fakePlayers{counts: []int{1, 0}}, life, &immediateJobs{})
	c.interval = time.Millisecond
	if _, err := c.Register(context.Background(), "hash"); err != nil {
		t.Fatal(err)
	}
	if life.restarts != 1 {
		t.Fatalf("restarts=%d players=%d", life.restarts, 2)
	}
}

func TestVPKJobForcesRestartAfterTimeoutDespitePlayerQueryFailures(t *testing.T) {
	r := &memoryRepo{instances: []domain.Instance{running("a", "c1")}, items: map[string]domain.VPKRestart{}}
	life := &fakeLife{}
	c := New(r, &fakePlayers{counts: []int{0}, errors: []error{errors.New("offline")}}, life, &immediateJobs{})
	c.interval = time.Millisecond
	c.waitTimeout = 5 * time.Millisecond
	if _, err := c.Register(context.Background(), "hash"); err != nil {
		t.Fatal(err)
	}
	if life.restarts != 1 {
		t.Fatalf("restarts=%d", life.restarts)
	}
}

func TestCheckRestartsWhenEmpty(t *testing.T) {
	r := &memoryRepo{instances: []domain.Instance{running("a", "c1")}, items: map[string]domain.VPKRestart{"a": {InstanceID: "a", ContainerID: "c1", Status: "waiting"}}}
	life := &fakeLife{}
	sink := &restartLogSink{}
	queue := jobs.NewManager(jobs.WithLogSink(sink))
	c := New(r, &fakePlayers{counts: []int{0}}, life, queue)
	if err := c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := queue.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if life.restarts != 1 || r.items["a"].Status != "completed" {
		t.Fatalf("life=%d item=%#v", life.restarts, r.items["a"])
	}
	if logs := sink.joined(); !strings.Contains(logs, "restart readiness confirmed instance=a reason=no-online-players query_failures=0") {
		t.Fatalf("logs=%q", logs)
	}
}

func TestCheckRestartsAfterThreeConsecutiveFailures(t *testing.T) {
	r := &memoryRepo{instances: []domain.Instance{running("a", "c1")}, items: map[string]domain.VPKRestart{"a": {InstanceID: "a", ContainerID: "c1", Status: "waiting"}}}
	life := &fakeLife{}
	c := New(r, &fakePlayers{errors: []error{errors.New("x"), errors.New("x"), errors.New("x")}}, life, &immediateJobs{})
	for i := 0; i < 3; i++ {
		if err := c.Check(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if life.restarts != 1 || r.items["a"].Status != "completed" {
		t.Fatalf("restarts=%d item=%#v", life.restarts, r.items["a"])
	}
}

func TestCheckCancelsStoppedAndCompletesChangedContainer(t *testing.T) {
	r := &memoryRepo{instances: []domain.Instance{{ID: "stopped", ContainerID: "c1", DesiredState: domain.StateStopped, ActualState: domain.StateStopped}, running("changed", "c2")}, items: map[string]domain.VPKRestart{
		"stopped": {InstanceID: "stopped", ContainerID: "c1", Status: "waiting"}, "changed": {InstanceID: "changed", ContainerID: "c1", Status: "waiting"},
	}}
	life := &fakeLife{}
	c := New(r, &fakePlayers{}, life, &immediateJobs{})
	if err := c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.items["stopped"].Status != "cancelled" || r.items["changed"].Status != "completed" || life.restarts != 0 {
		t.Fatalf("items=%#v restarts=%d", r.items, life.restarts)
	}
}

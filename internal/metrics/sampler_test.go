package metrics

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/docker"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/players"
	"github.com/not0721here/l4d2-control-panel/internal/traffic"
)

type fakeInstances struct {
	mu    sync.Mutex
	items []domain.Instance
	err   error
	fn    func(context.Context) ([]domain.Instance, error)
}

func (f *fakeInstances) Instances(ctx context.Context) ([]domain.Instance, error) {
	if f.fn != nil {
		return f.fn(ctx)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]domain.Instance(nil), f.items...), f.err
}

type fakeRuntime struct {
	mu                   sync.Mutex
	runtime              map[string]docker.RuntimeState
	stats                map[string]docker.ResourceStats
	runtimeErr, statsErr error
	runtimeFn            func(context.Context, string) (docker.RuntimeState, error)
	statsDeadlineSeen    bool
}

func (f *fakeRuntime) Runtime(ctx context.Context, id string) (docker.RuntimeState, error) {
	if f.runtimeFn != nil {
		return f.runtimeFn(ctx, id)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, v := range f.runtime {
		return v, f.runtimeErr
	}
	return docker.RuntimeState{}, f.runtimeErr
}
func (f *fakeRuntime) Stats(ctx context.Context, _ string) (docker.ResourceStats, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, f.statsDeadlineSeen = ctx.Deadline()
	for _, v := range f.stats {
		return v, f.statsErr
	}
	return docker.ResourceStats{}, f.statsErr
}

type fakeTraffic struct {
	mu               sync.Mutex
	totals           traffic.Totals
	err              error
	registered       []traffic.Session
	stopped          []traffic.Session
	respectContext   bool
	deadlineSeen     bool
	stopFn           func(context.Context, string, string) error
	stopDeadlineSeen bool
}

func (f *fakeTraffic) Register(_ context.Context, s traffic.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.registered = append(f.registered, s)
	return f.err
}
func (f *fakeTraffic) Stop(ctx context.Context, id, run string) error {
	f.mu.Lock()
	f.stopped = append(f.stopped, traffic.Session{InstanceID: id, RunID: run})
	_, f.stopDeadlineSeen = ctx.Deadline()
	stopFn := f.stopFn
	err := f.err
	f.mu.Unlock()
	if stopFn != nil {
		return stopFn(ctx, id, run)
	}
	return err
}
func (f *fakeTraffic) Totals(ctx context.Context, _ string) (traffic.Totals, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, f.deadlineSeen = ctx.Deadline()
	if f.respectContext && ctx.Err() != nil {
		return traffic.Totals{}, ctx.Err()
	}
	return f.totals, f.err
}

type fakePlayers struct {
	summary        players.Summary
	err            error
	clock          *fakeClock
	advance        time.Duration
	respectContext bool
	deadlineSeen   bool
}

func (f *fakePlayers) Summary(ctx context.Context, _ string) (players.Summary, error) {
	_, f.deadlineSeen = ctx.Deadline()
	if f.respectContext && ctx.Err() != nil {
		return players.Summary{}, ctx.Err()
	}
	if f.clock != nil {
		f.clock.Advance(f.advance)
	}
	return f.summary, f.err
}

func TestRuntimeDeadlineDoesNotConsumeIndependentSourceBudgets(t *testing.T) {
	now := time.Now().UTC()
	s, _, runtime, network, player, _ := testSampler(now)
	player.advance = 5 * time.Millisecond
	s.Sample(context.Background())
	prior, _ := s.Latest("one")
	runtimeDeadlineSeen := false
	runtime.runtimeFn = func(ctx context.Context, _ string) (docker.RuntimeState, error) {
		_, runtimeDeadlineSeen = ctx.Deadline()
		<-ctx.Done()
		return docker.RuntimeState{}, ctx.Err()
	}
	network.respectContext = true
	player.respectContext = true
	s.instanceTimeout = 20 * time.Millisecond
	s.Sample(context.Background())
	got, _ := s.Latest("one")
	if got.ContainerRunning != nil || got.RunID != prior.RunID {
		t.Fatalf("runtime state=%+v run=%q", got.ContainerRunning, got.RunID)
	}
	if got.NetworkRXBytes == nil || *got.NetworkRXBytes != network.totals.RXBytes || got.Map == nil || *got.Map != player.summary.Map || got.A2SLatencyMS == nil {
		t.Fatalf("independent sources lost after runtime deadline: %+v", got)
	}
	if !hasIssue(got.Issues, "runtime") || !runtimeDeadlineSeen || !runtime.statsDeadlineSeen || !network.deadlineSeen || !player.deadlineSeen {
		t.Fatalf("issues=%+v deadlines runtime=%v stats=%v traffic=%v player=%v", got.Issues, runtimeDeadlineSeen, runtime.statsDeadlineSeen, network.deadlineSeen, player.deadlineSeen)
	}
}

type fakeTicker struct {
	ch   chan time.Time
	once sync.Once
}

func (t *fakeTicker) C() <-chan time.Time { return t.ch }
func (t *fakeTicker) Stop()               { t.once.Do(func() { close(t.ch) }) }

type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	ticker *fakeTicker
}

func newFakeClock(now time.Time) *fakeClock {
	return &fakeClock{now: now, ticker: &fakeTicker{ch: make(chan time.Time, 1000)}}
}
func (c *fakeClock) Now() time.Time                 { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *fakeClock) NewTicker(time.Duration) Ticker { return c.ticker }
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	now := c.now
	c.mu.Unlock()
	c.ticker.ch <- now
}

func testSampler(now time.Time) (*Sampler, *fakeInstances, *fakeRuntime, *fakeTraffic, *fakePlayers, *fakeClock) {
	instances := &fakeInstances{items: []domain.Instance{{ID: "one", ContainerID: "container", GamePort: 27015, SourceTVPort: 27020, PluginPorts: []int{27015, 27021}}}}
	runtime := &fakeRuntime{runtime: map[string]docker.RuntimeState{"container": {Running: true, StartedAt: now.Add(-time.Minute)}}, stats: map[string]docker.ResourceStats{"container": {CPUPercent: 0, MemoryBytes: 0, MemoryLimitBytes: 100, BlockReadBytes: 100, BlockWriteBytes: 200, PIDs: 0}}}
	trafficProvider := &fakeTraffic{totals: traffic.Totals{RXBytes: 1000, TXBytes: 2000}}
	clock := newFakeClock(now)
	playerProvider := &fakePlayers{summary: players.Summary{Map: "c1m1_hotel", Players: 2, MaxPlayers: 8}, clock: clock, advance: 25 * time.Millisecond}
	return New(instances, runtime, trafficProvider, playerProvider, clock), instances, runtime, trafficProvider, playerProvider, clock
}

func waitLatest(t *testing.T, s *Sampler, id string, timestamp time.Time) Snapshot {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		if got, ok := s.Latest(id); ok && !got.Timestamp.Before(timestamp) {
			return got
		}
		select {
		case <-deadline:
			t.Fatal("sample not published")
		default:
		}
	}
}

func TestStartSamplesImmediatelyTicksAndStops(t *testing.T) {
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	s, _, _, _, _, clock := testSampler(now)
	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)
	first := waitLatest(t, s, "one", now)
	if first.RunID == "" {
		t.Fatal("missing run ID")
	}
	clock.Advance(5 * time.Second)
	second := waitLatest(t, s, "one", now.Add(5*time.Second))
	if second.ContainerRunning == nil || !*second.ContainerRunning {
		t.Fatal("running=false")
	}
	cancel()
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRatesMemoryUptimeLatencyAndPorts(t *testing.T) {
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	s, _, runtime, network, _, clock := testSampler(now)
	s.Sample(context.Background())
	first, _ := s.Latest("one")
	if first.NetworkRXBytesPerSecond != nil || first.BlockReadBytesPerSecond != nil {
		t.Fatal("first rates must be nil")
	}
	if first.CPUPercent == nil || *first.CPUPercent != 0 || first.MemoryPercent == nil || *first.MemoryPercent != 0 || first.PIDs == nil || *first.PIDs != 0 {
		t.Fatalf("legitimate zero lost: %+v", first)
	}
	if first.UptimeSeconds == nil || *first.UptimeSeconds != 60 {
		t.Fatalf("uptime=%v", first.UptimeSeconds)
	}
	if first.A2SLatencyMS == nil || *first.A2SLatencyMS != 25 {
		t.Fatalf("latency=%v", first.A2SLatencyMS)
	}
	if len(network.registered) != 1 || len(network.registered[0].Ports) != 3 {
		t.Fatalf("registered=%+v", network.registered)
	}
	clock.mu.Lock()
	clock.now = now.Add(5 * time.Second)
	clock.mu.Unlock()
	runtime.stats["container"] = docker.ResourceStats{CPUPercent: 10, MemoryBytes: 50, MemoryLimitBytes: 100, BlockReadBytes: 150, BlockWriteBytes: 300, PIDs: 4}
	network.totals = traffic.Totals{RXBytes: 1250, TXBytes: 2500}
	s.Sample(context.Background())
	second, _ := s.Latest("one")
	if second.NetworkRXBytesPerSecond == nil || *second.NetworkRXBytesPerSecond != 50 || second.NetworkTXBytesPerSecond == nil || *second.NetworkTXBytesPerSecond != 100 {
		t.Fatalf("network rates=%v/%v", second.NetworkRXBytesPerSecond, second.NetworkTXBytesPerSecond)
	}
	if second.BlockReadBytesPerSecond == nil || *second.BlockReadBytesPerSecond != 10 || second.BlockWriteBytesPerSecond == nil || *second.BlockWriteBytesPerSecond != 20 {
		t.Fatalf("block rates=%v/%v", second.BlockReadBytesPerSecond, second.BlockWriteBytesPerSecond)
	}
}

func TestInvalidRateIntervalsRollbackAndRunChange(t *testing.T) {
	now := time.Now().UTC()
	for _, tc := range []struct {
		name                string
		elapsed             time.Duration
		rollback, runChange bool
	}{
		{"nonpositive", 0, false, false}, {"too long", 31 * time.Second, false, false}, {"rollback", 5 * time.Second, true, false}, {"run change", 5 * time.Second, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, runtime, network, player, clock := testSampler(now)
			player.advance = 0
			s.Sample(context.Background())
			clock.mu.Lock()
			clock.now = clock.now.Add(tc.elapsed)
			clock.mu.Unlock()
			if tc.rollback {
				network.totals.RXBytes = 1
				runtime.stats["container"] = docker.ResourceStats{BlockReadBytes: 1}
			} else {
				network.totals.RXBytes = 2000
				runtime.stats["container"] = docker.ResourceStats{BlockReadBytes: 200}
			}
			if tc.runChange {
				runtime.runtime["container"] = docker.RuntimeState{Running: true, StartedAt: now.Add(time.Second)}
			}
			s.Sample(context.Background())
			got, _ := s.Latest("one")
			if got.NetworkRXBytesPerSecond != nil || got.BlockReadBytesPerSecond != nil {
				t.Fatalf("rates not nil: %+v", got)
			}
		})
	}
}

func TestPartialFailuresAndStoppedGap(t *testing.T) {
	now := time.Now().UTC()
	s, _, runtime, network, player, _ := testSampler(now)
	runtime.statsErr = errors.New("docker down")
	network.err = errors.New("traffic down")
	player.err = errors.New("a2s down")
	s.Sample(context.Background())
	got, _ := s.Latest("one")
	if got.CPUPercent != nil || got.NetworkRXBytes != nil || got.Map != nil || len(got.Issues) != 4 {
		t.Fatalf("partial snapshot=%+v", got)
	}
	runtime.statsErr = nil
	network.err = nil
	player.err = nil
	s.Sample(context.Background())
	running, _ := s.Latest("one")
	runtime.runtime["container"] = docker.RuntimeState{Running: false}
	s.Sample(context.Background())
	gap, _ := s.Latest("one")
	if gap.ContainerRunning == nil || *gap.ContainerRunning || gap.CPUPercent != nil || len(network.stopped) == 0 || network.stopped[len(network.stopped)-1].RunID != running.RunID {
		t.Fatalf("gap=%+v stops=%+v", gap, network.stopped)
	}
	s.Sample(context.Background())
	if len(network.stopped) != 1 {
		t.Fatalf("stopped calls=%+v", network.stopped)
	}
}

func TestHistoryRetains720CopiesAndPrunesRemovedInstances(t *testing.T) {
	now := time.Now().UTC()
	s, instances, _, _, _, clock := testSampler(now)
	for i := 0; i < 725; i++ {
		s.Sample(context.Background())
		clock.mu.Lock()
		clock.now = clock.now.Add(5 * time.Second)
		clock.mu.Unlock()
	}
	history := s.History("one")
	if len(history) != 720 {
		t.Fatalf("len=%d", len(history))
	}
	if !history[0].Timestamp.Before(history[719].Timestamp) {
		t.Fatal("history not chronological")
	}
	history[0].RunID = "mutated"
	fresh := s.History("one")
	if fresh[0].RunID == "mutated" {
		t.Fatal("history aliases internal storage")
	}
	instances.mu.Lock()
	instances.items = nil
	instances.mu.Unlock()
	s.Sample(context.Background())
	if _, ok := s.Latest("one"); ok || len(s.History("one")) != 0 {
		t.Fatal("removed instance retained")
	}
}

func TestRuntimeFailurePublishesUnknownPreservesSessionAndSamplesA2S(t *testing.T) {
	now := time.Now().UTC()
	s, _, runtime, network, _, clock := testSampler(now)
	s.Sample(context.Background())
	prior, _ := s.Latest("one")
	runtime.runtimeErr = errors.New("runtime unavailable")
	clock.mu.Lock()
	clock.now = clock.now.Add(5 * time.Second)
	clock.mu.Unlock()
	s.Sample(context.Background())
	got, _ := s.Latest("one")
	if got.ContainerRunning != nil {
		t.Fatalf("running must be unknown: %v", *got.ContainerRunning)
	}
	if got.RunID != prior.RunID || len(network.stopped) != 0 {
		t.Fatalf("prior session changed: run=%q stops=%+v", got.RunID, network.stopped)
	}
	if got.Map == nil || *got.Map != "c1m1_hotel" || got.A2SLatencyMS == nil || *got.A2SLatencyMS != 25 {
		t.Fatalf("A2S not isolated: %+v", got)
	}
	if got.CPUPercent == nil || got.NetworkRXBytes == nil {
		t.Fatalf("independent Docker/traffic sources were suppressed: %+v", got)
	}
	if !hasIssue(got.Issues, "runtime") {
		t.Fatalf("issues=%+v", got.Issues)
	}
}

func TestRemovedInstanceStopsPriorSessionAndPrunesEvenWhenStopFails(t *testing.T) {
	now := time.Now().UTC()
	for _, stopErr := range []error{nil, errors.New("stop failed")} {
		s, instances, _, network, _, _ := testSampler(now)
		s.Sample(context.Background())
		prior, _ := s.Latest("one")
		network.err = stopErr
		instances.mu.Lock()
		instances.items = nil
		instances.mu.Unlock()
		s.Sample(context.Background())
		if len(network.stopped) != 1 || network.stopped[0].RunID != prior.RunID {
			t.Fatalf("stop=%+v", network.stopped)
		}
		if _, ok := s.Latest("one"); ok || len(s.History("one")) != 0 {
			t.Fatal("removed instance retained after stop attempt")
		}
	}
}

func TestSnapshotCopiesMemoryLimitAndNegativeUptime(t *testing.T) {
	now := time.Now().UTC()
	s, _, runtime, _, player, _ := testSampler(now)
	runtime.stats["container"] = docker.ResourceStats{MemoryBytes: 12, MemoryLimitBytes: 0}
	runtime.runtime["container"] = docker.RuntimeState{Running: true, StartedAt: now.Add(time.Minute)}
	player.err = errors.New("a2s unavailable")
	s.Sample(context.Background())
	got, _ := s.Latest("one")
	if got.MemoryPercent != nil || got.UptimeSeconds == nil || *got.UptimeSeconds != 0 {
		t.Fatalf("memory/uptime=%+v", got)
	}
	got.Issues[0].Source = "mutated"
	*got.MemoryBytes = 999
	*got.ContainerRunning = false
	fresh, _ := s.Latest("one")
	if *fresh.MemoryBytes == 999 || !*fresh.ContainerRunning || hasIssue(fresh.Issues, "mutated") {
		t.Fatalf("Latest aliases internal state: %+v", fresh)
	}
}

type workerRuntime struct {
	mu        sync.Mutex
	active    int
	maxActive int
	started   chan string
	release   chan struct{}
	blockedID string
	calls     int
}

func (w *workerRuntime) Runtime(ctx context.Context, id string) (docker.RuntimeState, error) {
	w.mu.Lock()
	w.active++
	w.calls++
	if w.active > w.maxActive {
		w.maxActive = w.active
	}
	w.mu.Unlock()
	defer func() { w.mu.Lock(); w.active--; w.mu.Unlock() }()
	w.started <- id
	if id == w.blockedID {
		select {
		case <-w.release:
		case <-ctx.Done():
			return docker.RuntimeState{}, ctx.Err()
		}
	}
	return docker.RuntimeState{Running: false}, nil
}

func TestWorkerPoolProcessesMoreJobsThanWorkers(t *testing.T) {
	items := make([]domain.Instance, 9)
	for i := range items {
		items[i] = domain.Instance{ID: string(rune('a' + i)), ContainerID: string(rune('a' + i))}
	}
	runtime := &workerRuntime{started: make(chan string, len(items)), release: make(chan struct{})}
	s := New(&fakeInstances{items: items}, runtime, &fakeTraffic{}, &fakePlayers{}, newFakeClock(time.Now()))
	s.workerCount = 3
	s.Sample(context.Background())
	runtime.mu.Lock()
	calls, maxActive := runtime.calls, runtime.maxActive
	runtime.mu.Unlock()
	if calls != len(items) || maxActive > 3 {
		t.Fatalf("calls=%d max active=%d", calls, maxActive)
	}
	for _, instance := range items {
		if _, ok := s.Latest(instance.ID); !ok {
			t.Fatalf("missing snapshot for %s", instance.ID)
		}
	}
}
func (*workerRuntime) Stats(context.Context, string) (docker.ResourceStats, error) {
	return docker.ResourceStats{}, nil
}

func TestSampleUsesBoundedWorkersAndCancellation(t *testing.T) {
	items := make([]domain.Instance, 9)
	for i := range items {
		items[i] = domain.Instance{ID: string(rune('a' + i)), ContainerID: string(rune('a' + i))}
	}
	runtime := &workerRuntime{started: make(chan string, len(items)), release: make(chan struct{}), blockedID: "a"}
	s := New(&fakeInstances{items: items}, runtime, &fakeTraffic{}, &fakePlayers{}, newFakeClock(time.Now()))
	s.workerCount = 2
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Sample(ctx); close(done) }()
	seenOther := false
	for !seenOther {
		select {
		case id := <-runtime.started:
			seenOther = id != "a"
		case <-time.After(time.Second):
			t.Fatal("blocked job prevented another worker")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Sample ignored cancellation")
	}
	runtime.mu.Lock()
	maxActive := runtime.maxActive
	runtime.mu.Unlock()
	if maxActive > 2 {
		t.Fatalf("max active=%d", maxActive)
	}
	if _, ok := s.Latest("b"); !ok {
		t.Fatal("other worker did not publish")
	}
}

func hasIssue(issues []Issue, source string) bool {
	for _, got := range issues {
		if got.Source == source {
			return true
		}
	}
	return false
}

func TestInstanceEnumerationDeadlinePreservesKnownCache(t *testing.T) {
	now := time.Now().UTC()
	s, instances, _, _, _, _ := testSampler(now)
	s.Sample(context.Background())
	instances.fn = func(ctx context.Context) ([]domain.Instance, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Error("Instances context has no deadline")
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	s.enumerationTimeout = 20 * time.Millisecond
	started := time.Now()
	s.Sample(context.Background())
	if time.Since(started) > 200*time.Millisecond {
		t.Fatal("enumeration timeout did not bound Sample")
	}
	if _, ok := s.Latest("one"); !ok || len(s.History("one")) == 0 {
		t.Fatal("failed enumeration pruned known cache")
	}
}

func TestRemovedInstanceStopHasDeadlineAndCannotBlockPruning(t *testing.T) {
	now := time.Now().UTC()
	s, instances, _, network, _, _ := testSampler(now)
	s.Sample(context.Background())
	instances.items = nil
	network.stopFn = func(ctx context.Context, _, _ string) error {
		<-ctx.Done()
		return ctx.Err()
	}
	s.cleanupTimeout = 20 * time.Millisecond
	started := time.Now()
	s.Sample(context.Background())
	if time.Since(started) > 200*time.Millisecond || !network.stopDeadlineSeen {
		t.Fatalf("cleanup duration=%v deadline=%v", time.Since(started), network.stopDeadlineSeen)
	}
	if _, ok := s.Latest("one"); ok || len(s.History("one")) != 0 {
		t.Fatal("timed out cleanup retained removed instance")
	}
}

func TestStartContinuesAfterEnumerationTimeout(t *testing.T) {
	now := time.Now().UTC()
	s, instances, _, _, _, clock := testSampler(now)
	var calls atomic.Int32
	instances.fn = func(ctx context.Context) ([]domain.Instance, error) {
		if calls.Add(1) == 1 {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return append([]domain.Instance(nil), instances.items...), nil
	}
	s.enumerationTimeout = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	deadline := time.After(time.Second)
	for calls.Load() < 1 {
		select {
		case <-deadline:
			t.Fatal("initial enumeration not called")
		default:
		}
	}
	clock.Advance(5 * time.Second)
	waitLatest(t, s, "one", now.Add(5*time.Second))
	cancel()
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

package metrics

import (
	"context"
	"errors"
	"sync"
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
}

func (f *fakeInstances) Instances(context.Context) ([]domain.Instance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]domain.Instance(nil), f.items...), f.err
}

type fakeRuntime struct {
	mu                   sync.Mutex
	runtime              map[string]docker.RuntimeState
	stats                map[string]docker.ResourceStats
	runtimeErr, statsErr error
}

func (f *fakeRuntime) Runtime(context.Context, string) (docker.RuntimeState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, v := range f.runtime {
		return v, f.runtimeErr
	}
	return docker.RuntimeState{}, f.runtimeErr
}
func (f *fakeRuntime) Stats(context.Context, string) (docker.ResourceStats, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, v := range f.stats {
		return v, f.statsErr
	}
	return docker.ResourceStats{}, f.statsErr
}

type fakeTraffic struct {
	mu         sync.Mutex
	totals     traffic.Totals
	err        error
	registered []traffic.Session
	stopped    []traffic.Session
}

func (f *fakeTraffic) Register(_ context.Context, s traffic.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.registered = append(f.registered, s)
	return f.err
}
func (f *fakeTraffic) Stop(_ context.Context, id, run string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = append(f.stopped, traffic.Session{InstanceID: id, RunID: run})
	return f.err
}
func (f *fakeTraffic) Totals(context.Context, string) (traffic.Totals, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.totals, f.err
}

type fakePlayers struct {
	summary players.Summary
	err     error
	clock   *fakeClock
	advance time.Duration
}

func (f *fakePlayers) Summary(context.Context, string) (players.Summary, error) {
	if f.clock != nil {
		f.clock.Advance(f.advance)
	}
	return f.summary, f.err
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
	if !second.ContainerRunning {
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
	if gap.ContainerRunning || gap.CPUPercent != nil || len(network.stopped) == 0 || network.stopped[len(network.stopped)-1].RunID != running.RunID {
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

package metrics

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/docker"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/players"
	"github.com/not0721here/l4d2-control-panel/internal/traffic"
)

const (
	SampleInterval         = 5 * time.Second
	MaxRateInterval        = 30 * time.Second
	historyLimit           = 720
	instanceTimeout        = 4 * time.Second
	maxConcurrentInstances = 4
)

type InstanceSource interface {
	Instances(context.Context) ([]domain.Instance, error)
}
type RuntimeProvider interface {
	Runtime(context.Context, string) (docker.RuntimeState, error)
	Stats(context.Context, string) (docker.ResourceStats, error)
}
type TrafficProvider interface {
	Register(context.Context, traffic.Session) error
	Stop(context.Context, string, string) error
	Totals(context.Context, string) (traffic.Totals, error)
}
type PlayerProvider interface {
	Summary(context.Context, string) (players.Summary, error)
}
type Ticker interface {
	C() <-chan time.Time
	Stop()
}
type Clock interface {
	Now() time.Time
	NewTicker(time.Duration) Ticker
}

type realClock struct{}

func (realClock) Now() time.Time                   { return time.Now() }
func (realClock) NewTicker(d time.Duration) Ticker { return realTicker{time.NewTicker(d)} }

type realTicker struct{ *time.Ticker }

func (t realTicker) C() <-chan time.Time { return t.Ticker.C }

type Issue struct {
	Source  string `json:"source"`
	Message string `json:"message"`
}

type Snapshot struct {
	Timestamp                time.Time `json:"timestamp"`
	RunID                    string    `json:"run_id"`
	ContainerRunning         *bool     `json:"container_running"`
	CPUPercent               *float64  `json:"cpu_percent"`
	MemoryBytes              *uint64   `json:"memory_bytes"`
	MemoryLimitBytes         *uint64   `json:"memory_limit_bytes"`
	MemoryPercent            *float64  `json:"memory_percent"`
	NetworkRXBytesPerSecond  *float64  `json:"network_rx_bytes_per_second"`
	NetworkTXBytesPerSecond  *float64  `json:"network_tx_bytes_per_second"`
	NetworkRXBytes           *uint64   `json:"network_rx_bytes"`
	NetworkTXBytes           *uint64   `json:"network_tx_bytes"`
	BlockReadBytesPerSecond  *float64  `json:"block_read_bytes_per_second"`
	BlockWriteBytesPerSecond *float64  `json:"block_write_bytes_per_second"`
	BlockReadBytes           *uint64   `json:"block_read_bytes"`
	BlockWriteBytes          *uint64   `json:"block_write_bytes"`
	PIDs                     *uint64   `json:"pids"`
	UptimeSeconds            *uint64   `json:"uptime_seconds"`
	A2SLatencyMS             *float64  `json:"a2s_latency_ms"`
	Map                      *string   `json:"map"`
	Players                  *int      `json:"players"`
	MaxPlayers               *int      `json:"max_players"`
	Issues                   []Issue   `json:"issues,omitempty"`
}

type counterSample struct {
	timestamp           time.Time
	runID               string
	rx, tx, read, write *uint64
}

type Sampler struct {
	instances InstanceSource
	runtime   RuntimeProvider
	traffic   TrafficProvider
	players   PlayerProvider
	clock     Clock

	mu                 sync.RWMutex
	latest             map[string]Snapshot
	history            map[string][]Snapshot
	previous           map[string]counterSample
	runs               map[string]string
	workerCount        int
	instanceTimeout    time.Duration
	enumerationTimeout time.Duration
	cleanupTimeout     time.Duration
	startOnce          sync.Once
	stopOnce           sync.Once
	cancel             context.CancelFunc
	done               chan struct{}
}

func New(instances InstanceSource, runtime RuntimeProvider, trafficProvider TrafficProvider, playerProvider PlayerProvider, clock Clock) *Sampler {
	if clock == nil {
		clock = realClock{}
	}
	return &Sampler{instances: instances, runtime: runtime, traffic: trafficProvider, players: playerProvider, clock: clock, latest: map[string]Snapshot{}, history: map[string][]Snapshot{}, previous: map[string]counterSample{}, runs: map[string]string{}, workerCount: maxConcurrentInstances, instanceTimeout: instanceTimeout, enumerationTimeout: instanceTimeout, cleanupTimeout: instanceTimeout, done: make(chan struct{})}
}

func (s *Sampler) Start(ctx context.Context) {
	s.startOnce.Do(func() {
		var runCtx context.Context
		runCtx, s.cancel = context.WithCancel(ctx)
		go func() {
			defer close(s.done)
			ticker := s.clock.NewTicker(SampleInterval)
			defer ticker.Stop()
			s.Sample(runCtx)
			for {
				select {
				case <-runCtx.Done():
					return
				case _, ok := <-ticker.C():
					if !ok {
						return
					}
					s.Sample(runCtx)
				}
			}
		}()
	})
}

func (s *Sampler) Stop(ctx context.Context) error {
	s.stopOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
	})
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Sampler) Sample(ctx context.Context) {
	enumerationCtx, cancelEnumeration := context.WithTimeout(ctx, s.enumerationTimeout)
	instances, err := s.instances.Instances(enumerationCtx)
	cancelEnumeration()
	if err != nil {
		return
	}
	present := make(map[string]struct{}, len(instances))
	for _, instance := range instances {
		present[instance.ID] = struct{}{}
	}
	s.prune(ctx, present)
	workers := s.workerCount
	if workers < 1 {
		workers = 1
	}
	jobs := make(chan domain.Instance, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for instance := range jobs {
				instanceCtx, cancel := context.WithTimeout(ctx, s.instanceTimeout)
				s.sampleInstance(instanceCtx, instance)
				cancel()
			}
		}()
	}
	for _, instance := range instances {
		select {
		case jobs <- instance:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return
		}
	}
	close(jobs)
	wg.Wait()
}

func (s *Sampler) sampleInstance(ctx context.Context, instance domain.Instance) {
	now := s.clock.Now().UTC()
	snapshot := Snapshot{Timestamp: now}
	priorRun := s.priorRun(instance.ID)
	runtimeResults := make(chan runtimeResult, 1)
	statsResults := make(chan sourceResult, 1)
	playerResults := make(chan Snapshot, 1)
	trafficResults := make(chan sourceResult, 1)
	go func() {
		state, err := s.runtime.Runtime(ctx, instance.ContainerID)
		runtimeResults <- runtimeResult{state: state, err: err}
	}()
	go func() {
		partial := Snapshot{}
		counters := counterSample{timestamp: now, runID: priorRun}
		s.sampleStats(ctx, instance.ContainerID, &partial, &counters)
		statsResults <- sourceResult{snapshot: partial, counters: counters}
	}()
	go func() {
		partial := Snapshot{}
		s.samplePlayers(ctx, instance.ID, &partial)
		playerResults <- partial
	}()
	go func() {
		partial := Snapshot{}
		counters := counterSample{timestamp: now, runID: priorRun}
		if priorRun != "" {
			s.sampleTraffic(ctx, instance.ID, priorRun, &partial, &counters)
		}
		trafficResults <- sourceResult{snapshot: partial, counters: counters}
	}()

	runtimeResult := <-runtimeResults
	statsResult := <-statsResults
	playerResult := <-playerResults
	priorTrafficResult := <-trafficResults
	err := runtimeResult.err
	if err != nil {
		snapshot.Issues = append(snapshot.Issues, issue("runtime", err))
		snapshot.RunID = priorRun
		mergeSnapshot(&snapshot, statsResult.snapshot)
		mergeSnapshot(&snapshot, priorTrafficResult.snapshot)
		mergeSnapshot(&snapshot, playerResult)
		s.publish(instance.ID, snapshot, nil)
		return
	}
	runtimeState := runtimeResult.state
	if !runtimeState.Running {
		snapshot.ContainerRunning = boolptr(false)
		s.stopPrevious(ctx, instance.ID)
		s.publish(instance.ID, snapshot, &counterSample{})
		return
	}
	snapshot.ContainerRunning = boolptr(true)
	snapshot.RunID = runtimeState.StartedAt.UTC().Format(time.RFC3339Nano)
	uptime := now.Sub(runtimeState.StartedAt.UTC())
	if uptime < 0 {
		uptime = 0
	}
	uptimeSeconds := uint64(uptime / time.Second)
	snapshot.UptimeSeconds = &uptimeSeconds

	ports := append([]int{instance.GamePort, instance.SourceTVPort}, instance.PluginPorts...)
	ports = uniquePositivePorts(ports)
	if err := s.traffic.Register(ctx, traffic.Session{InstanceID: instance.ID, RunID: snapshot.RunID, Ports: ports}); err != nil {
		snapshot.Issues = append(snapshot.Issues, issue("traffic_register", err))
	}

	counters := counterSample{timestamp: now, runID: snapshot.RunID}
	mergeSnapshot(&snapshot, statsResult.snapshot)
	counters.read = statsResult.counters.read
	counters.write = statsResult.counters.write
	s.sampleTraffic(ctx, instance.ID, snapshot.RunID, &snapshot, &counters)
	mergeSnapshot(&snapshot, playerResult)
	s.publish(instance.ID, snapshot, &counters)
}

type runtimeResult struct {
	state docker.RuntimeState
	err   error
}

type sourceResult struct {
	snapshot Snapshot
	counters counterSample
}

func mergeSnapshot(target *Snapshot, source Snapshot) {
	if source.CPUPercent != nil {
		target.CPUPercent = source.CPUPercent
	}
	if source.MemoryBytes != nil {
		target.MemoryBytes = source.MemoryBytes
	}
	if source.MemoryLimitBytes != nil {
		target.MemoryLimitBytes = source.MemoryLimitBytes
	}
	if source.MemoryPercent != nil {
		target.MemoryPercent = source.MemoryPercent
	}
	if source.NetworkRXBytes != nil {
		target.NetworkRXBytes = source.NetworkRXBytes
	}
	if source.NetworkTXBytes != nil {
		target.NetworkTXBytes = source.NetworkTXBytes
	}
	if source.BlockReadBytes != nil {
		target.BlockReadBytes = source.BlockReadBytes
	}
	if source.BlockWriteBytes != nil {
		target.BlockWriteBytes = source.BlockWriteBytes
	}
	if source.PIDs != nil {
		target.PIDs = source.PIDs
	}
	if source.A2SLatencyMS != nil {
		target.A2SLatencyMS = source.A2SLatencyMS
	}
	if source.Map != nil {
		target.Map = source.Map
	}
	if source.Players != nil {
		target.Players = source.Players
	}
	if source.MaxPlayers != nil {
		target.MaxPlayers = source.MaxPlayers
	}
	target.Issues = append(target.Issues, source.Issues...)
}

func (s *Sampler) sampleStats(ctx context.Context, containerID string, snapshot *Snapshot, counters *counterSample) {
	if stats, err := s.runtime.Stats(ctx, containerID); err != nil {
		snapshot.Issues = append(snapshot.Issues, issue("docker_stats", err))
	} else {
		snapshot.CPUPercent = float64ptr(stats.CPUPercent)
		snapshot.MemoryBytes = uint64ptr(stats.MemoryBytes)
		snapshot.MemoryLimitBytes = uint64ptr(stats.MemoryLimitBytes)
		if stats.MemoryLimitBytes > 0 {
			snapshot.MemoryPercent = float64ptr(float64(stats.MemoryBytes) / float64(stats.MemoryLimitBytes) * 100)
		}
		snapshot.BlockReadBytes = uint64ptr(stats.BlockReadBytes)
		snapshot.BlockWriteBytes = uint64ptr(stats.BlockWriteBytes)
		snapshot.PIDs = uint64ptr(stats.PIDs)
		counters.read = uint64ptr(stats.BlockReadBytes)
		counters.write = uint64ptr(stats.BlockWriteBytes)
	}
}

func (s *Sampler) sampleTraffic(ctx context.Context, instanceID, runID string, snapshot *Snapshot, counters *counterSample) {
	if totals, err := s.traffic.Totals(ctx, instanceID); err != nil {
		snapshot.Issues = append(snapshot.Issues, issue("traffic_totals", err))
	} else if totals.RunID == "" || totals.RunID == runID {
		snapshot.NetworkRXBytes = uint64ptr(totals.RXBytes)
		snapshot.NetworkTXBytes = uint64ptr(totals.TXBytes)
		counters.rx = uint64ptr(totals.RXBytes)
		counters.tx = uint64ptr(totals.TXBytes)
	} else {
		snapshot.Issues = append(snapshot.Issues, Issue{Source: "traffic_totals", Message: "run ID mismatch"})
	}
}

func (s *Sampler) samplePlayers(ctx context.Context, id string, snapshot *Snapshot) {
	started := s.clock.Now()
	if summary, err := s.players.Summary(ctx, id); err != nil {
		snapshot.Issues = append(snapshot.Issues, issue("a2s", err))
	} else {
		latency := s.clock.Now().Sub(started).Seconds() * 1000
		snapshot.A2SLatencyMS = &latency
		snapshot.Map = stringptr(summary.Map)
		snapshot.Players = intptr(summary.Players)
		snapshot.MaxPlayers = intptr(summary.MaxPlayers)
	}
}

func (s *Sampler) publish(id string, snapshot Snapshot, counters *counterSample) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if previous, ok := s.previous[id]; ok && counters != nil && counters.runID != "" {
		elapsed := counters.timestamp.Sub(previous.timestamp)
		if previous.runID == counters.runID && elapsed > 0 && elapsed <= MaxRateInterval {
			seconds := elapsed.Seconds()
			snapshot.NetworkRXBytesPerSecond = rate(previous.rx, counters.rx, seconds)
			snapshot.NetworkTXBytesPerSecond = rate(previous.tx, counters.tx, seconds)
			snapshot.BlockReadBytesPerSecond = rate(previous.read, counters.read, seconds)
			snapshot.BlockWriteBytesPerSecond = rate(previous.write, counters.write, seconds)
		}
	}
	if counters != nil {
		if counters.runID != "" {
			s.previous[id] = cloneCounters(*counters)
			s.runs[id] = counters.runID
		} else {
			delete(s.previous, id)
			delete(s.runs, id)
		}
	}
	copySnapshot := cloneSnapshot(snapshot)
	s.latest[id] = copySnapshot
	history := append(s.history[id], copySnapshot)
	if len(history) > historyLimit {
		history = append([]Snapshot(nil), history[len(history)-historyLimit:]...)
	}
	s.history[id] = history
}

func (s *Sampler) stopPrevious(ctx context.Context, id string) {
	s.mu.RLock()
	runID := s.runs[id]
	s.mu.RUnlock()
	if runID != "" {
		_ = s.traffic.Stop(ctx, id, runID)
	}
}

func (s *Sampler) priorRun(id string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.runs[id]
}

func (s *Sampler) prune(ctx context.Context, present map[string]struct{}) {
	s.mu.RLock()
	removed := make(map[string]string)
	for id := range s.latest {
		if _, ok := present[id]; !ok {
			removed[id] = s.runs[id]
		}
	}
	s.mu.RUnlock()
	cleanupCtx, cancelCleanup := context.WithTimeout(ctx, s.cleanupTimeout)
	defer cancelCleanup()
	for id, runID := range removed {
		if runID != "" {
			stopCtx, cancelStop := context.WithTimeout(cleanupCtx, s.cleanupTimeout)
			err := s.traffic.Stop(stopCtx, id, runID)
			cancelStop()
			if err != nil {
				log.Printf("metrics: stop removed instance %s traffic session: %v", id, err)
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range removed {
		delete(s.latest, id)
		delete(s.history, id)
		delete(s.previous, id)
		delete(s.runs, id)
	}
}

func (s *Sampler) Latest(id string) (Snapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.latest[id]
	return cloneSnapshot(v), ok
}
func (s *Sampler) History(id string) []Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Snapshot, len(s.history[id]))
	for i := range result {
		result[i] = cloneSnapshot(s.history[id][i])
	}
	return result
}

func issue(source string, err error) Issue { return Issue{Source: source, Message: err.Error()} }
func uint64ptr(v uint64) *uint64           { return &v }
func float64ptr(v float64) *float64        { return &v }
func boolptr(v bool) *bool                 { return &v }
func intptr(v int) *int                    { return &v }
func stringptr(v string) *string           { return &v }
func rate(old, new *uint64, seconds float64) *float64 {
	if old == nil || new == nil || *new < *old {
		return nil
	}
	return float64ptr(float64(*new-*old) / seconds)
}
func cloneCounters(v counterSample) counterSample {
	v.rx = cloneUint64(v.rx)
	v.tx = cloneUint64(v.tx)
	v.read = cloneUint64(v.read)
	v.write = cloneUint64(v.write)
	return v
}
func cloneUint64(v *uint64) *uint64 {
	if v == nil {
		return nil
	}
	return uint64ptr(*v)
}
func cloneFloat64(v *float64) *float64 {
	if v == nil {
		return nil
	}
	return float64ptr(*v)
}
func cloneBool(v *bool) *bool {
	if v == nil {
		return nil
	}
	return boolptr(*v)
}
func cloneInt(v *int) *int {
	if v == nil {
		return nil
	}
	return intptr(*v)
}
func cloneString(v *string) *string {
	if v == nil {
		return nil
	}
	return stringptr(*v)
}
func cloneSnapshot(v Snapshot) Snapshot {
	v.ContainerRunning = cloneBool(v.ContainerRunning)
	v.CPUPercent = cloneFloat64(v.CPUPercent)
	v.MemoryBytes = cloneUint64(v.MemoryBytes)
	v.MemoryLimitBytes = cloneUint64(v.MemoryLimitBytes)
	v.MemoryPercent = cloneFloat64(v.MemoryPercent)
	v.NetworkRXBytesPerSecond = cloneFloat64(v.NetworkRXBytesPerSecond)
	v.NetworkTXBytesPerSecond = cloneFloat64(v.NetworkTXBytesPerSecond)
	v.NetworkRXBytes = cloneUint64(v.NetworkRXBytes)
	v.NetworkTXBytes = cloneUint64(v.NetworkTXBytes)
	v.BlockReadBytesPerSecond = cloneFloat64(v.BlockReadBytesPerSecond)
	v.BlockWriteBytesPerSecond = cloneFloat64(v.BlockWriteBytesPerSecond)
	v.BlockReadBytes = cloneUint64(v.BlockReadBytes)
	v.BlockWriteBytes = cloneUint64(v.BlockWriteBytes)
	v.PIDs = cloneUint64(v.PIDs)
	v.UptimeSeconds = cloneUint64(v.UptimeSeconds)
	v.A2SLatencyMS = cloneFloat64(v.A2SLatencyMS)
	v.Map = cloneString(v.Map)
	v.Players = cloneInt(v.Players)
	v.MaxPlayers = cloneInt(v.MaxPlayers)
	v.Issues = append([]Issue(nil), v.Issues...)
	return v
}
func uniquePositivePorts(ports []int) []int {
	seen := map[int]struct{}{}
	result := make([]int, 0, len(ports))
	for _, port := range ports {
		if port <= 0 {
			continue
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		result = append(result, port)
	}
	sort.Ints(result)
	return result
}

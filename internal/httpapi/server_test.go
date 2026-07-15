package httpapi

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/not0721here/l4d2-control-panel/internal/auth"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/docker"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
	"github.com/not0721here/l4d2-control-panel/internal/metrics"
	"github.com/not0721here/l4d2-control-panel/internal/players"
	"github.com/not0721here/l4d2-control-panel/internal/scheduler"
	"github.com/not0721here/l4d2-control-panel/internal/store"
	"github.com/not0721here/l4d2-control-panel/internal/updates"
)

type fakeLifecycle struct{ action string }

func (f *fakeLifecycle) Start(context.Context, string) error        { f.action = "start"; return nil }
func (f *fakeLifecycle) Stop(context.Context, string) error         { f.action = "stop"; return nil }
func (f *fakeLifecycle) Restart(context.Context, string) error      { f.action = "restart"; return nil }
func (f *fakeLifecycle) Rebuild(context.Context, string) error      { f.action = "rebuild"; return nil }
func (f *fakeLifecycle) Delete(context.Context, string, bool) error { f.action = "delete"; return nil }

type fakeAttacher struct{ peer net.Conn }

type overviewPlayers struct {
	summary players.Summary
	calls   int
}

func (p *overviewPlayers) Summary(context.Context, string) (players.Summary, error) {
	p.calls++
	return p.summary, nil
}
func (*overviewPlayers) Online(context.Context, string) (players.Snapshot, error) {
	return players.Snapshot{}, nil
}
func (*overviewPlayers) Kick(context.Context, string, int) error     { return nil }
func (*overviewPlayers) Ban(context.Context, string, int, int) error { return nil }

type overviewResources struct {
	running      bool
	stats        docker.ResourceStats
	runningCalls int
	statsCalls   int
}

func (r *overviewResources) Running(context.Context, string) (bool, error) {
	r.runningCalls++
	return r.running, nil
}
func (r *overviewResources) Stats(context.Context, string) (docker.ResourceStats, error) {
	r.statsCalls++
	return r.stats, nil
}

type overviewPerformance struct {
	latest  metrics.Snapshot
	found   bool
	history []metrics.Snapshot
}

func (p *overviewPerformance) Latest(string) (metrics.Snapshot, bool) { return p.latest, p.found }
func (p *overviewPerformance) History(string) []metrics.Snapshot      { return p.history }

type fakeScheduleDispatcher struct{}

func (fakeScheduleDispatcher) Dispatch(context.Context, domain.ScheduledTask) error { return nil }

func (f *fakeAttacher) AttachSupervisor(context.Context, string) (io.ReadWriteCloser, error) {
	client, peer := net.Pipe()
	f.peer = peer
	return client, nil
}

func testServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	a := auth.NewService()
	if err := a.Bootstrap("correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	packages, err := content.NewPackageManager(root)
	if err != nil {
		t.Fatal(err)
	}
	addTestPackage(t, packages, "default.zip", "default")
	pipeline := updates.New(root)
	return New(db, a, WithContent(nil, nil, packages, pipeline, nil)), db
}

func addTestPackage(t *testing.T, manager *content.PackageManager, name, version string) string {
	t.Helper()
	var raw bytes.Buffer
	writer := zip.NewWriter(&raw)
	file, err := writer.Create("cfg/plugin.cfg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("sm_cvar fixture 1")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	item, err := manager.AddUpload(name, version, bytes.NewReader(raw.Bytes()), int64(raw.Len()))
	if err != nil {
		t.Fatal(err)
	}
	return item.ID
}

func defaultPackageID(t *testing.T, server *Server) string {
	t.Helper()
	items, err := server.packages.List()
	if err != nil || len(items) == 0 {
		t.Fatalf("packages=%#v err=%v", items, err)
	}
	return items[0].ID
}

func authenticatedJSON(t *testing.T, server *Server, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func waitForJob(t *testing.T, manager *jobs.Manager, id string) jobs.Job {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, ok := manager.Get(id)
		if ok && (job.Status == jobs.Succeeded || job.Status == jobs.Failed) {
			return job
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("job did not finish")
	return jobs.Job{}
}
func loginCookie(t *testing.T, s *Server) *http.Cookie {
	t.Helper()
	body := bytes.NewBufferString(`{"password":"correct horse battery staple"}`)
	r := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	return w.Result().Cookies()[0]
}

func TestProtectedRoutesRequireSession(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/instances", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestLoginCookieSecurityCanBeDisabled(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	s = New(db, s.auth, WithSecureCookie(false))
	cookie := loginCookie(t, s)
	if cookie.Secure {
		t.Fatal("HTTP deployments must be able to disable the Secure cookie attribute")
	}
}

func TestCreateAndListInstance(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	cookie := loginCookie(t, s)
	payload := map[string]any{"name": "Coop One", "game_port": 27015, "start_map": "c2m1_highway", "game_mode": "coop", "tickrate": 100, "max_players": 8, "package_id": defaultPackageID(t, s)}
	raw, _ := json.Marshal(payload)
	r := httptest.NewRequest(http.MethodPost, "/api/instances", bytes.NewReader(raw))
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	r = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/instances", nil)
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte("Coop One")) {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	events, err := db.AuditEvents(context.Background(), 10)
	if err != nil || len(events) != 1 || events[0].Target != "/api/instances" || events[0].Result != "201" {
		t.Fatalf("audit=%#v err=%v", events, err)
	}
}

func TestInstanceOverviewUsesSamplerObservations(t *testing.T) {
	t.Run("running", func(t *testing.T) {
		s, db := testServer(t)
		defer db.Close()
		instance := domain.Instance{ID: "live", NodeID: "local", Name: "Live", ContainerID: "container-live", GamePort: 27015, StartMap: "configured", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", DesiredState: domain.StateRunning, ActualState: domain.StateStopped}
		if err := db.CreateInstance(context.Background(), instance); err != nil {
			t.Fatal(err)
		}
		playerSource := &overviewPlayers{}
		resourceSource := &overviewResources{}
		running := true
		zeroFloat := 0.0
		zeroUint := uint64(0)
		playersOnline := 0
		maxPlayers := 8
		gameMap := "c5m1_waterfront"
		sampledAt := time.Date(2026, 7, 15, 12, 30, 45, 123456789, time.FixedZone("fixture", 8*60*60))
		performance := &overviewPerformance{found: true, latest: metrics.Snapshot{
			Timestamp: sampledAt, RunID: "run-7", ContainerRunning: &running,
			CPUPercent: &zeroFloat, MemoryBytes: &zeroUint, MemoryLimitBytes: uint64TestPointer(2 << 30), MemoryPercent: float64TestPointer(0),
			NetworkRXBytesPerSecond: float64TestPointer(12.5), NetworkTXBytesPerSecond: &zeroFloat, NetworkRXBytes: uint64TestPointer(100), NetworkTXBytes: &zeroUint,
			BlockReadBytesPerSecond: &zeroFloat, BlockWriteBytesPerSecond: float64TestPointer(1.5), BlockReadBytes: &zeroUint, BlockWriteBytes: uint64TestPointer(200),
			PIDs: &zeroUint, UptimeSeconds: uint64TestPointer(90), A2SLatencyMS: &zeroFloat, Map: &gameMap, Players: &playersOnline, MaxPlayers: &maxPlayers,
			Issues: []metrics.Issue{{Source: "traffic_totals", Message: "temporarily unavailable"}},
		}}
		s = New(db, s.auth, WithPlayers(playerSource), WithResources(resourceSource), WithPerformance(performance))

		response := authenticatedJSON(t, s, loginCookie(t, s), http.MethodGet, "/api/instances/live/overview", "")
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["actual_state"] != string(domain.StateRunning) || body["container_running"] != true || body["container_running_known"] != true || body["sampled_at"] != sampledAt.UTC().Format(time.RFC3339Nano) || body["run_id"] != "run-7" || body["map"] != "c5m1_waterfront" || body["players"] != float64(0) || body["max_players"] != float64(8) || body["cpu_percent"] != float64(0) || body["memory_bytes"] != float64(0) || body["memory_limit_bytes"] != float64(2<<30) || body["memory_percent"] != float64(0) || body["network_rx_bytes_per_sec"] != 12.5 || body["network_tx_bytes_per_sec"] != float64(0) || body["network_rx_bytes"] != float64(100) || body["network_tx_bytes"] != float64(0) || body["block_read_bytes_per_sec"] != float64(0) || body["block_write_bytes_per_sec"] != 1.5 || body["block_read_bytes"] != float64(0) || body["block_write_bytes"] != float64(200) || body["pids"] != float64(0) || body["uptime_seconds"] != float64(90) || body["a2s_latency_ms"] != float64(0) {
			t.Fatalf("overview=%s", response.Body.String())
		}
		if _, ok := body["network_rx_bytes_per_second"]; ok {
			t.Fatalf("legacy sampler field name leaked: %s", response.Body.String())
		}
		if playerSource.calls != 0 || resourceSource.runningCalls != 0 || resourceSource.statsCalls != 0 {
			t.Fatalf("overview performed live fan-out: playerCalls=%d runningCalls=%d statsCalls=%d", playerSource.calls, resourceSource.runningCalls, resourceSource.statsCalls)
		}
		if got := performance.latest.Issues[0].Message; got != "temporarily unavailable" {
			t.Fatalf("provider snapshot mutated: %q", got)
		}
	})

	t.Run("running container with a2s issue is unhealthy", func(t *testing.T) {
		s, db := testServer(t)
		defer db.Close()
		instance := domain.Instance{ID: "unhealthy", NodeID: "local", Name: "Unhealthy", ContainerID: "container-unhealthy", GamePort: 27021, StartMap: "configured", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", DesiredState: domain.StateRunning, ActualState: domain.StateRunning}
		if err := db.CreateInstance(context.Background(), instance); err != nil {
			t.Fatal(err)
		}
		running := true
		performance := &overviewPerformance{found: true, latest: metrics.Snapshot{Timestamp: time.Now(), ContainerRunning: &running, Issues: []metrics.Issue{{Source: "a2s", Message: "query timed out"}}}}
		s = New(db, s.auth, WithPerformance(performance))
		response := authenticatedJSON(t, s, loginCookie(t, s), http.MethodGet, "/api/instances/unhealthy/overview", "")
		var body struct {
			ActualState domain.InstanceState `json:"actual_state"`
			Issues      []string             `json:"issues"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK || body.ActualState != domain.StateFaulted || len(body.Issues) != 1 || body.Issues[0] != "a2s: query timed out" {
			t.Fatalf("overview=%s", response.Body.String())
		}
	})

	t.Run("stopped", func(t *testing.T) {
		s, db := testServer(t)
		defer db.Close()
		instance := domain.Instance{ID: "stale", NodeID: "local", Name: "Stale", ContainerID: "container-stale", GamePort: 27016, StartMap: "configured", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", DesiredState: domain.StateRunning, ActualState: domain.StateRunning}
		if err := db.CreateInstance(context.Background(), instance); err != nil {
			t.Fatal(err)
		}
		stopped := false
		performance := &overviewPerformance{found: true, latest: metrics.Snapshot{Timestamp: time.Date(2026, 7, 15, 1, 2, 3, 0, time.UTC), ContainerRunning: &stopped}}
		s = New(db, s.auth, WithPerformance(performance))

		response := authenticatedJSON(t, s, loginCookie(t, s), http.MethodGet, "/api/instances/stale/overview", "")
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		var body struct {
			ActualState      domain.InstanceState `json:"actual_state"`
			ContainerRunning bool                 `json:"container_running"`
			Players          *int                 `json:"players"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.ActualState != domain.StateStopped || body.ContainerRunning || body.Players != nil {
			t.Fatalf("overview=%s", response.Body.String())
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
			t.Fatal(err)
		}
		if _, exists := fields["map"]; exists {
			t.Fatalf("unavailable map must be omitted: %s", response.Body.String())
		}
	})

	t.Run("valid empty map follows legacy omission", func(t *testing.T) {
		s, db := testServer(t)
		defer db.Close()
		instance := domain.Instance{ID: "empty-map", NodeID: "local", Name: "Empty Map", ContainerID: "container-empty-map", GamePort: 27022, StartMap: "configured", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", DesiredState: domain.StateRunning, ActualState: domain.StateRunning}
		if err := db.CreateInstance(context.Background(), instance); err != nil {
			t.Fatal(err)
		}
		running := true
		gameMap := ""
		playersOnline, maxPlayers := 0, 8
		performance := &overviewPerformance{found: true, latest: metrics.Snapshot{Timestamp: time.Now(), ContainerRunning: &running, Map: &gameMap, Players: &playersOnline, MaxPlayers: &maxPlayers}}
		s = New(db, s.auth, WithPerformance(performance))
		response := authenticatedJSON(t, s, loginCookie(t, s), http.MethodGet, "/api/instances/empty-map/overview", "")
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		if _, exists := fields["map"]; exists {
			t.Fatalf("empty map must follow legacy omission: %s", response.Body.String())
		}
		if string(fields["players"]) != "0" || string(fields["cpu_percent"]) != "null" {
			t.Fatalf("other null and zero semantics changed: %s", response.Body.String())
		}
	})

	t.Run("runtime unknown preserves state and reports issues", func(t *testing.T) {
		s, db := testServer(t)
		defer db.Close()
		instance := domain.Instance{ID: "unknown", NodeID: "local", Name: "Unknown", ContainerID: "container-unknown", GamePort: 27018, StartMap: "configured", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", DesiredState: domain.StateRunning, ActualState: domain.StateStarting}
		if err := db.CreateInstance(context.Background(), instance); err != nil {
			t.Fatal(err)
		}
		performance := &overviewPerformance{found: true, latest: metrics.Snapshot{Timestamp: time.Now(), Issues: []metrics.Issue{{Source: "runtime", Message: "timeout"}}}}
		s = New(db, s.auth, WithPerformance(performance))
		response := authenticatedJSON(t, s, loginCookie(t, s), http.MethodGet, "/api/instances/unknown/overview", "")
		var body struct {
			ActualState           domain.InstanceState `json:"actual_state"`
			ContainerRunning      bool                 `json:"container_running"`
			ContainerRunningKnown bool                 `json:"container_running_known"`
			Issues                []string             `json:"issues"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK || body.ActualState != domain.StateStarting || body.ContainerRunning || body.ContainerRunningKnown || len(body.Issues) != 1 || body.Issues[0] != "runtime: timeout" {
			t.Fatalf("overview=%s", response.Body.String())
		}
	})

	t.Run("missing container", func(t *testing.T) {
		s, db := testServer(t)
		defer db.Close()
		instance := domain.Instance{ID: "missing", NodeID: "local", Name: "Missing", GamePort: 27017, StartMap: "configured", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", DesiredState: domain.StateRunning, ActualState: domain.StateRunning}
		if err := db.CreateInstance(context.Background(), instance); err != nil {
			t.Fatal(err)
		}
		s = New(db, s.auth, WithPerformance(&overviewPerformance{}))

		response := authenticatedJSON(t, s, loginCookie(t, s), http.MethodGet, "/api/instances/missing/overview", "")
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		var body struct {
			ActualState domain.InstanceState `json:"actual_state"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.ActualState != domain.StateOrphaned {
			t.Fatalf("overview=%s", response.Body.String())
		}
	})

	t.Run("missing provider is unavailable", func(t *testing.T) {
		s, db := testServer(t)
		defer db.Close()
		instance := domain.Instance{ID: "unwired", NodeID: "local", Name: "Unwired", ContainerID: "container", GamePort: 27019, StartMap: "configured", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", ActualState: domain.StateRunning}
		if err := db.CreateInstance(context.Background(), instance); err != nil {
			t.Fatal(err)
		}
		response := authenticatedJSON(t, s, loginCookie(t, s), http.MethodGet, "/api/instances/unwired/overview", "")
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})
}

func TestInstancePerformanceHistory(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	instance := domain.Instance{ID: "history", NodeID: "local", Name: "History", ContainerID: "container-history", GamePort: 27020, StartMap: "configured", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", ActualState: domain.StateRunning}
	if err := db.CreateInstance(context.Background(), instance); err != nil {
		t.Fatal(err)
	}

	unauthorized := httptest.NewRecorder()
	s.Handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/instances/history/performance-history", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d", unauthorized.Code)
	}

	provider := &overviewPerformance{history: []metrics.Snapshot{}}
	s = New(db, s.auth, WithPerformance(provider))
	cookie := loginCookie(t, s)
	empty := authenticatedJSON(t, s, cookie, http.MethodGet, "/api/instances/history/performance-history", "")
	if empty.Code != http.StatusOK || strings.TrimSpace(empty.Body.String()) != "[]" {
		t.Fatalf("empty status=%d body=%s", empty.Code, empty.Body.String())
	}
	missing := authenticatedJSON(t, s, cookie, http.MethodGet, "/api/instances/missing/performance-history", "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d body=%s", missing.Code, missing.Body.String())
	}

	zero := 0.0
	start := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	provider.history = []metrics.Snapshot{
		{Timestamp: start.Add(721 * time.Second), RunID: "run-721", CPUPercent: &zero, NetworkRXBytesPerSecond: &zero},
		{Timestamp: start.Add(500 * time.Second), RunID: "equal-first", CPUPercent: &zero},
		{Timestamp: start, RunID: "run-0", CPUPercent: &zero},
	}
	for i := 1; i <= 720; i++ {
		provider.history = append(provider.history, metrics.Snapshot{Timestamp: start.Add(time.Duration(i) * time.Second), RunID: fmt.Sprintf("run-%d", i), CPUPercent: &zero})
	}
	provider.history[0].MemoryPercent = nil
	provider.history[0].Issues = []metrics.Issue{{Source: "secret", Message: "must not leak"}}
	providerBefore, err := json.Marshal(provider.history)
	if err != nil {
		t.Fatal(err)
	}
	response := authenticatedJSON(t, s, cookie, http.MethodGet, "/api/instances/history/performance-history", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var points []map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &points); err != nil {
		t.Fatal(err)
	}
	if len(points) != 720 || points[0]["at"] != start.Add(3*time.Second).Format(time.RFC3339) || points[0]["run_id"] != "run-3" || points[497]["run_id"] != "equal-first" || points[498]["run_id"] != "run-500" || points[719]["at"] != start.Add(721*time.Second).Format(time.RFC3339) || points[719]["run_id"] != "run-721" || points[719]["cpu_percent"] != float64(0) || points[719]["network_rx_bytes_per_sec"] != float64(0) || points[719]["memory_percent"] != nil {
		t.Fatalf("history len=%d first=%v last=%v", len(points), points[0], points[len(points)-1])
	}
	for _, forbidden := range []string{"network_rx_bytes", "network_tx_bytes", "block_read_bytes", "block_write_bytes", "issues", "error", "errors", "map", "players", "container_running", "address", "packet"} {
		if _, ok := points[719][forbidden]; ok {
			t.Fatalf("history leaked %s: %s", forbidden, response.Body.String())
		}
	}
	providerAfter, err := json.Marshal(provider.history)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(providerAfter, providerBefore) {
		t.Fatal("provider history order or content mutated")
	}
}

func uint64TestPointer(value uint64) *uint64    { return &value }
func float64TestPointer(value float64) *float64 { return &value }

func TestCreateRejectsInvalidPort(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	cookie := loginCookie(t, s)
	r := httptest.NewRequest(http.MethodPost, "/api/instances", bytes.NewBufferString(`{"name":"Bad","game_port":80}`))
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateAndUpdateExposeSourceTVAndPluginPorts(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	cookie := loginCookie(t, s)
	packageID := defaultPackageID(t, s)
	create := fmt.Sprintf(`{"name":"Ports","game_port":27015,"sourcetv_port":27020,"plugin_ports":[27021,27022],"start_map":"c2m1_highway","game_mode":"coop","tickrate":100,"max_players":8,"package_id":%q}`, packageID)
	r := httptest.NewRequest(http.MethodPost, "/api/instances", bytes.NewBufferString(create))
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"sourcetv_port":27020`) || !strings.Contains(w.Body.String(), `"plugin_ports":[27021,27022]`) {
		t.Fatalf("create: status=%d body=%s", w.Code, w.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id, _ := created["ID"].(string)
	update := fmt.Sprintf(`{"name":"Ports","game_port":27015,"sourcetv_port":27030,"plugin_ports":[27031],"start_map":"c2m1_highway","game_mode":"coop","tickrate":100,"max_players":8,"extra_args":"","package_id":%q}`, packageID)
	r = httptest.NewRequest(http.MethodPut, "/api/instances/"+id, bytes.NewBufferString(update))
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"sourcetv_port":27030`) || !strings.Contains(w.Body.String(), `"plugin_ports":[27031]`) {
		t.Fatalf("update: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreatePersistsPackageAndExtraArgs(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	cookie := loginCookie(t, s)
	packageID := defaultPackageID(t, s)
	body := fmt.Sprintf(`{"name":"Startup","game_port":27015,"start_map":"c2m1_highway","game_mode":"coop","tickrate":100,"max_players":8,"extra_args":"-strictportbind +hostname \"Night Coop\"","package_id":%q}`, packageID)
	response := authenticatedJSON(t, s, cookie, http.MethodPost, "/api/instances", body)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	items, err := db.Instances(context.Background())
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	if items[0].SelectedPackageID != packageID || items[0].PackageVersion != "" || items[0].ExtraArgs != `-strictportbind +hostname "Night Coop"` {
		t.Fatalf("instance=%#v", items[0])
	}
}

func TestCreateRejectsMissingPackageAndReservedArguments(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	cookie := loginCookie(t, s)

	missing := authenticatedJSON(t, s, cookie, http.MethodPost, "/api/instances", `{"name":"Missing","game_port":27015,"start_map":"map","game_mode":"coop","tickrate":100,"max_players":8,"package_id":"00000000-0000-0000-0000-000000000000"}`)
	if missing.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing: status=%d body=%s", missing.Code, missing.Body.String())
	}

	reservedBody := fmt.Sprintf(`{"name":"Reserved","game_port":27015,"start_map":"map","game_mode":"coop","tickrate":100,"max_players":8,"extra_args":"-port 27016","package_id":%q}`, defaultPackageID(t, s))
	reserved := authenticatedJSON(t, s, cookie, http.MethodPost, "/api/instances", reservedBody)
	if reserved.Code != http.StatusUnprocessableEntity || !strings.Contains(reserved.Body.String(), "managed by the Panel") {
		t.Fatalf("reserved: status=%d body=%s", reserved.Code, reserved.Body.String())
	}
}

func TestCreateRejectsRuntimeImageOverride(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	cookie := loginCookie(t, s)
	body := fmt.Sprintf(`{"name":"Image Override","game_port":27015,"start_map":"map","game_mode":"coop","tickrate":100,"max_players":8,"package_id":%q,"runtime_image":"attacker/runtime:latest"}`, defaultPackageID(t, s))
	response := authenticatedJSON(t, s, cookie, http.MethodPost, "/api/instances", body)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "unknown field") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestUpdatePlansOnlyRequiredRuntimeWork(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	packageA := defaultPackageID(t, s)
	packageB := addTestPackage(t, s.packages, "second.zip", "second")
	value := domain.Instance{ID: "configured", NodeID: "local", Name: "Configured", ContainerID: "container", GamePort: 27015, StartMap: "map", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", SelectedPackageID: packageA, PackageVersion: packageA, DesiredState: domain.StateStopped, ActualState: domain.StateStopped}
	if err := db.CreateInstance(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	life := &fakeLifecycle{}
	manager := jobs.NewPersistentManager(db)
	coordinator := &updates.Coordinator{Lifecycle: life, Deployer: s.updates, Instances: db}
	s = New(db, s.auth, WithOperations(life, manager), WithContent(nil, nil, s.packages, s.updates, coordinator))
	cookie := loginCookie(t, s)

	nameOnly := fmt.Sprintf(`{"name":"Renamed","game_port":27015,"start_map":"map","game_mode":"coop","tickrate":100,"max_players":8,"extra_args":"","package_id":%q}`, packageA)
	response := authenticatedJSON(t, s, cookie, http.MethodPut, "/api/instances/"+value.ID, nameOnly)
	if response.Code != http.StatusOK || life.action != "" {
		t.Fatalf("name only: status=%d action=%q body=%s", response.Code, life.action, response.Body.String())
	}

	runtimeOnly := fmt.Sprintf(`{"name":"Renamed","game_port":27015,"start_map":"map","game_mode":"coop","tickrate":100,"max_players":8,"extra_args":"-strictportbind","package_id":%q}`, packageA)
	response = authenticatedJSON(t, s, cookie, http.MethodPut, "/api/instances/"+value.ID, runtimeOnly)
	if response.Code != http.StatusAccepted {
		t.Fatalf("runtime: status=%d body=%s", response.Code, response.Body.String())
	}
	var runtimeJob jobs.Job
	if err := json.Unmarshal(response.Body.Bytes(), &runtimeJob); err != nil {
		t.Fatal(err)
	}
	if got := waitForJob(t, manager, runtimeJob.ID); got.Status != jobs.Succeeded || life.action != "rebuild" {
		t.Fatalf("job=%#v action=%q", got, life.action)
	}

	life.action = ""
	combined := fmt.Sprintf(`{"name":"Renamed","game_port":27015,"start_map":"map","game_mode":"coop","tickrate":100,"max_players":8,"extra_args":"+hostname \"Changed\"","package_id":%q}`, packageB)
	response = authenticatedJSON(t, s, cookie, http.MethodPut, "/api/instances/"+value.ID, combined)
	if response.Code != http.StatusAccepted {
		t.Fatalf("combined: status=%d body=%s", response.Code, response.Body.String())
	}
	var combinedJob jobs.Job
	if err := json.Unmarshal(response.Body.Bytes(), &combinedJob); err != nil {
		t.Fatal(err)
	}
	if got := waitForJob(t, manager, combinedJob.ID); got.Status != jobs.Succeeded || life.action != "rebuild" {
		t.Fatalf("job=%#v action=%q", got, life.action)
	}
	stored, err := db.Instance(context.Background(), value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.SelectedPackageID != packageB || stored.PackageVersion != packageB || stored.ExtraArgs != `+hostname "Changed"` {
		t.Fatalf("instance=%#v", stored)
	}
}

func TestCreateRejectsDuplicateDeclaredPorts(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	cookie := loginCookie(t, s)
	r := httptest.NewRequest(http.MethodPost, "/api/instances", bytes.NewBufferString(`{"name":"Bad Ports","game_port":27015,"sourcetv_port":27015,"plugin_ports":[27020,27020],"start_map":"map","game_mode":"coop","tickrate":100,"max_players":8}`))
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateRejectsInvalidPluginPort(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	cookie := loginCookie(t, s)
	r := httptest.NewRequest(http.MethodPost, "/api/instances", bytes.NewBufferString(`{"name":"Bad Plugin Port","game_port":27015,"plugin_ports":[80],"start_map":"map","game_mode":"coop","tickrate":100,"max_players":8}`))
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestScheduleAcceptsSnakeCaseJSONAndRejectsUnknownFields(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	schedules := scheduler.NewService(db, fakeScheduleDispatcher{})
	defer schedules.Stop()
	s = New(db, s.auth, WithScheduler(schedules))
	cookie := loginCookie(t, s)

	t.Run("snake case", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/api/schedules", bytes.NewBufferString(`{"instance_id":"abc","type":"game_update","cron":"0 4 * * *","timezone":"Asia/Hong_Kong","online_policy":"skip","enabled":true,"payload":"{}"}`))
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"instance_id":"abc"`) || !strings.Contains(w.Body.String(), `"online_policy":"skip"`) {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("unknown field", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/api/schedules", bytes.NewBufferString(`{"instance_id":"abc","type":"game_update","cron":"0 4 * * *","timezone":"UTC","online_policy":"skip","enabled":true,"payload":"{}","surprise":true}`))
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "unknown field") {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	})
}

func TestGitHubSourceCRUD(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	cookie := loginCookie(t, s)
	request := authenticatedJSON(t, s, cookie, http.MethodPost, "/api/github-sources", `{"name":"第二源","repository":"owner/repo","asset_pattern":"^plugins\\.zip$"}`)
	if request.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", request.Code, request.Body.String())
	}
	var created domain.GitHubSource
	_ = json.Unmarshal(request.Body.Bytes(), &created)
	listed := authenticatedJSON(t, s, cookie, http.MethodGet, "/api/github-sources", "")
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), "第二源") {
		t.Fatalf("list=%d %s", listed.Code, listed.Body.String())
	}
	deleted := authenticatedJSON(t, s, cookie, http.MethodDelete, "/api/github-sources/"+created.ID, "")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete=%d %s", deleted.Code, deleted.Body.String())
	}
}

func TestInstanceActionRunsAsPersistentJob(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	cookie := loginCookie(t, s)
	v := map[string]any{"name": "Coop", "game_port": 27015, "start_map": "map", "game_mode": "coop", "tickrate": 100, "max_players": 8, "package_id": defaultPackageID(t, s)}
	raw, _ := json.Marshal(v)
	r := httptest.NewRequest(http.MethodPost, "/api/instances", bytes.NewReader(raw))
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	var created struct {
		ID string `json:"ID"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	life := &fakeLifecycle{}
	s = New(db, s.auth, WithOperations(life, jobs.NewPersistentManager(db)))
	r = httptest.NewRequest(http.MethodPost, "/api/instances/"+created.ID+"/actions", bytes.NewBufferString(`{"action":"start"}`))
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var job jobs.Job
	_ = json.Unmarshal(w.Body.Bytes(), &job)
	deadline := time.Now().Add(time.Second)
	for life.action == "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if life.action != "start" {
		t.Fatalf("action=%q", life.action)
	}
	r = httptest.NewRequest(http.MethodGet, "/api/jobs/"+job.ID, nil)
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"succeeded"`)) {
		t.Fatalf("job: %d %s", w.Code, w.Body.String())
	}
}

func TestStopActionRequiresConfirmation(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	life := &fakeLifecycle{}
	s = New(db, s.auth, WithOperations(life, jobs.NewPersistentManager(db)))
	cookie := loginCookie(t, s)
	r := httptest.NewRequest(http.MethodPost, "/api/instances/abc/actions", bytes.NewBufferString(`{"action":"stop"}`))
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusPreconditionRequired {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestContentReadRoutesReturnUnavailableWithoutManagers(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	cookie := loginCookie(t, s)
	for _, path := range []string{
		"/api/content/vpk/missing.vpk/download",
		"/api/instances/abc/private",
		"/api/instances/abc/private/history/cfg/server.cfg",
		"/api/instances/abc/private/file/cfg/server.cfg",
	} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: status=%d body=%s", path, w.Code, w.Body.String())
		}
	}
}

func TestContentReadRoutesAndJobFeed(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	root := t.TempDir()
	uploads, err := content.NewUploadManager(root)
	if err != nil {
		t.Fatal(err)
	}
	private := content.NewPrivateManager(root, 1024)
	s = New(db, s.auth, WithContent(uploads, private, nil, nil, nil))
	cookie := loginCookie(t, s)

	if _, err := private.Save(context.Background(), "abc", "cfg/server.cfg", []byte("first")); err != nil {
		t.Fatal(err)
	}
	if _, err := private.Save(context.Background(), "abc", "cfg/server.cfg", []byte("second")); err != nil {
		t.Fatal(err)
	}
	vpk := []byte("vpk-content")
	digest := sha256.Sum256(vpk)
	session, err := uploads.Begin("maps.vpk", int64(len(vpk)), hex.EncodeToString(digest[:]))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uploads.Write(session.ID, 0, bytes.NewReader(vpk)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := uploads.Complete(session.ID); err != nil {
		t.Fatal(err)
	}

	checks := []struct {
		path string
		want string
	}{
		{"/api/content/vpk/maps.vpk/download", "vpk-content"},
		{"/api/instances/abc/private", "cfg/server.cfg"},
		{"/api/instances/abc/private/file/cfg/server.cfg", "second"},
		{"/api/instances/abc/private/history/cfg/server.cfg", "cfg/server.cfg."},
	}
	for _, check := range checks {
		r := httptest.NewRequest(http.MethodGet, check.path, nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), check.want) {
			t.Fatalf("%s: status=%d body=%s", check.path, w.Code, w.Body.String())
		}
		if check.path == "/api/instances/abc/private" && !strings.Contains(w.Body.String(), `"path":"cfg/server.cfg"`) {
			t.Fatalf("private list must use stable lower-case fields: %s", w.Body.String())
		}
		if strings.Contains(w.Body.String(), root) {
			t.Fatalf("%s leaked data root in %s", check.path, w.Body.String())
		}
	}

	now := time.Now().UTC()
	if err := db.SaveJob(domain.JobRecord{ID: "job-1", InstanceID: "abc", Type: "update", Status: "running", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "job-1") {
		t.Fatalf("jobs: status=%d body=%s", w.Code, w.Body.String())
	}

	token, err := s.auth.Login("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	remote := httptest.NewServer(s.Handler())
	defer remote.Close()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remote.URL+"/api/jobs/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	response, err := remote.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(response.Body)
	var event strings.Builder
	for event.Len() < 4096 {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			t.Fatal(readErr)
		}
		event.WriteString(line)
		if line == "\n" {
			break
		}
	}
	cancel()
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(event.String(), "event: jobs") || !strings.Contains(event.String(), "job-1") {
		t.Fatalf("SSE status=%d event=%q", response.StatusCode, event.String())
	}
}

func TestConsoleWebSocketProxiesSupervisorAttach(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	v := domain.Instance{ID: "abc", NodeID: "local", Name: "one", ContainerID: "container-1", GamePort: 27015, StartMap: "map", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", DesiredState: domain.StateRunning, ActualState: domain.StateRunning}
	if err := db.CreateInstance(context.Background(), v); err != nil {
		t.Fatal(err)
	}
	attacher := &fakeAttacher{}
	s = New(db, s.auth, WithConsole(attacher))
	token, err := s.auth.Login("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	header := http.Header{"Cookie": []string{sessionCookie + "=" + token}}
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/api/instances/abc/console", header)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer attacher.peer.Close()
		raw := make([]byte, 7)
		_, _ = io.ReadFull(attacher.peer, raw)
		_, _ = attacher.peer.Write([]byte("console output"))
	}()
	if err := conn.WriteMessage(websocket.TextMessage, []byte("status\n")); err != nil {
		t.Fatal(err)
	}
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "console output" {
		t.Fatalf("got %q", raw)
	}
	<-done
}

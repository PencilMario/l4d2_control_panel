//go:build e2e

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/auth"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/docker"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/httpapi"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
	"github.com/not0721here/l4d2-control-panel/internal/metrics"
	"github.com/not0721here/l4d2-control-panel/internal/players"
	"github.com/not0721here/l4d2-control-panel/internal/scheduler"
	"github.com/not0721here/l4d2-control-panel/internal/secrets"
	"github.com/not0721here/l4d2-control-panel/internal/store"
	"github.com/not0721here/l4d2-control-panel/internal/updates"
)

const fixturePassword = "correct horse battery staple"

type fixtureLifecycle struct {
	db   *store.Store
	root string
}

func (l *fixtureLifecycle) Start(ctx context.Context, id string) error {
	time.Sleep(750 * time.Millisecond)
	instance, err := l.db.Instance(ctx, id)
	if err != nil {
		return err
	}
	for _, directory := range []string{"game/left4dead2", "private", "backups", "console"} {
		if err := os.MkdirAll(filepath.Join(l.root, "instances", id, filepath.FromSlash(directory)), 0750); err != nil {
			return err
		}
	}
	seed := filepath.Join(l.root, "instances", id, "game", "left4dead2", "cfg", "seeded.cfg")
	if err := os.MkdirAll(filepath.Dir(seed), 0750); err != nil {
		return err
	}
	if _, err := os.Stat(seed); errors.Is(err, os.ErrNotExist) {
		if err = os.WriteFile(seed, []byte("fixture lower layer\n"), 0640); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	instance.ContainerID = "fixture-" + id
	if instance.SelectedPackageID != "" {
		instance.PackageVersion = instance.SelectedPackageID
	}
	instance.DesiredState = domain.StateRunning
	instance.ActualState = domain.StateRunning
	return l.db.UpdateInstance(ctx, instance)
}

func (l *fixtureLifecycle) Stop(ctx context.Context, id string) error {
	time.Sleep(100 * time.Millisecond)
	instance, err := l.db.Instance(ctx, id)
	if err != nil {
		return err
	}
	instance.DesiredState = domain.StateStopped
	instance.ActualState = domain.StateStopped
	return l.db.UpdateInstance(ctx, instance)
}

func (l *fixtureLifecycle) Restart(ctx context.Context, id string) error {
	if err := l.Stop(ctx, id); err != nil {
		return err
	}
	return l.Start(ctx, id)
}

func (l *fixtureLifecycle) Rebuild(ctx context.Context, id string) error {
	return l.Restart(ctx, id)
}

func (l *fixtureLifecycle) Delete(ctx context.Context, id string, deleteData bool) error {
	if err := l.db.DeleteInstance(ctx, id); err != nil {
		return err
	}
	if deleteData {
		return os.RemoveAll(filepath.Join(l.root, "instances", id))
	}
	return nil
}

type fixtureConsoleClient struct {
	peer net.Conn
	mu   sync.Mutex
}

func (c *fixtureConsoleClient) write(value string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := io.WriteString(c.peer, value)
	return err
}

type fixtureConsole struct {
	mu      sync.Mutex
	clients map[string]map[*fixtureConsoleClient]struct{}
}

func (f *fixtureConsole) AttachSupervisor(_ context.Context, containerID string) (io.ReadWriteCloser, error) {
	client, peer := net.Pipe()
	attached := &fixtureConsoleClient{peer: peer}
	f.mu.Lock()
	if f.clients[containerID] == nil {
		f.clients[containerID] = make(map[*fixtureConsoleClient]struct{})
	}
	f.clients[containerID][attached] = struct{}{}
	f.mu.Unlock()
	go func() {
		defer func() {
			f.mu.Lock()
			delete(f.clients[containerID], attached)
			f.mu.Unlock()
			_ = peer.Close()
		}()
		var initial strings.Builder
		initial.WriteString("fixture console ready\n")
		for index := 0; index < 120; index++ {
			fmt.Fprintf(&initial, "fixture overflow %03d | deterministic console output\n", index)
		}
		if err := attached.write(initial.String()); err != nil {
			return
		}
		buffer := make([]byte, 4096)
		for {
			n, err := peer.Read(buffer)
			if n > 0 {
				if writeErr := attached.write("echo:" + string(buffer[:n])); writeErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return client, nil
}

func (f *fixtureConsole) Emit(containerID, value string) bool {
	f.mu.Lock()
	clients := make([]*fixtureConsoleClient, 0, len(f.clients[containerID]))
	for client := range f.clients[containerID] {
		clients = append(clients, client)
	}
	f.mu.Unlock()
	for _, client := range clients {
		_ = client.write(value)
	}
	return len(clients) > 0
}

type fixturePlayers struct{}

func (fixturePlayers) Summary(context.Context, string) (players.Summary, error) {
	return players.Summary{Map: "c2m1_highway", Players: 1, MaxPlayers: 8}, nil
}
func (fixturePlayers) Online(context.Context, string) (players.Snapshot, error) {
	score := int32(12)
	secure := true
	return players.Snapshot{
		Map:        "c2m1_highway",
		MaxPlayers: 8,
		Match:      players.MatchInfo{Hostname: "Fixture Host", Version: "2.2.4.3 10097", Secure: &secure, OS: "Linux Dedicated", Map: "c2m1_highway", PrivateAddress: "127.0.1.1:27015", PublicAddress: "203.0.113.15:27015", Humans: 1, MaxPlayers: 8},
		Players: []players.OnlinePlayer{{
			UserID: 7, Name: "Fixture Player", UniqueID: "STEAM_1:0:42", Connected: "01:30", Ping: 45, Loss: 0, Score: &score, Duration: 90 * time.Second,
		}},
	}, nil
}
func (fixturePlayers) Kick(context.Context, string, int) error {
	time.Sleep(100 * time.Millisecond)
	return nil
}
func (fixturePlayers) Ban(context.Context, string, int, int) error {
	time.Sleep(100 * time.Millisecond)
	return nil
}

type fixtureResources struct{}

func (fixtureResources) Running(context.Context, string) (bool, error) { return true, nil }
func (fixtureResources) Stats(context.Context, string) (docker.ResourceStats, error) {
	return docker.ResourceStats{CPUPercent: 12.5, MemoryBytes: 768 << 20}, nil
}

type fixturePerformance struct{}

func (fixturePerformance) Latest(string) (metrics.Snapshot, bool) {
	running := true
	gameMap := "c2m1_highway"
	playersOnline, maxPlayers := 1, 8
	return metrics.Snapshot{
		Timestamp: time.Date(2026, 7, 15, 12, 0, 10, 0, time.UTC), RunID: "fixture-run", ContainerRunning: &running,
		ImageSizeBytes: fixtureUint64(3_221_225_472),
		CPUPercent:     fixtureFloat64(12.5), MemoryBytes: fixtureUint64(768 << 20), MemoryLimitBytes: fixtureUint64(2 << 30), MemoryPercent: fixtureFloat64(37.5),
		NetworkRXBytesPerSecond: fixtureFloat64(128), NetworkTXBytesPerSecond: fixtureFloat64(64), NetworkRXBytes: fixtureUint64(4096), NetworkTXBytes: fixtureUint64(2048),
		BlockReadBytesPerSecond: fixtureFloat64(32), BlockWriteBytesPerSecond: fixtureFloat64(16), BlockReadBytes: fixtureUint64(1024), BlockWriteBytes: fixtureUint64(512),
		PIDs: fixtureUint64(24), UptimeSeconds: fixtureUint64(3600), A2SLatencyMS: fixtureFloat64(2.5), Map: &gameMap, Players: &playersOnline, MaxPlayers: &maxPlayers,
	}, true
}

func (fixturePerformance) History(string) []metrics.Snapshot {
	zero := 0.0
	return []metrics.Snapshot{
		{Timestamp: time.Date(2026, 7, 15, 12, 0, 5, 0, time.UTC), RunID: "fixture-run", CPUPercent: fixtureFloat64(10), MemoryPercent: fixtureFloat64(37)},
		{Timestamp: time.Date(2026, 7, 15, 12, 0, 10, 0, time.UTC), RunID: "fixture-run", CPUPercent: fixtureFloat64(12.5), MemoryPercent: fixtureFloat64(37.5), NetworkRXBytesPerSecond: &zero, NetworkTXBytesPerSecond: &zero, BlockReadBytesPerSecond: &zero, BlockWriteBytesPerSecond: &zero},
	}
}

func fixtureFloat64(value float64) *float64 { return &value }
func fixtureUint64(value uint64) *uint64    { return &value }

type fixtureSystem struct{}

func (fixtureSystem) Info(context.Context) (docker.Info, error) {
	return docker.Info{ServerVersion: "fixture-29.6.1", ContainersRunning: 1}, nil
}

type fixtureGameUpdater struct{}

func (fixtureGameUpdater) HasMaintenance(context.Context, string) (bool, error) { return false, nil }
func (fixtureGameUpdater) UpdateGame(context.Context, string, domain.Instance) error {
	time.Sleep(250 * time.Millisecond)
	return nil
}

type fixtureDispatcher struct{}

func (fixtureDispatcher) Dispatch(context.Context, domain.ScheduledTask) error { return nil }

func main() {
	root, cleanup, err := fixtureRoot()
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()
	db, err := store.Open(filepath.Join(root, "panel.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	sessions, err := auth.NewPersistentService(db)
	if err != nil {
		log.Fatal(err)
	}
	if !sessions.Configured() {
		if err := sessions.Bootstrap(fixturePassword); err != nil {
			log.Fatal(err)
		}
	}
	secretService, err := secrets.New(db, bytes.Repeat([]byte{0x2a}, 32))
	if err != nil {
		log.Fatal(err)
	}
	uploads, err := content.NewUploadManager(root)
	if err != nil {
		log.Fatal(err)
	}
	packages, err := content.NewPackageManager(root)
	if err != nil {
		log.Fatal(err)
	}
	private := content.NewPrivateManager(root, 1<<20)
	privateUploads := content.NewPrivateUploadManager(root, 8<<20)
	pipeline, err := newFixturePipeline(root)
	if err != nil {
		log.Fatal(err)
	}
	lifecycle := &fixtureLifecycle{db: db, root: root}
	jobManager := jobs.NewPersistentManager(db)
	seedJobs(db)
	packageUpdates := &updates.Coordinator{Lifecycle: lifecycle, Deployer: pipeline, Instances: db}
	gameUpdates := &updates.GameCoordinator{Root: root, Instances: db, Lifecycle: lifecycle, Updater: fixtureGameUpdater{}, Private: private, Packages: packages, Deployer: pipeline}
	schedules := scheduler.NewService(db, fixtureDispatcher{})
	defer schedules.Stop()

	console := &fixtureConsole{clients: make(map[string]map[*fixtureConsoleClient]struct{})}
	api := httpapi.New(
		db,
		sessions,
		httpapi.WithOperations(lifecycle, jobManager),
		httpapi.WithConsole(console),
		httpapi.WithPlayers(fixturePlayers{}),
		httpapi.WithContent(uploads, private, packages, pipeline, packageUpdates),
		httpapi.WithPrivateUploads(privateUploads),
		httpapi.WithGameUpdates(gameUpdates),
		httpapi.WithScheduler(schedules),
		httpapi.WithSecrets(secretService),
		httpapi.WithResources(fixtureResources{}),
		httpapi.WithPerformance(fixturePerformance{}),
		httpapi.WithSystem(fixtureSystem{}),
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/__e2e/private-lower", privateLowerDiagnostic(root))
	mux.HandleFunc("/__e2e/console-output", consoleOutputControl(console))
	mux.Handle("/api/", api.Handler())
	mux.Handle("/", spaHandler(webRoot()))
	address := os.Getenv("L4D2_E2E_LISTEN")
	if address == "" {
		address = "127.0.0.1:18082"
	}
	server := &http.Server{Addr: address, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Printf("e2e fixture listening on http://%s", address)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func privateLowerDiagnostic(root string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := r.URL.Query().Get("id")
		name := filepath.Clean(filepath.FromSlash(r.URL.Query().Get("path")))
		if filepath.Base(id) != id || id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\\`) || name == "." || filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			http.Error(w, "invalid diagnostic path", http.StatusBadRequest)
			return
		}
		base, err := filepath.Abs(filepath.Join(root, "instances", id, "game", "left4dead2"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		target, err := filepath.Abs(filepath.Join(base, name))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		relative, err := filepath.Rel(base, target)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			http.Error(w, "invalid diagnostic path", http.StatusBadRequest)
			return
		}
		value, err := os.ReadFile(target)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		_, _ = w.Write(value)
	}
}

func consoleOutputControl(console *fixtureConsole) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		value, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !console.Emit("fixture-"+r.URL.Query().Get("id"), string(value)) {
			http.Error(w, "console unavailable", http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func fixtureRoot() (string, func(), error) {
	if configured := os.Getenv("L4D2_E2E_DATA_ROOT"); configured != "" {
		if err := os.MkdirAll(configured, 0750); err != nil {
			return "", nil, err
		}
		return configured, func() {}, nil
	}
	root, err := os.MkdirTemp("", "l4d2-panel-e2e-*")
	return root, func() { _ = os.RemoveAll(root) }, err
}

func newFixturePipeline(root string) (*updates.Pipeline, error) {
	pipeline := updates.New(root)
	return pipeline, pipeline.Recover(context.Background())
}

func seedJobs(db *store.Store) {
	if err := db.SetCompletedJobLimit(40); err != nil {
		log.Fatal(err)
	}
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	successStarted := now.Add(time.Second)
	successFinished := now.Add(30 * time.Second)
	slowStarted := now.Add(2 * time.Second)
	interruptedStarted := now.Add(3 * time.Second)
	interruptedFinished := now.Add(45 * time.Second)
	values := []domain.JobRecord{
		{ID: "fixture-success", Type: "fixture_success", Stage: "complete", Status: "succeeded", Percent: 100, CreatedAt: now, UpdatedAt: successFinished, StartedAt: &successStarted, FinishedAt: &successFinished},
		{ID: "fixture-slow", Type: "fixture_slow", Stage: "waiting", Status: "running", Message: "deterministic slow job", Percent: 50, CreatedAt: now, UpdatedAt: slowStarted, StartedAt: &slowStarted},
		{ID: "fixture-interrupted", Type: "fixture_recovery", Stage: "recovered", Status: "interrupted", Error: "recovered after fixture restart", Percent: 25, CreatedAt: now, UpdatedAt: interruptedFinished, StartedAt: &interruptedStarted, FinishedAt: &interruptedFinished},
	}
	for _, value := range values {
		if err := db.SaveJob(value); err != nil {
			log.Fatal(err)
		}
	}

	failureStarted := now.Add(2 * time.Second)
	failureProgress := now.Add(time.Minute)
	failureFinished := now.Add(2*time.Minute + 20*time.Second)
	failure := domain.JobRecord{ID: "fixture-failure", Type: "fixture_failure", CreatedAt: now}
	steps := []struct {
		status, stage, message, kind string
		percent                      int
		updated                      time.Time
	}{
		{status: "pending", message: "Task queued", kind: "queued", updated: now},
		{status: "running", stage: "prepare", message: "Task started", kind: "started", updated: failureStarted},
		{status: "running", stage: "download", message: "downloading fixture payload", kind: "progress", percent: 45, updated: failureProgress},
		{status: "failed", stage: "download", message: "deterministic fixture failure", kind: "failed", percent: 45, updated: failureFinished},
	}
	for _, step := range steps {
		failure.Status = step.status
		failure.Stage = step.stage
		failure.Message = step.message
		failure.Percent = step.percent
		failure.UpdatedAt = step.updated
		if step.kind != "queued" {
			failure.StartedAt = &failureStarted
		}
		if step.kind == "failed" {
			failure.Error = step.message
			failure.FinishedAt = &failureFinished
		}
		if err := db.SaveJobWithEvent(failure, domain.JobEvent{
			JobID: failure.ID, Kind: step.kind, Stage: step.stage,
			Percent: step.percent, Message: step.message, CreatedAt: step.updated,
		}); err != nil {
			log.Fatal(err)
		}
	}
}

func webRoot() string {
	if configured := os.Getenv("L4D2_E2E_WEB_ROOT"); configured != "" {
		return configured
	}
	return "dist"
}

func spaHandler(root string) http.Handler {
	assets := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			assets.ServeHTTP(w, r)
			return
		}
		index := filepath.Join(root, "index.html")
		if _, err := os.Stat(index); err != nil {
			http.Error(w, fmt.Sprintf("SPA unavailable: %v", err), http.StatusServiceUnavailable)
			return
		}
		http.ServeFile(w, r, index)
	})
}

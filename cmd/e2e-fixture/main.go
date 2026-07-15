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
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/auth"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/docker"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/httpapi"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
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

type fixtureConsole struct{}

func (fixtureConsole) AttachSupervisor(context.Context, string) (io.ReadWriteCloser, error) {
	client, peer := net.Pipe()
	go func() {
		defer peer.Close()
		if _, err := io.WriteString(peer, "fixture console ready\n"); err != nil {
			return
		}
		buffer := make([]byte, 4096)
		for {
			n, err := peer.Read(buffer)
			if n > 0 {
				if _, writeErr := io.WriteString(peer, "echo:"+string(buffer[:n])); writeErr != nil {
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

type fixturePlayers struct{}

func (fixturePlayers) Online(context.Context, string) (players.Snapshot, error) {
	return players.Snapshot{
		Map:        "c2m1_highway",
		MaxPlayers: 8,
		Players: []players.OnlinePlayer{{
			UserID: 7, Name: "Fixture Player", Score: 12, Duration: 90 * time.Second,
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

func (fixtureResources) Stats(context.Context, string) (docker.ResourceStats, error) {
	return docker.ResourceStats{CPUPercent: 12.5, MemoryBytes: 768 << 20}, nil
}

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
	pipeline, err := newFixturePipeline(root)
	if err != nil {
		log.Fatal(err)
	}
	lifecycle := &fixtureLifecycle{db: db, root: root}
	jobManager := jobs.NewPersistentManager(db)
	seedJobs(db)
	packageUpdates := &updates.Coordinator{Lifecycle: lifecycle, Deployer: pipeline, Instances: db}
	gameUpdates := &updates.GameCoordinator{Root: root, Instances: db, Lifecycle: lifecycle, Updater: fixtureGameUpdater{}, Private: private}
	schedules := scheduler.NewService(db, fixtureDispatcher{})
	defer schedules.Stop()

	api := httpapi.New(
		db,
		sessions,
		httpapi.WithOperations(lifecycle, jobManager),
		httpapi.WithConsole(fixtureConsole{}),
		httpapi.WithPlayers(fixturePlayers{}),
		httpapi.WithContent(uploads, private, packages, pipeline, packageUpdates),
		httpapi.WithGameUpdates(gameUpdates),
		httpapi.WithScheduler(schedules),
		httpapi.WithSecrets(secretService),
		httpapi.WithResources(fixtureResources{}),
		httpapi.WithSystem(fixtureSystem{}),
	)
	mux := http.NewServeMux()
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
	now := time.Now().UTC()
	values := []domain.JobRecord{
		{ID: "fixture-success", Type: "fixture_success", Stage: "complete", Status: "succeeded", Percent: 100, CreatedAt: now, UpdatedAt: now},
		{ID: "fixture-failure", Type: "fixture_failure", Stage: "failed", Status: "failed", Error: "deterministic fixture failure", Percent: 45, CreatedAt: now, UpdatedAt: now},
		{ID: "fixture-slow", Type: "fixture_slow", Stage: "waiting", Status: "running", Message: "deterministic slow job", Percent: 50, CreatedAt: now, UpdatedAt: now},
		{ID: "fixture-interrupted", Type: "fixture_recovery", Stage: "recovered", Status: "interrupted", Error: "recovered after fixture restart", Percent: 25, CreatedAt: now, UpdatedAt: now},
	}
	for _, value := range values {
		if err := db.SaveJob(value); err != nil {
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

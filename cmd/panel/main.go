package main

import (
	"context"
	"errors"
	"github.com/not0721here/l4d2-control-panel/internal/a2s"
	"github.com/not0721here/l4d2-control-panel/internal/auth"
	"github.com/not0721here/l4d2-control-panel/internal/automation"
	"github.com/not0721here/l4d2-control-panel/internal/config"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/disk"
	"github.com/not0721here/l4d2-control-panel/internal/docker"
	"github.com/not0721here/l4d2-control-panel/internal/health"
	"github.com/not0721here/l4d2-control-panel/internal/httpapi"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
	"github.com/not0721here/l4d2-control-panel/internal/lifecycle"
	"github.com/not0721here/l4d2-control-panel/internal/maintenance"
	"github.com/not0721here/l4d2-control-panel/internal/players"
	"github.com/not0721here/l4d2-control-panel/internal/ports"
	"github.com/not0721here/l4d2-control-panel/internal/provisioning"
	"github.com/not0721here/l4d2-control-panel/internal/releases"
	"github.com/not0721here/l4d2-control-panel/internal/scheduler"
	"github.com/not0721here/l4d2-control-panel/internal/secrets"
	"github.com/not0721here/l4d2-control-panel/internal/store"
	"github.com/not0721here/l4d2-control-panel/internal/updates"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

type httpShutdowner interface {
	Shutdown(context.Context) error
}

type jobWaiter interface {
	Wait(context.Context) error
}

func shutdownPanel(ctx context.Context, server httpShutdowner, stopScheduler func(), waiter jobWaiter) error {
	httpErr := server.Shutdown(ctx)
	schedulerDone := make(chan struct{})
	go func() {
		stopScheduler()
		close(schedulerDone)
	}()
	var schedulerErr error
	select {
	case <-schedulerDone:
	case <-ctx.Done():
		schedulerErr = ctx.Err()
	}
	return errors.Join(httpErr, schedulerErr, waiter.Wait(ctx))
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	sessions, err := auth.NewPersistentService(db)
	if err != nil {
		log.Fatal(err)
	}
	if !sessions.Configured() {
		password := os.Getenv("L4D2_PANEL_ADMIN_PASSWORD")
		if password == "" {
			log.Fatal("L4D2_PANEL_ADMIN_PASSWORD is required for initial bootstrap")
		}
		if err := sessions.Bootstrap(password); err != nil {
			log.Fatal(err)
		}
	}
	secretKey, err := secrets.LoadOrCreateKey(filepath.Join(cfg.PanelDir, "secret.key"))
	if err != nil {
		log.Fatal(err)
	}
	secretService, err := secrets.New(db, secretKey)
	if err != nil {
		log.Fatal(err)
	}
	dockerHost := os.Getenv("DOCKER_HOST")
	if dockerHost == "" {
		dockerHost = "http://127.0.0.1:23750"
	}
	steamCredentials := func() (string, string) {
		username, _, _ := secretService.Get(context.Background(), "steam_username")
		password, _, _ := secretService.Get(context.Background(), "steam_password")
		return username, password
	}
	engine := docker.NewEngine(dockerHost, docker.WithDownloadProxy(os.Getenv("L4D2_PANEL_DOWNLOAD_PROXY")), docker.WithSteamCredentials(steamCredentials))
	updatePipeline := updates.New(cfg.DataRoot)
	if err := updatePipeline.Recover(context.Background()); err != nil {
		log.Fatal(err)
	}
	packageManager, err := content.NewPackageManager(cfg.DataRoot)
	if err != nil {
		log.Fatal(err)
	}
	portChecker := ports.Checker{Configured: func(ctx context.Context) ([]ports.Reservation, error) {
		instances, err := db.Instances(ctx)
		if err != nil {
			return nil, err
		}
		return ports.Reservations(instances), nil
	}, Listening: ports.IsListening}
	healthChecker := health.Checker{Host: cfg.GameHost, Query: a2s.Client{}, Probe: engine}
	instanceProvisioner := provisioning.Service{Root: cfg.DataRoot, Installer: engine, Packages: packageManager, Deployer: updatePipeline, Instances: db}
	life := lifecycle.New(db, engine, portChecker, cfg.DataRoot, lifecycle.WithHealth(healthChecker), lifecycle.WithSpace(disk.Checker{}, 12<<30), lifecycle.WithProvisioner(instanceProvisioner))
	if unknown, reconcileErr := life.Reconcile(context.Background()); reconcileErr != nil {
		log.Printf("container reconciliation deferred: %v", reconcileErr)
	} else if len(unknown) > 0 {
		log.Printf("found %d unclaimed managed containers", len(unknown))
	}
	jobManager := jobs.NewPersistentManager(db)
	playerService := players.NewService(db, a2s.Client{}, engine, cfg.GameHost)
	uploadManager, err := content.NewUploadManager(cfg.DataRoot)
	if err != nil {
		log.Fatal(err)
	}
	privateManager := content.NewPrivateManager(cfg.DataRoot, 1<<20)
	updateCoordinator := &updates.Coordinator{Lifecycle: life, Deployer: updatePipeline}
	gameCoordinator := &updates.GameCoordinator{Root: cfg.DataRoot, Instances: db, Lifecycle: life, Updater: engine, Private: privateManager}
	dispatcher := automation.Dispatcher{Jobs: jobManager, Players: playerService, Packages: packageManager, PackagesUpdate: updateCoordinator, GameUpdate: gameCoordinator, Releases: releases.Client{}, Maintenance: maintenance.New(cfg.DataRoot), Secrets: secretService}
	scheduleService := scheduler.NewService(db, dispatcher)
	api := httpapi.New(db, sessions, httpapi.WithOperations(life, jobManager), httpapi.WithConsole(engine), httpapi.WithPlayers(playerService), httpapi.WithContent(uploadManager, privateManager, packageManager, updatePipeline, updateCoordinator), httpapi.WithGameUpdates(gameCoordinator), httpapi.WithScheduler(scheduleService), httpapi.WithSecrets(secretService), httpapi.WithResources(engine), httpapi.WithSystem(engine))
	mux := http.NewServeMux()
	mux.Handle("/api/", api.Handler())
	web := os.Getenv("L4D2_PANEL_WEB_ROOT")
	if web == "" {
		web = "web/dist"
	}
	mux.Handle("/assets/", http.FileServer(http.Dir(web)))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			if _, err := os.Stat(filepath.Join(web, filepath.Clean(r.URL.Path))); err == nil {
				http.ServeFile(w, r, filepath.Join(web, filepath.Clean(r.URL.Path)))
				return
			}
		}
		http.ServeFile(w, r, filepath.Join(web, "index.html"))
	})
	server := &http.Server{Addr: cfg.ListenAddress, Handler: mux, ReadHeaderTimeout: 10_000_000_000}
	log.Printf("panel listening on %s", cfg.ListenAddress)
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case err := <-serverErrors:
		drain, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		shutdownErr := shutdownPanel(drain, server, scheduleService.Stop, jobManager)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("panel server stopped: %v", errors.Join(err, shutdownErr))
		} else if shutdownErr != nil {
			log.Printf("panel drain failed: %v", shutdownErr)
		}
	case received := <-signals:
		log.Printf("received %s; draining panel", received)
		drain, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := shutdownPanel(drain, server, scheduleService.Stop, jobManager); err != nil {
			log.Printf("panel shutdown incomplete: %v", err)
		}
	}
}

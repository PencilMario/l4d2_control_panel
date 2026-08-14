package main

import (
	"context"
	"errors"
	"github.com/not0721here/l4d2-control-panel/internal/a2s"
	"github.com/not0721here/l4d2-control-panel/internal/a2sdefense"
	"github.com/not0721here/l4d2-control-panel/internal/accelerator"
	"github.com/not0721here/l4d2-control-panel/internal/auth"
	"github.com/not0721here/l4d2-control-panel/internal/automation"
	"github.com/not0721here/l4d2-control-panel/internal/config"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/crashanalysis"
	"github.com/not0721here/l4d2-control-panel/internal/crashartifacts"
	"github.com/not0721here/l4d2-control-panel/internal/crashreports"
	"github.com/not0721here/l4d2-control-panel/internal/crashsymbols"
	"github.com/not0721here/l4d2-control-panel/internal/disk"
	"github.com/not0721here/l4d2-control-panel/internal/docker"
	"github.com/not0721here/l4d2-control-panel/internal/gamelogs"
	"github.com/not0721here/l4d2-control-panel/internal/health"
	"github.com/not0721here/l4d2-control-panel/internal/httpapi"
	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
	"github.com/not0721here/l4d2-control-panel/internal/lifecycle"
	"github.com/not0721here/l4d2-control-panel/internal/maintenance"
	"github.com/not0721here/l4d2-control-panel/internal/metrics"
	sharedmigration "github.com/not0721here/l4d2-control-panel/internal/migration"
	"github.com/not0721here/l4d2-control-panel/internal/overlayfs"
	"github.com/not0721here/l4d2-control-panel/internal/players"
	"github.com/not0721here/l4d2-control-panel/internal/ports"
	"github.com/not0721here/l4d2-control-panel/internal/provisioning"
	"github.com/not0721here/l4d2-control-panel/internal/releases"
	"github.com/not0721here/l4d2-control-panel/internal/scheduler"
	"github.com/not0721here/l4d2-control-panel/internal/secrets"
	"github.com/not0721here/l4d2-control-panel/internal/store"
	"github.com/not0721here/l4d2-control-panel/internal/traffic"
	"github.com/not0721here/l4d2-control-panel/internal/updates"
	"github.com/not0721here/l4d2-control-panel/internal/vpkrestart"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type httpShutdowner interface {
	Shutdown(context.Context) error
}

type jobWaiter interface {
	Wait(context.Context) error
}

type samplerStopper interface {
	Stop(context.Context) error
}

type a2sEventRunner interface {
	Run(context.Context)
}

type crashAnalysisEnqueuer interface {
	Enqueue(context.Context, string, bool) error
}

func enqueueCrashReportStackwalk(ctx context.Context, worker crashAnalysisEnqueuer, report crashreports.Report) error {
	if worker == nil {
		return errors.New("crash analysis worker is not started")
	}
	return worker.Enqueue(ctx, report.ID, false)
}

func startA2SEventLogger(parent context.Context, logger a2sEventRunner) func() {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		logger.Run(ctx)
	}()
	return func() {
		cancel()
		<-done
	}
}

const startMinimumFreeBytes uint64 = 1 << 30

func shutdownPanel(ctx context.Context, server httpShutdowner, stopScheduler func(), sampler samplerStopper, waiter jobWaiter) error {
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
	return errors.Join(httpErr, schedulerErr, sampler.Stop(ctx), waiter.Wait(ctx))
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
	secretKey, err := secrets.LoadOrCreateKey(filepath.Join(cfg.PanelDir, "secret.key"))
	if err != nil {
		log.Fatal(err)
	}
	secretService, err := secrets.New(db, secretKey)
	if err != nil {
		log.Fatal(err)
	}
	var analysisWorker *crashanalysis.Worker
	dockerHost := os.Getenv("DOCKER_HOST")
	if dockerHost == "" {
		dockerHost = "unix:///run/l4d2-panel/proxy.sock"
	}
	steamCredentials := func() (string, string) {
		username, _, _ := secretService.Get(context.Background(), "steam_username")
		password, _, _ := secretService.Get(context.Background(), "steam_password")
		return username, password
	}
	engine := docker.NewEngine(dockerHost, docker.WithDownloadProxy(os.Getenv("L4D2_PANEL_DOWNLOAD_PROXY")), docker.WithSteamCredentials(steamCredentials))
	crashReportManager, err := crashreports.New(crashreports.Config{
		Root:            cfg.CrashReportsDir,
		Token:           cfg.CrashReportToken,
		ResolveInstance: newCrashReportInstanceResolver(cfg.DataRoot, db),
		ResolveContainerID: func(ctx context.Context, instanceID string) (string, error) {
			instance, err := db.Instance(ctx, instanceID)
			if err != nil {
				return "", err
			}
			return instance.ContainerID, nil
		},
		EnqueueAnalysis: func(ctx context.Context, report crashreports.Report) error {
			if report.InstanceID == "" {
				return nil
			}
			return enqueueCrashReportStackwalk(ctx, analysisWorker, report)
		},
		AnalysisEnqueueError: func(err error) { log.Printf("crash analysis enqueue: %v", err) },
	})
	if err != nil {
		log.Fatal(err)
	}
	symbolGenerator, err := crashsymbols.New(crashsymbols.Config{Path: cfg.DumpSymsPath})
	if err != nil {
		log.Fatal(err)
	}
	symbolIndexer := crashsymbols.Indexer{
		Generator: symbolGenerator,
		Source: crashsymbols.FilesystemRootSource{
			ReleasesRoot:  cfg.GameReleasesDir,
			InstancesRoot: cfg.InstancesDir,
		},
		Store: crashReportManager,
	}
	stopCrashSymbolIndexer := symbolIndexer.Start(context.Background(), 0, func(summary crashsymbols.Summary, err error) {
		if err != nil {
			log.Printf("crash symbol indexing: %v", err)
			return
		}
		log.Printf("crash symbol indexing: scanned=%d candidates=%d generated=%d skipped=%d duplicates=%d failed=%d", summary.Scanned, summary.Candidates, summary.Generated, summary.Skipped, summary.Duplicates, summary.Failed)
		for _, failure := range summary.Failures {
			log.Printf("crash symbol module failed path=%s error=%s", failure.Path, failure.Error)
		}
	})
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
	stackwalker, err := crashanalysis.NewStackwalker(crashanalysis.StackwalkConfig{
		Path:       cfg.StackwalkPath,
		SymbolRoot: filepath.Join(cfg.CrashReportsDir, "symbols"),
	})
	if err != nil {
		log.Fatal(err)
	}
	artifactPreparer := &crashartifacts.Preparer{
		Containers: engine,
		Store:      crashReportManager,
		Generator:  symbolGenerator,
		TempRoot:   filepath.Join(cfg.CrashReportsDir, "incoming"),
	}
	analysisWorker, err = crashanalysis.NewWorker(crashanalysis.WorkerConfig{
		Store:       crashReportManager,
		Stackwalker: stackwalker,
		Preparer:    artifactPreparer,
		AIProvider: func(ctx context.Context) (crashanalysis.AIAnalyzer, string, error) {
			settings, err := db.CrashAnalysisSettings(ctx)
			if err != nil {
				return nil, "", err
			}
			if strings.TrimSpace(settings.Endpoint) == "" || strings.TrimSpace(settings.Model) == "" {
				return nil, settings.Model, nil
			}
			apiKey, _, err := secretService.Get(ctx, "accelerator_ai_api_key")
			if err != nil {
				return nil, settings.Model, err
			}
			client, err := crashanalysis.NewOpenAIClient(crashanalysis.OpenAIConfig{
				Endpoint: settings.Endpoint, Model: settings.Model, APIKey: apiKey,
			})
			return client, settings.Model, err
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := analysisWorker.Start(context.Background()); err != nil {
		log.Fatal(err)
	}
	jobLogManager, err := joblogs.Open(filepath.Join(cfg.PanelDir, "job-logs"), joblogs.Options{Redactor: joblogs.NewRedactor(func() []string {
		values := []string{os.Getenv("L4D2_PANEL_ADMIN_PASSWORD")}
		for _, name := range []string{"steam_username", "steam_password", "github_token"} {
			if value, found, getErr := secretService.Get(context.Background(), name); getErr == nil && found {
				values = append(values, value)
			}
		}
		return values
	})})
	if err != nil {
		log.Fatal(err)
	}
	defer jobLogManager.Close()
	jobIDs, err := db.JobIDs(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	if err := jobLogManager.CleanupOrphans(context.Background(), jobIDs); err != nil {
		log.Printf("cleanup orphan job logs: %v", err)
	}
	db.SetPrunedJobHook(func(ids []string) {
		for _, id := range ids {
			if deleteErr := jobLogManager.Delete(context.Background(), id); deleteErr != nil {
				log.Printf("delete pruned job log %s: %v", id, deleteErr)
			}
		}
	})
	trafficClient := traffic.NewUnixClient(strings.TrimPrefix(dockerHost, "unix://"))
	updatePipeline := updates.New(cfg.DataRoot)
	if err := updatePipeline.Recover(context.Background()); err != nil {
		log.Fatal(err)
	}
	packageManager, err := content.NewPackageManager(cfg.DataRoot)
	if err != nil {
		log.Fatal(err)
	}
	acceleratorManager, err := accelerator.New(accelerator.Config{
		InstancesRoot: cfg.InstancesDir,
		CacheRoot:     filepath.Join(cfg.PanelDir, "accelerator-cache"),
		DownloadURLProvider: func(ctx context.Context) (string, error) {
			settings, err := db.AcceleratorSettings(ctx)
			if err != nil {
				return "", err
			}
			if strings.TrimSpace(settings.DownloadURL) == "" {
				return "", errors.New("Accelerator download URL is not configured")
			}
			return settings.DownloadURL, nil
		},
		GitHubProxyProvider: func(context.Context) (string, error) {
			settings, err := db.AcceleratorSettings(context.Background())
			if err != nil || !settings.UseGitHubProxy {
				return "", err
			}
			return db.GitHubReleasesAccelerator()
		},
		Token:          cfg.CrashReportToken,
		PanelPort:      cfg.AcceleratorPort,
		TargetPlatform: runtime.GOOS,
	})
	if err != nil {
		log.Fatal(err)
	}
	overlaySocket := os.Getenv("L4D2_PANEL_OVERLAY_SOCKET")
	if overlaySocket == "" {
		overlaySocket = "/run/l4d2-panel-overlay/overlay.sock"
	}
	overlayClient := overlayfs.NewClient(overlaySocket)
	a2sDefenseSocket := os.Getenv("L4D2_PANEL_A2S_DEFENSE_SOCKET")
	if a2sDefenseSocket == "" {
		a2sDefenseSocket = "/run/l4d2-panel-a2s-defense/a2s-defense.sock"
	}
	a2sDefenseClient := a2sdefense.NewClient(a2sDefenseSocket)
	a2sDefenseCoordinator := a2sdefense.NewCoordinator(db, a2sDefenseClient, time.Minute, time.Now)
	a2sDefenseCoordinator.Start(context.Background())
	updatePipeline.WithSharedOverlay(overlayClient, db)
	portChecker := ports.Checker{Configured: func(ctx context.Context) ([]ports.Reservation, error) {
		instances, err := db.Instances(ctx)
		if err != nil {
			return nil, err
		}
		return ports.Reservations(instances), nil
	}, Listening: ports.IsListening}
	healthChecker := health.Checker{Host: cfg.GameHost, Query: a2s.Client{}, Probe: engine}
	instanceProvisioner := provisioning.Service{Root: cfg.DataRoot, Packages: packageManager, Sources: db, Deployer: updatePipeline, Instances: db, SharedState: db, Overlay: overlayClient, Accelerator: acceleratorManager}
	sharedGate := maintenance.NewGate()
	gameLogManager := gamelogs.NewManager(cfg.DataRoot, gamelogs.Options{})
	a2sEventLogger := a2sdefense.NewEventLogger(a2sDefenseClient, db, gameLogManager, time.Second, func(err error) { log.Printf("A2S defense event log: %v", err) })
	stopA2SEventLogger := startA2SEventLogger(context.Background(), a2sEventLogger)
	life := lifecycle.New(db, engine, portChecker, cfg.DataRoot, lifecycle.WithHealth(healthChecker), lifecycle.WithSpace(disk.Checker{}, startMinimumFreeBytes), lifecycle.WithProvisioner(instanceProvisioner), lifecycle.WithMaintenanceGate(sharedGate), lifecycle.WithLogPreparer(gameLogManager), lifecycle.WithDefenseGate(a2sDefenseCoordinator), lifecycle.WithAccelerator(acceleratorManager))
	if recoverErr := instanceProvisioner.RecoverOverlays(context.Background()); recoverErr != nil {
		log.Printf("container reconciliation deferred: recover overlays: %v", recoverErr)
	} else if unknown, reconcileErr := life.Reconcile(context.Background()); reconcileErr != nil {
		log.Printf("container reconciliation deferred: %v", reconcileErr)
	} else if len(unknown) > 0 {
		log.Printf("found %d unclaimed managed containers", len(unknown))
	}
	jobManager := jobs.NewPersistentManager(db, jobs.WithLogSink(jobLogManager))
	gameLogCleaner := gamelogs.NewCleaner(db, gameLogManager)
	playerService := players.NewService(db, a2s.Client{}, engine, cfg.GameHost)
	vpkRestartCoordinator := vpkrestart.New(db, playerService, life, jobManager)
	vpkRestartCoordinator.Start(context.Background())
	performanceSampler := metrics.New(db, engine, trafficClient, playerService, nil).WithStorage(metrics.DirectoryStorage{Root: cfg.DataRoot})
	uploadManager, err := content.NewUploadManager(cfg.DataRoot)
	if err != nil {
		log.Fatal(err)
	}
	selfServiceVPKManager := content.NewSelfServiceVPKManager(db, uploadManager)
	selfServiceVPKScheduler := content.NewSelfServiceVPKScheduler(selfServiceVPKManager)
	if err := selfServiceVPKScheduler.Start(); err != nil {
		log.Fatal(err)
	}
	privateManager := content.NewPrivateManager(cfg.DataRoot, 1<<20)
	privateUploadManager := content.NewPrivateUploadManager(cfg.DataRoot, 2<<30)
	if err := privateUploadManager.RecoverAll(); err != nil {
		log.Printf("private upload recovery: %v", err)
	}
	updateCoordinator := &updates.Coordinator{Lifecycle: life, Deployer: updatePipeline, Instances: db, Accelerator: acceleratorManager}
	releaseClient := releases.Client{
		ReleaseDownloadAcceleratorProvider: func(context.Context) (string, error) {
			return db.GitHubReleasesAccelerator()
		},
	}
	sourceSynchronizer := releases.Synchronizer{Client: releaseClient, Sources: db, Packages: packageManager, Secrets: secretService}
	gameCoordinator := &updates.GameCoordinator{Root: cfg.DataRoot, Instances: db, Lifecycle: life, Updater: engine, Private: privateManager, Packages: packageManager, Sources: sourceSynchronizer, Deployer: updatePipeline, Accelerator: acceleratorManager}
	sharedPublisher := updates.FilesystemGamePublisher{Root: cfg.DataRoot}
	sharedRebuilder := updates.SharedGameRebuilder{Overlay: overlayClient, Packages: packageManager, Sources: db, Deployer: updatePipeline, Private: privateManager, Accelerator: acceleratorManager}
	sharedGameCoordinator := &updates.SharedGameCoordinator{Root: cfg.DataRoot, Instances: db, Players: playerService, Installer: engine, Reconciler: sharedRebuilder, Lifecycle: life, Gate: sharedGate}
	sharedGameMigration := &sharedmigration.SharedGameService{Root: cfg.DataRoot, Instances: db, Installer: engine, Publisher: sharedPublisher, Layout: sharedmigration.FilesystemLayout{Root: cfg.DataRoot}, Reconciler: sharedRebuilder, Gate: sharedGate}
	dispatcher := automation.Dispatcher{Jobs: jobManager, Players: playerService, Packages: packageManager, PackagesUpdate: updateCoordinator, GameUpdate: gameCoordinator, SharedGameUpdate: sharedGameCoordinator, Releases: releaseClient, Sources: db, Instances: db, Maintenance: maintenance.New(cfg.DataRoot, maintenance.WithPackageCleanup(db, packageManager)), GameLogs: gameLogCleaner, CrashReports: crashReportManager, Gate: sharedGate, Secrets: secretService}
	scheduleService := scheduler.NewService(db, dispatcher)
	secureCookie := true
	if configured := os.Getenv("L4D2_PANEL_SECURE_COOKIE"); configured != "" {
		secureCookie, err = strconv.ParseBool(configured)
		if err != nil {
			log.Fatal("L4D2_PANEL_SECURE_COOKIE must be true or false")
		}
	}
	api := httpapi.New(db, sessions, httpapi.WithGameLogs(gameLogManager), httpapi.WithOperations(life, jobManager), httpapi.WithMaintenanceGate(sharedGate), httpapi.WithJobLogs(jobLogManager), httpapi.WithConsole(engine), httpapi.WithPlayers(playerService), httpapi.WithContent(uploadManager, privateManager, packageManager, updatePipeline, updateCoordinator), httpapi.WithReleases(releaseClient), httpapi.WithSelfServiceVPK(selfServiceVPKManager), httpapi.WithSelfServiceVPKKey(secretKey), httpapi.WithVPKRestartRegistrar(vpkRestartCoordinator), httpapi.WithPrivateUploads(privateUploadManager), httpapi.WithGameUpdates(gameCoordinator), httpapi.WithSharedGameUpdates(sharedGameCoordinator), httpapi.WithSharedGameMigration(sharedGameMigration), httpapi.WithSharedGamePath(cfg.GameCurrentPath), httpapi.WithScheduler(scheduleService), httpapi.WithSecrets(secretService), httpapi.WithResources(engine), httpapi.WithPerformance(performanceSampler), httpapi.WithSystem(engine), httpapi.WithA2SDefenseMutations(a2sDefenseCoordinator), httpapi.WithA2SDefenseSettings(a2sDefenseCoordinator), httpapi.WithCrashReports(crashReportManager), httpapi.WithCrashAnalysis(analysisWorker), httpapi.WithSecureCookie(secureCookie))
	stopBackground := func() {
		if err := analysisWorker.Stop(context.Background()); err != nil {
			log.Printf("stop crash analysis worker: %v", err)
		}
		stopCrashSymbolIndexer()
		stopA2SEventLogger()
		a2sDefenseCoordinator.Stop()
		vpkRestartCoordinator.Stop()
		scheduleService.Stop()
		selfServiceVPKScheduler.Stop()
	}
	mux := http.NewServeMux()
	mux.Handle("/api/", api.Handler())
	mux.Handle("/submit", api.AcceleratorSubmitHandler())
	mux.Handle("/symbols/submit", api.AcceleratorSymbolHandler())
	mux.Handle("/binary/submit", api.AcceleratorBinaryHandler())
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
	performanceSampler.Start(context.Background())
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
		shutdownErr := shutdownPanel(drain, server, stopBackground, performanceSampler, jobManager)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("panel server stopped: %v", errors.Join(err, shutdownErr))
		} else if shutdownErr != nil {
			log.Printf("panel drain failed: %v", shutdownErr)
		}
	case received := <-signals:
		log.Printf("received %s; draining panel", received)
		drain, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := shutdownPanel(drain, server, stopBackground, performanceSampler, jobManager); err != nil {
			log.Printf("panel shutdown incomplete: %v", err)
		}
	}
}

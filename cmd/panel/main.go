package main

import (
	"context"
	"github.com/not0721here/l4d2-control-panel/internal/auth"
	"github.com/not0721here/l4d2-control-panel/internal/config"
	"github.com/not0721here/l4d2-control-panel/internal/docker"
	"github.com/not0721here/l4d2-control-panel/internal/httpapi"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
	"github.com/not0721here/l4d2-control-panel/internal/lifecycle"
	"github.com/not0721here/l4d2-control-panel/internal/ports"
	"github.com/not0721here/l4d2-control-panel/internal/store"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

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
	dockerHost := os.Getenv("DOCKER_HOST")
	if dockerHost == "" {
		dockerHost = "http://127.0.0.1:23750"
	}
	engine := docker.NewEngine(dockerHost)
	portChecker := ports.Checker{Configured: func() []int { return nil }, Listening: ports.IsListening}
	life := lifecycle.New(db, engine, portChecker, cfg.DataRoot)
	if unknown, reconcileErr := life.Reconcile(context.Background()); reconcileErr != nil {
		log.Printf("container reconciliation deferred: %v", reconcileErr)
	} else if len(unknown) > 0 {
		log.Printf("found %d unclaimed managed containers", len(unknown))
	}
	jobManager := jobs.NewPersistentManager(db)
	api := httpapi.New(db, sessions, httpapi.WithOperations(life, jobManager))
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
	log.Fatal(server.ListenAndServe())
}

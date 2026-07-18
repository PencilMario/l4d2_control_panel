//go:build e2e

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/not0721here/l4d2-control-panel/internal/auth"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/gamelogs"
	"github.com/not0721here/l4d2-control-panel/internal/httpapi"
	"github.com/not0721here/l4d2-control-panel/internal/store"
	"github.com/not0721here/l4d2-control-panel/internal/updates"
)

func TestFixtureCleanupJobSummaryIsAvailableOverHTTP(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	jobManager, logManager, err := newFixtureJobServices(root, db)
	if err != nil {
		t.Fatal(err)
	}
	defer logManager.Close()

	id := uuid.NewString()
	if err := db.CreateInstance(context.Background(), domain.Instance{ID: id, NodeID: "local", Name: "Cleanup logs", GamePort: 27015}); err != nil {
		t.Fatal(err)
	}
	if err := seedGameLogs(root, id); err != nil {
		t.Fatal(err)
	}
	scheduler := gamelogs.NewScheduler(db, jobManager, gamelogs.NewManager(root, gamelogs.Options{}))
	result := scheduler.EnqueueAll(context.Background())
	if result.Queued != 1 || len(result.JobIDs) != 1 {
		t.Fatalf("enqueue result=%+v", result)
	}
	jobID := result.JobIDs[0]
	deadline := time.Now().Add(5 * time.Second)
	for {
		job, ok := jobManager.Get(jobID)
		if ok && job.Status == "succeeded" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("cleanup job did not succeed: job=%+v ok=%v", job, ok)
		}
		time.Sleep(10 * time.Millisecond)
	}

	sessions := auth.NewService()
	if err := sessions.Bootstrap(fixturePassword); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(db, sessions, httpapi.WithOperations(nil, jobManager), httpapi.WithJobLogs(logManager)).Handler())
	defer server.Close()
	client := server.Client()
	login, err := client.Post(server.URL+"/api/auth/login", "application/json", strings.NewReader(`{"password":"correct horse battery staple"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer login.Body.Close()
	if login.StatusCode != http.StatusOK || len(login.Cookies()) == 0 {
		t.Fatalf("login status=%d", login.StatusCode)
	}
	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/jobs/"+jobID+"/logs", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(login.Cookies()[0])
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !bytes.Contains(body, []byte("Scanned=")) || !bytes.Contains(body, []byte("Deleted=")) || !bytes.Contains(body, []byte("ReleasedBytes=")) {
		t.Fatalf("logs status=%d body=%s", response.StatusCode, body)
	}
}

func TestPrivateLowerDiagnosticRejectsInstanceTraversal(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside", "game", "left4dead2", "secret.txt")
	if err := os.MkdirAll(filepath.Dir(outside), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("outside sentinel"), 0640); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/__e2e/private-lower?id=../outside&path=secret.txt", nil)
	response := httptest.NewRecorder()
	privateLowerDiagnostic(root).ServeHTTP(response, request)
	if response.Code < 400 || response.Code >= 500 {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if raw, err := os.ReadFile(outside); err != nil || string(raw) != "outside sentinel" {
		t.Fatalf("outside=%q err=%v", raw, err)
	}
}

func TestFixturePerformanceEndpointsExposeDeterministicHTTPContract(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	sessions := auth.NewService()
	if err := sessions.Bootstrap(fixturePassword); err != nil {
		t.Fatal(err)
	}
	instance := domain.Instance{ID: "fixture-instance", NodeID: "local", Name: "Fixture", ContainerID: "fixture-container", GamePort: 27015, StartMap: "c2m1_highway", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", DesiredState: domain.StateRunning, ActualState: domain.StateStopped}
	if err := db.CreateInstance(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	server := httpapi.New(db, sessions, httpapi.WithPerformance(fixturePerformance{}))

	login := httptest.NewRecorder()
	server.Handler().ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"password":"correct horse battery staple"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
	}
	cookie := login.Result().Cookies()[0]

	overview := httptest.NewRecorder()
	overviewRequest := httptest.NewRequest(http.MethodGet, "/api/instances/fixture-instance/overview", nil)
	overviewRequest.AddCookie(cookie)
	server.Handler().ServeHTTP(overview, overviewRequest)
	if overview.Code != http.StatusOK || !strings.Contains(overview.Body.String(), `"sampled_at":"2026-07-15T12:00:10Z"`) || !strings.Contains(overview.Body.String(), `"network_rx_bytes_per_sec":128`) || !strings.Contains(overview.Body.String(), `"map":"c2m1_highway"`) {
		t.Fatalf("overview status=%d body=%s", overview.Code, overview.Body.String())
	}

	history := httptest.NewRecorder()
	historyRequest := httptest.NewRequest(http.MethodGet, "/api/instances/fixture-instance/performance-history", nil)
	historyRequest.AddCookie(cookie)
	server.Handler().ServeHTTP(history, historyRequest)
	var points []map[string]any
	if err := json.Unmarshal(history.Body.Bytes(), &points); err != nil {
		t.Fatal(err)
	}
	if history.Code != http.StatusOK || len(points) != 2 || points[0]["network_rx_bytes_per_sec"] != nil || points[1]["network_rx_bytes_per_sec"] != float64(0) {
		t.Fatalf("history status=%d body=%s", history.Code, history.Body.String())
	}
}

func TestFixtureStartupRecoversInterruptedPackageDeployment(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "instances", "fixture", "game", "left4dead2", "cfg", "plugin.cfg")
	if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old"), 0640); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(root, "package.zip")
	writeFixturePackage(t, archivePath, "cfg/plugin.cfg", "new")

	pipeline := updates.New(root)
	deployment, err := pipeline.Begin(context.Background(), "fixture", archivePath, "v2", updates.Full)
	if err != nil {
		t.Fatal(err)
	}
	_ = deployment
	if raw, err := os.ReadFile(target); err != nil || string(raw) != "new" {
		t.Fatalf("deployed content=%q err=%v", raw, err)
	}

	if _, err := newFixturePipeline(root); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(target); err != nil || string(raw) != "old" {
		t.Fatalf("recovered content=%q err=%v", raw, err)
	}
	if journals, err := filepath.Glob(filepath.Join(root, "instances", "fixture", "backups", "update-*", "journal.json")); err != nil || len(journals) != 0 {
		t.Fatalf("journals=%v err=%v", journals, err)
	}
}

func TestFixtureLifecycleDoesNotReseedPersistentGameLogs(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id := uuid.NewString()
	instance := domain.Instance{ID: id, NodeID: "local", Name: "Logs", GamePort: 27015}
	if err := db.CreateInstance(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	seed := httptest.NewRecorder()
	seedGameLogsControl(root, db).ServeHTTP(seed, httptest.NewRequest(http.MethodGet, "/__e2e/seed-game-logs?id="+id, nil))
	if seed.Code != http.StatusNoContent {
		t.Fatalf("seed status=%d body=%q", seed.Code, seed.Body.String())
	}
	if err := (&fixtureLifecycle{db: db, root: root}).Start(context.Background(), id); err != nil {
		t.Fatal(err)
	}

	recent := filepath.Join(root, "instances", id, "logs", "game", "server.log")
	aged := filepath.Join(root, "instances", id, "logs", "sourcemod", "errors", "aged-error.log")
	for _, path := range []string{recent, aged} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("seeded log %s: %v", path, err)
		}
	}
	info, err := os.Stat(aged)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(info.ModTime()) < 20*24*time.Hour {
		t.Fatalf("aged log mtime=%s", info.ModTime())
	}
	content, err := os.ReadFile(recent)
	if err != nil || !strings.Contains(string(content), "ERROR") || !strings.Contains(string(content), "instance="+id) {
		t.Fatalf("recent content=%q err=%v", content, err)
	}
	fixedModified := time.Date(2026, 7, 18, 12, 34, 56, 0, time.UTC)
	if err := os.Chtimes(recent, fixedModified, fixedModified); err != nil {
		t.Fatal(err)
	}
	if err := (&fixtureLifecycle{db: db, root: root}).Rebuild(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	afterRebuild, err := os.ReadFile(recent)
	if err != nil || !bytes.Equal(afterRebuild, content) {
		t.Fatalf("rebuild content=%q want=%q err=%v", afterRebuild, content, err)
	}
	rebuiltInfo, err := os.Stat(recent)
	if err != nil {
		t.Fatal(err)
	}
	if !rebuiltInfo.ModTime().Equal(fixedModified) {
		t.Fatalf("rebuild mtime=%v want=%v", rebuiltInfo.ModTime(), fixedModified)
	}
	if err := os.Remove(aged); err != nil {
		t.Fatal(err)
	}
	if err := (&fixtureGameUpdater{}).UpdateGame(context.Background(), id, instance); err != nil {
		t.Fatal(err)
	}
	if err := (&fixtureLifecycle{db: db, root: root}).Rebuild(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(aged); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rebuild unexpectedly restored deleted log: %v", err)
	}
}

func TestSeedGameLogsControlValidatesInstanceAndSeedsFiles(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := seedGameLogsControl(root, db)

	for _, target := range []string{
		"/__e2e/seed-game-logs",
		"/__e2e/seed-game-logs?id=../outside",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("target=%q status=%d body=%q", target, response.Code, response.Body.String())
		}
	}

	missingID := uuid.NewString()
	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/__e2e/seed-game-logs?id="+missingID, nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status=%d body=%q", missing.Code, missing.Body.String())
	}

	id := uuid.NewString()
	instance := domain.Instance{ID: id, NodeID: "local", Name: "Logs", GamePort: 27015}
	if err := db.CreateInstance(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/__e2e/seed-game-logs?id="+id, nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}

	paths := []string{
		filepath.Join(root, "instances", id, "logs", "game", "server.log"),
		filepath.Join(root, "instances", id, "logs", "sourcemod", "errors", "current-error.log"),
		filepath.Join(root, "instances", id, "logs", "sourcemod", "errors", "aged-error.log"),
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("seeded log %s: %v", path, err)
		}
	}
	agedInfo, err := os.Stat(paths[2])
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(agedInfo.ModTime()) < 20*24*time.Hour {
		t.Fatalf("aged log mtime=%s", agedInfo.ModTime())
	}
}

func writeFixturePackage(t *testing.T, path, name, content string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

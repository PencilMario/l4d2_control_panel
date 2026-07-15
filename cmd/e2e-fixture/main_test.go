//go:build e2e

package main

import (
	"archive/zip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/not0721here/l4d2-control-panel/internal/updates"
)

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

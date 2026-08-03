package releases

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFetchLatestSelectsAssetAndStoresPackage(t *testing.T) {
	raw := packageBytes()
	downloads := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			fmt.Fprintf(w, `{"tag_name":"v2.0","assets":[{"name":"plugins.zip","browser_download_url":%q}]}`, server.URL+"/plugins.zip")
		case "/plugins.zip":
			downloads++
			w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
			_, _ = w.Write(raw)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	manager, _ := content.NewPackageManager(t.TempDir())
	client := Client{BaseURL: server.URL, HTTP: server.Client(), MaxBytes: 1 << 20}
	result, err := client.FetchLatest(context.Background(), "owner/repo", `^plugins\.zip$`, "secret", manager)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.Package.Version != "v2.0" || result.Package.Filename != "plugins.zip" || result.Package.SourceRepository != "owner/repo" {
		t.Fatalf("result=%#v", result)
	}
	second, err := client.FetchLatest(context.Background(), "owner/repo", `^plugins\.zip$`, "secret", manager)
	if err != nil || second.Updated || second.Package.ID != result.Package.ID || downloads != 1 {
		t.Fatalf("second=%#v downloads=%d err=%v", second, downloads, err)
	}
}

func TestFetchLatestUsesConfiguredReleaseDownloadAccelerator(t *testing.T) {
	raw := packageBytes()
	var downloadedPath, authorization string
	accelerator := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloadedPath = r.URL.EscapedPath()
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
		_, _ = w.Write(raw)
	}))
	defer accelerator.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/releases/latest" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"tag_name":"v2.0","assets":[{"name":"plugins.zip","browser_download_url":"https://github.com/owner/repo/releases/download/v2.0/plugins.zip"}]}`)
	}))
	defer api.Close()
	manager, _ := content.NewPackageManager(t.TempDir())
	client := Client{BaseURL: api.URL, HTTP: accelerator.Client(), MaxBytes: 1 << 20, ReleaseDownloadAccelerator: accelerator.URL + "/"}

	result, err := client.FetchLatest(context.Background(), "owner/repo", `^plugins\.zip$`, "secret", manager)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated {
		t.Fatalf("result=%#v", result)
	}
	wantPath := "/https://github.com/owner/repo/releases/download/v2.0/plugins.zip"
	if downloadedPath != wantPath {
		t.Fatalf("downloadedPath = %q, want %q", downloadedPath, wantPath)
	}
	if authorization != "" {
		t.Fatalf("Authorization header was forwarded to accelerator: %q", authorization)
	}
}

func TestFetchLatestReadsReleaseDownloadAcceleratorFromProvider(t *testing.T) {
	raw := packageBytes()
	accelerator := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
		_, _ = w.Write(raw)
	}))
	defer accelerator.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/releases/latest" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, "{\"tag_name\":\"v3.0\",\"assets\":[{\"name\":\"plugins.zip\",\"browser_download_url\":\"https://github.com/owner/repo/releases/download/v3.0/plugins.zip\"}]}")
	}))
	defer api.Close()
	manager, _ := content.NewPackageManager(t.TempDir())
	configured := accelerator.URL + "/"
	client := Client{
		BaseURL: api.URL, HTTP: accelerator.Client(), MaxBytes: 1 << 20,
		ReleaseDownloadAcceleratorProvider: func(context.Context) (string, error) { return configured, nil },
	}
	result, err := client.FetchLatest(context.Background(), "owner/repo", "^plugins[.]zip$", "secret", manager)
	if err != nil || !result.Updated {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestFetchLatestRetainsPreviousReleaseUntilMaintenanceCleanup(t *testing.T) {
	raw := packageBytes()
	tag := "v1.0"
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			fmt.Fprintf(w, `{"tag_name":%q,"assets":[{"name":"plugins.zip","browser_download_url":%q}]}`, tag, server.URL+"/plugins.zip")
		case "/plugins.zip":
			w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
			_, _ = w.Write(raw)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	manager, _ := content.NewPackageManager(t.TempDir())
	client := Client{BaseURL: server.URL, HTTP: server.Client(), MaxBytes: 1 << 20}
	first, err := client.FetchLatest(context.Background(), "owner/repo", `^plugins\.zip$`, "", manager)
	if err != nil {
		t.Fatal(err)
	}
	tag = "v2.0"
	second, err := client.FetchLatest(context.Background(), "owner/repo", `^plugins\.zip$`, "", manager)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Updated || !second.Updated || first.Package.ID == second.Package.ID {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
	for _, item := range []content.PackageVersion{first.Package, second.Package} {
		if _, err := manager.Get(item.ID); err != nil {
			t.Fatalf("synchronized package %s removed before maintenance: %v", item.ID, err)
		}
	}
}

func TestInterruptedReleaseDownloadUsesManagedTemporaryArtifact(t *testing.T) {
	root := t.TempDir()
	manager, _ := content.NewPackageManager(root)
	assetStarted := make(chan struct{})
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			fmt.Fprintf(w, `{"tag_name":"v2.0","assets":[{"name":"plugins.zip","browser_download_url":%q}]}`, server.URL+"/plugins.zip")
		case "/plugins.zip":
			_, _ = w.Write([]byte("partial"))
			w.(http.Flusher).Flush()
			close(assetStarted)
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := (Client{BaseURL: server.URL, HTTP: server.Client(), MaxBytes: 1 << 20}).FetchLatest(ctx, "owner/repo", `^plugins\.zip$`, "", manager)
		done <- err
	}()
	<-assetStarted
	uploadDir := filepath.Join(root, "packages", "uploads")
	foundManagedPart := false
	var entries []os.DirEntry
	var readErr error
	deadline := time.Now().Add(time.Second)
	for !foundManagedPart && time.Now().Before(deadline) {
		entries, readErr = os.ReadDir(uploadDir)
		for _, entry := range entries {
			foundManagedPart = foundManagedPart || strings.HasSuffix(entry.Name(), ".part")
		}
		if !foundManagedPart {
			time.Sleep(10 * time.Millisecond)
		}
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("interrupted download unexpectedly succeeded")
	}
	if readErr != nil || !foundManagedPart {
		t.Fatalf("download was not staged below %s: entries=%v err=%v", uploadDir, entries, readErr)
	}
	entries, err := os.ReadDir(uploadDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary downloads were not cleaned: %v", entries)
	}
}

func TestDefaultClientAllowsThreeHoursForLargeReleaseDownloads(t *testing.T) {
	if timeout := (Client{}).httpClient().Timeout; timeout != 3*time.Hour {
		t.Fatalf("timeout=%s, want 3h", timeout)
	}
}

type releaseLogSink struct {
	mu       sync.Mutex
	messages []string
}

func (s *releaseLogSink) Append(_ context.Context, _, _ string, _ joblogs.Level, message string) (joblogs.Record, error) {
	s.mu.Lock()
	s.messages = append(s.messages, message)
	s.mu.Unlock()
	return joblogs.Record{}, nil
}

func (*releaseLogSink) Finalize(context.Context, string) error { return nil }

func (s *releaseLogSink) joined() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.messages, "\n")
}

func TestFetchLatestLogsReleaseAssetDownloadAndReuseWithoutCredentials(t *testing.T) {
	raw := packageBytes()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			fmt.Fprintf(w, `{"name":"July Stable","tag_name":"v2.0","published_at":"2026-07-29T12:00:00Z","assets":[{"name":"plugins.zip","size":%d,"browser_download_url":%q}]}`, len(raw), server.URL+"/plugins.zip?signature=do-not-log")
		case "/plugins.zip":
			w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
			_, _ = w.Write(raw)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	manager, _ := content.NewPackageManager(t.TempDir())
	client := Client{BaseURL: server.URL, HTTP: server.Client(), MaxBytes: 1 << 20}
	sink := &releaseLogSink{}
	jobManager := jobs.NewManager(jobs.WithLogSink(sink))

	for range 2 {
		if _, err := jobManager.Start(context.Background(), "global", "release_fetch", func(ctx context.Context, _ jobs.Reporter) error {
			_, err := client.FetchLatest(ctx, "owner/repo", `^plugins\.zip$`, "ghp_do_not_log", manager)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if err := jobManager.Wait(context.Background()); err != nil {
			t.Fatal(err)
		}
	}

	logs := sink.joined()
	for _, want := range []string{"repository=owner/repo", `release="July Stable"`, "tag=v2.0", "published=2026-07-29T12:00:00Z", "asset=plugins.zip", fmt.Sprintf("advertised_size=%d", len(raw)), "download completed", "package downloaded", "package reused"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("logs=%q missing %q", logs, want)
		}
	}
	for _, secret := range []string{"ghp_do_not_log", "do-not-log", "signature="} {
		if strings.Contains(logs, secret) {
			t.Fatalf("logs leaked %q: %q", secret, logs)
		}
	}
}

func packageBytes() []byte {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, _ := writer.Create("cfg/plugin.cfg")
	_, _ = entry.Write([]byte("x"))
	_ = writer.Close()
	return buffer.Bytes()
}

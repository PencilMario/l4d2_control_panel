package releases

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type httpTestDownloader struct{ client *http.Client }

func (d httpTestDownloader) Download(ctx context.Context, request DownloadRequest) (int64, error) {
	response, err := d.client.Do(mustRequest(ctx, request.URL))
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	file, err := os.Create(request.Destination)
	if err != nil {
		return 0, err
	}
	written, copyErr := io.Copy(file, response.Body)
	closeErr := file.Close()
	if copyErr != nil {
		return 0, copyErr
	}
	return written, closeErr
}

func mustRequest(ctx context.Context, rawURL string) *http.Request {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		panic(err)
	}
	return request
}

type recordingDownloader struct {
	request DownloadRequest
	raw     []byte
}

func (d *recordingDownloader) Download(_ context.Context, request DownloadRequest) (int64, error) {
	d.request = request
	if err := os.WriteFile(request.Destination, d.raw, 0600); err != nil {
		return 0, err
	}
	return int64(len(d.raw)), nil
}

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
	client := Client{BaseURL: server.URL, HTTP: server.Client(), MaxBytes: 1 << 20, Downloader: httpTestDownloader{client: server.Client()}}
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
	client := Client{BaseURL: server.URL, HTTP: server.Client(), MaxBytes: 1 << 20, Downloader: httpTestDownloader{client: server.Client()}}
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
		_, err := (Client{BaseURL: server.URL, HTTP: server.Client(), MaxBytes: 1 << 20, Downloader: httpTestDownloader{client: server.Client()}}).FetchLatest(ctx, "owner/repo", `^plugins\.zip$`, "", manager)
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

func TestDefaultClientAllowsLargeReleaseDownloads(t *testing.T) {
	if timeout := (Client{}).httpClient().Timeout; timeout < 10*time.Minute {
		t.Fatalf("timeout=%s", timeout)
	}
}

func TestClientTrustsGitHubReleaseAssetHost(t *testing.T) {
	asset, err := url.Parse("https://release-assets.githubusercontent.com/github-production-release-asset/file.zip")
	if err != nil {
		t.Fatal(err)
	}
	if !(Client{}).allowedAssetHost(asset, "https://api.github.com") {
		t.Fatal("GitHub release asset host was rejected")
	}
}

func TestClientRejectsAssetWithUnexpectedScheme(t *testing.T) {
	asset, err := url.Parse("ftp://api.github.com/plugins.zip")
	if err != nil {
		t.Fatal(err)
	}
	if (Client{}).allowedAssetHost(asset, "https://api.github.com") {
		t.Fatal("unexpected asset scheme was trusted")
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
	client := Client{BaseURL: server.URL, HTTP: server.Client(), MaxBytes: 1 << 20, Downloader: httpTestDownloader{client: server.Client()}}
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

func TestFetchLatestExchangesTokenForTrustedAssetRedirectBeforeDownload(t *testing.T) {
	raw := packageBytes()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			fmt.Fprintf(w, `{"tag_name":"v3","assets":[{"name":"plugins.zip","url":%q,"browser_download_url":%q}]}`, server.URL+"/assets/1", server.URL+"/browser")
		case "/assets/1":
			if r.Header.Get("Authorization") != "Bearer private-token" || r.Header.Get("Accept") != "application/octet-stream" {
				t.Errorf("asset headers=%v", r.Header)
			}
			w.Header().Set("Location", server.URL+"/signed/plugins.zip?signature=secret")
			w.WriteHeader(http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	manager, _ := content.NewPackageManager(t.TempDir())
	downloader := &recordingDownloader{raw: raw}
	client := Client{BaseURL: server.URL, HTTP: server.Client(), MaxBytes: 1 << 20, Downloader: downloader}
	result, err := client.FetchLatest(context.Background(), "owner/repo", `^plugins\.zip$`, "private-token", manager)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || downloader.request.URL != server.URL+"/signed/plugins.zip?signature=secret" {
		t.Fatalf("result=%+v download=%+v", result, downloader.request)
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

package releases

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFetchLatestSelectsAssetAndStoresPackage(t *testing.T) {
	raw := packageBytes()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/latest":
			fmt.Fprintf(w, `{"tag_name":"v2.0","assets":[{"name":"plugins.zip","browser_download_url":%q}]}`, server.URL+"/plugins.zip")
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
	item, err := client.FetchLatest(context.Background(), "owner/repo", `^plugins\.zip$`, "secret", manager)
	if err != nil {
		t.Fatal(err)
	}
	if item.Version != "v2.0" || item.Filename != "plugins.zip" {
		t.Fatalf("item=%#v", item)
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
func packageBytes() []byte {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, _ := writer.Create("cfg/plugin.cfg")
	_, _ = entry.Write([]byte("x"))
	_ = writer.Close()
	return buffer.Bytes()
}

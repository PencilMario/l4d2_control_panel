package releases

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
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
func packageBytes() []byte {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, _ := writer.Create("cfg/plugin.cfg")
	_, _ = entry.Write([]byte("x"))
	_ = writer.Close()
	return buffer.Bytes()
}

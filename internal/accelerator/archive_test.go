package accelerator

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

type testZipEntry struct {
	name string
	data string
	mode os.FileMode
}

func TestResolveDownloadURLOnlyAppliesProxyToGitHub(t *testing.T) {
	proxy := "https://proxy.example.test/"
	github := "https://github.com/owner/accelerator/releases/download/v1/accelerator.zip"
	got, err := resolveDownloadURL(github, proxy)
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.TrimRight(proxy, "/") + "/" + github; got != want {
		t.Fatalf("github URL=%q want=%q", got, want)
	}
	other := "https://objects.githubusercontent.com/accelerator.zip"
	got, err = resolveDownloadURL(other, proxy)
	if err != nil || got != other {
		t.Fatalf("other URL=%q err=%v", got, err)
	}
	httpURL := "http://downloads.example.test/accelerator.zip"
	got, err = resolveDownloadURL(httpURL, proxy)
	if err != nil || got != httpURL {
		t.Fatalf("HTTP URL=%q err=%v", got, err)
	}
	for _, value := range []string{"ftp://github.com/owner/accelerator.zip", "https://"} {
		if _, err := resolveDownloadURL(value, proxy); err == nil {
			t.Fatalf("accepted invalid URL %q", value)
		}
	}
	if _, err := resolveDownloadURL(github, "http://proxy.example.test/"); err == nil {
		t.Fatal("accepted invalid proxy URL")
	}
}

func TestDownloadArchiveCachesByResolvedURLAndHash(t *testing.T) {
	archive := []byte("fake accelerator archive")
	digest := sha256.Sum256(archive)
	wantHash := hex.EncodeToString(digest[:])
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	manager := newArchiveTestManager(t)
	manager.httpClient = server.Client()
	first, err := manager.downloadArchive(context.Background(), server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.downloadArchive(context.Background(), server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 != wantHash || second.SHA256 != wantHash || first.Path != second.Path || requests.Load() != 1 {
		t.Fatalf("first=%+v second=%+v requests=%d", first, second, requests.Load())
	}
	assertArchiveFile(t, first.Path, archive)
	if info, err := os.Stat(first.Path); err != nil || info.Mode().Perm() != 0o600 {
		if err != nil || runtime.GOOS != "windows" {
			t.Fatalf("cache mode info=%v err=%v", info, err)
		}
	}
}

func TestDownloadArchiveRejectsHTTPStatusAndDeclaredSize(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Length", strconv.FormatInt(MaxArchiveBytes+1, 10))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	manager := newArchiveTestManager(t)
	manager.httpClient = server.Client()
	if _, err := manager.downloadArchive(context.Background(), server.URL+"/status", ""); err == nil {
		t.Fatal("accepted failed HTTP status")
	}
	if _, err := manager.downloadArchive(context.Background(), server.URL+"/size", ""); err == nil {
		t.Fatal("accepted oversized archive")
	}
}

func TestValidateArchiveAcceptsTargetAcceleratorPackage(t *testing.T) {
	archive := makeTestArchive(t, []testZipEntry{
		{name: "addons/sourcemod/extensions/accelerator.autoload", data: "accelerator.ext"},
		{name: "addons/sourcemod/extensions/x64/accelerator.ext.so", data: "binary"},
		{name: "addons/sourcemod/gamedata/accelerator.games.txt", data: "gamedata"},
		{name: "addons/sourcemod/scripting/include/accelerator.inc", data: "include"},
	})
	if _, err := validateArchive(archive, "linux", "x86_64"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateArchiveRejectsUnsafeOrIncompletePackages(t *testing.T) {
	valid := []testZipEntry{
		{name: "addons/sourcemod/extensions/accelerator.autoload", data: "accelerator.ext"},
		{name: "addons/sourcemod/extensions/x64/accelerator.ext.so", data: "binary"},
		{name: "addons/sourcemod/gamedata/accelerator.games.txt", data: "gamedata"},
	}
	for _, test := range []struct {
		name    string
		entries []testZipEntry
	}{
		{name: "absolute", entries: append(append([]testZipEntry{}, valid...), testZipEntry{name: "/addons/sourcemod/plugins/escape.smx", data: "x"})},
		{name: "dot dot", entries: append(append([]testZipEntry{}, valid...), testZipEntry{name: "addons/sourcemod/../../private/escape", data: "x"})},
		{name: "backslash dot dot", entries: append(append([]testZipEntry{}, valid...), testZipEntry{name: `addons\sourcemod\..\..\private\escape`, data: "x"})},
		{name: "unknown path", entries: append(append([]testZipEntry{}, valid...), testZipEntry{name: "cfg/server.cfg", data: "x"})},
		{name: "symlink", entries: append(append([]testZipEntry{}, valid...), testZipEntry{name: "addons/sourcemod/plugins/link.smx", data: "target", mode: os.ModeSymlink})},
		{name: "wrong architecture", entries: []testZipEntry{
			{name: "addons/sourcemod/extensions/accelerator.autoload", data: "accelerator.ext"},
			{name: "addons/sourcemod/extensions/accelerator.ext.so", data: "binary"},
			{name: "addons/sourcemod/gamedata/accelerator.games.txt", data: "gamedata"},
		}},
		{name: "missing autoload", entries: valid[1:]},
		{name: "missing extension", entries: []testZipEntry{valid[0], valid[2]}},
		{name: "missing gamedata", entries: valid[:2]},
	} {
		t.Run(test.name, func(t *testing.T) {
			archive := makeTestArchive(t, test.entries)
			if _, err := validateArchive(archive, "linux", "x86_64"); err == nil {
				t.Fatal("accepted invalid Accelerator archive")
			}
		})
	}
}

func makeTestArchive(t *testing.T, entries []testZipEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "accelerator.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Store}
		if entry.mode != 0 {
			header.SetMode(entry.mode)
		}
		part, err := archive.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(part, bytes.NewBufferString(entry.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func newArchiveTestManager(t *testing.T) *Manager {
	t.Helper()
	manager, err := New(Config{InstancesRoot: t.TempDir(), CacheRoot: filepath.Join(t.TempDir(), "cache"), HTTPClient: &http.Client{}})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func assertArchiveFile(t *testing.T, path string, want []byte) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	got, err := io.ReadAll(file)
	if err != nil || string(got) != string(want) {
		t.Fatalf("archive=%q err=%v want=%q", got, err, want)
	}
}

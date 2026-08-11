package accelerator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

func TestBackupFileFallsBackAcrossFilesystems(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	backup := filepath.Join(root, "backup", "target")
	if err := os.WriteFile(target, []byte("payload"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := backupFile(target, backup, func(string, string) error { return syscall.EXDEV }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target still exists err=%v", err)
	}
	if got, err := os.ReadFile(backup); err != nil || string(got) != "payload" {
		t.Fatalf("backup=%q err=%v", got, err)
	}
}

func TestEnsureInstallsAcceleratorAndWritesManifest(t *testing.T) {
	root := t.TempDir()
	instanceRoot := filepath.Join(root, "instances", "instance-a")
	gameRoot := filepath.Join(instanceRoot, "game", "left4dead2")
	if err := os.MkdirAll(filepath.Join(gameRoot, "addons", "sourcemod", "configs"), 0o750); err != nil {
		t.Fatal(err)
	}
	originalCore := []byte(`// preserve comments
"Core"
{
	"CustomKey" "keep"
	"MinidumpUrl" "https://old.example/submit"
}
`)
	corePath := filepath.Join(gameRoot, "addons", "sourcemod", "configs", "core.cfg")
	if err := os.WriteFile(corePath, originalCore, 0o640); err != nil {
		t.Fatal(err)
	}
	preexistingGamedata := filepath.Join(gameRoot, "addons", "sourcemod", "gamedata", "accelerator.games.txt")
	if err := os.MkdirAll(filepath.Dir(preexistingGamedata), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(preexistingGamedata, []byte("legacy package gamedata"), 0o640); err != nil {
		t.Fatal(err)
	}
	archive := makeTestArchive(t, []testZipEntry{
		{name: "linux/", data: ""},
		{name: "linux/addons/", data: ""},
		{name: "linux/addons/sourcemod/", data: ""},
		{name: "linux/addons/sourcemod/extensions/", data: ""},
		{name: "linux/addons/sourcemod/extensions/accelerator.autoload", data: "autoload"},
		{name: "linux/addons/sourcemod/extensions/accelerator.ext.so", data: "legacy extension"},
		{name: "linux/addons/sourcemod/extensions/x64/", data: ""},
		{name: "linux/addons/sourcemod/extensions/x64/accelerator.ext.so", data: "extension"},
		{name: "linux/addons/sourcemod/gamedata/", data: ""},
		{name: "linux/addons/sourcemod/gamedata/accelerator.games.txt", data: "gamedata"},
	})
	archiveBytes, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	var downloads atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads.Add(1)
		_, _ = w.Write(archiveBytes)
	}))
	defer server.Close()
	manager, err := New(Config{
		InstancesRoot: filepath.Join(root, "instances"),
		CacheRoot:     filepath.Join(root, "cache"),
		DownloadURLProvider: func(context.Context) (string, error) {
			return server.URL, nil
		},
		Token:      "token+value",
		PanelPort:  9090,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	instance := domain.Instance{ID: "instance-a", AcceleratorEnabled: true}
	if err := manager.Ensure(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"addons/sourcemod/extensions/accelerator.autoload": "autoload",
		"addons/sourcemod/extensions/accelerator.ext.so":   "legacy extension",
		"addons/sourcemod/gamedata/accelerator.games.txt":  "gamedata",
	} {
		got, err := os.ReadFile(filepath.Join(gameRoot, filepath.FromSlash(path)))
		if err != nil || string(got) != want {
			t.Fatalf("installed %s=%q err=%v", path, got, err)
		}
	}
	patched, err := os.ReadFile(corePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"MinidumpUrl" "http://127.0.0.1:9090/submit?token=token%2Bvalue"`,
		`"MinidumpSymbolUrl" "http://127.0.0.1:9090/symbols/submit?token=token%2Bvalue"`,
		`"MinidumpBinaryUrl" "http://127.0.0.1:9090/binary/submit?token=token%2Bvalue"`,
		`"MinidumpBinaryUpload" "yes"`,
	} {
		if !strings.Contains(string(patched), want) {
			t.Fatalf("core.cfg missing %q:\n%s", want, patched)
		}
	}
	manifestRaw, err := os.ReadFile(filepath.Join(instanceRoot, "accelerator-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	if !manifest.Enabled || manifest.ArchiveSHA256 == "" || len(manifest.Files) != 3 || manifest.CoreConfigSHA256 == "" {
		t.Fatalf("manifest=%+v", manifest)
	}
	if !manifest.PreservedFiles["addons/sourcemod/gamedata/accelerator.games.txt"] {
		t.Fatalf("manifest preserved_files=%v", manifest.PreservedFiles)
	}
	if err := manager.Ensure(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	if downloads.Load() != 1 {
		t.Fatalf("downloads=%d want 1", downloads.Load())
	}
	if err := os.WriteFile(preexistingGamedata, []byte("package update"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := manager.Ensure(context.Background(), instance); err != nil {
		t.Fatalf("reinstall after package update: %v", err)
	}
	if got, err := os.ReadFile(corePath); err != nil || !bytes.Equal(got, patched) {
		t.Fatalf("idempotent core equal=%v err=%v", bytes.Equal(got, patched), err)
	}
	instance.AcceleratorEnabled = false
	if err := manager.Ensure(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(preexistingGamedata); err != nil || string(got) != "gamedata" {
		t.Fatalf("preserved gamedata=%q err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(gameRoot, "addons", "sourcemod", "extensions", "accelerator.autoload")); !os.IsNotExist(err) {
		t.Fatalf("autoload still exists err=%v", err)
	}
}

func TestEnsureBootstrapsStaticExtensionPackage(t *testing.T) {
	root := t.TempDir()
	gameRoot := filepath.Join(root, "instances", "instance-a", "game", "left4dead2")
	corePath := filepath.Join(gameRoot, "addons", "sourcemod", "configs", "core.cfg")
	if err := os.MkdirAll(filepath.Dir(corePath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(corePath, []byte("\"Core\" {}\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	archive := makeTestArchive(t, []testZipEntry{
		{name: "linux/addons/sourcemod/extensions/accelerator.ext.so", data: "static extension"},
	})
	archiveBytes, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(archiveBytes) }))
	defer server.Close()
	manager, err := New(Config{
		InstancesRoot: filepath.Join(root, "instances"), CacheRoot: filepath.Join(root, "cache"),
		DownloadURLProvider: func(context.Context) (string, error) { return server.URL, nil },
		Token:               "secret", PanelPort: 8080, HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Ensure(context.Background(), domain.Instance{ID: "instance-a", AcceleratorEnabled: true}); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(gameRoot, "addons", "sourcemod", "extensions", "accelerator.ext.so")); err != nil || string(got) != "static extension" {
		t.Fatalf("extension=%q err=%v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(gameRoot, "addons", "sourcemod", "extensions", "accelerator.autoload")); err != nil || len(got) != 0 {
		t.Fatalf("autoload=%q err=%v", got, err)
	}
	gamedata, err := os.ReadFile(filepath.Join(gameRoot, "addons", "sourcemod", "gamedata", "accelerator.games.txt"))
	if err != nil || !strings.Contains(string(gamedata), "left4dead2") {
		t.Fatalf("gamedata=%q err=%v", gamedata, err)
	}
}

func TestEnsureDisabledRestoresOnlyPanelOwnedChanges(t *testing.T) {
	root := t.TempDir()
	gameRoot := filepath.Join(root, "instances", "instance-a", "game", "left4dead2")
	coreDir := filepath.Join(gameRoot, "addons", "sourcemod", "configs")
	if err := os.MkdirAll(coreDir, 0o750); err != nil {
		t.Fatal(err)
	}
	original := []byte(`"Core" { "CustomKey" "keep" "MinidumpPresubmit" "no" }
`)
	corePath := filepath.Join(coreDir, "core.cfg")
	if err := os.WriteFile(corePath, original, 0o640); err != nil {
		t.Fatal(err)
	}
	archive := makeTestArchive(t, []testZipEntry{
		{name: "addons/sourcemod/extensions/accelerator.autoload", data: "autoload"},
		{name: "addons/sourcemod/extensions/accelerator.ext.so", data: "extension"},
		{name: "addons/sourcemod/gamedata/accelerator.games.txt", data: "gamedata"},
	})
	archiveBytes, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(archiveBytes) }))
	defer server.Close()
	manager, err := New(Config{
		InstancesRoot: filepath.Join(root, "instances"), CacheRoot: filepath.Join(root, "cache"),
		DownloadURLProvider: func(context.Context) (string, error) { return server.URL, nil },
		Token:               "secret", PanelPort: 8080, HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	instance := domain.Instance{ID: "instance-a", AcceleratorEnabled: true}
	if err := manager.Ensure(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	manager.token = "rotated-secret"
	if err := manager.Ensure(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	instance.AcceleratorEnabled = false
	if err := manager.Ensure(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(corePath); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("core restored=%q err=%v want=%q", got, err, original)
	}
	if _, err := os.Stat(filepath.Join(gameRoot, "addons", "sourcemod", "extensions", "accelerator.autoload")); !os.IsNotExist(err) {
		t.Fatalf("autoload still exists err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "instances", "instance-a", "accelerator-manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("manifest still exists err=%v", err)
	}
}

func TestRemoveReportsExternalModificationWithoutOverwriting(t *testing.T) {
	root := t.TempDir()
	gameRoot := filepath.Join(root, "instances", "instance-a", "game", "left4dead2")
	coreDir := filepath.Join(gameRoot, "addons", "sourcemod", "configs")
	if err := os.MkdirAll(coreDir, 0o750); err != nil {
		t.Fatal(err)
	}
	corePath := filepath.Join(coreDir, "core.cfg")
	if err := os.WriteFile(corePath, []byte(`"Core" { "CustomKey" "keep" }
`), 0o640); err != nil {
		t.Fatal(err)
	}
	archive := makeTestArchive(t, []testZipEntry{
		{name: "addons/sourcemod/extensions/accelerator.autoload", data: "autoload"},
		{name: "addons/sourcemod/extensions/accelerator.ext.so", data: "extension"},
		{name: "addons/sourcemod/gamedata/accelerator.games.txt", data: "gamedata"},
	})
	archiveBytes, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(archiveBytes) }))
	defer server.Close()
	manager, err := New(Config{
		InstancesRoot: filepath.Join(root, "instances"), CacheRoot: filepath.Join(root, "cache"),
		DownloadURLProvider: func(context.Context) (string, error) { return server.URL, nil },
		Token:               "secret", PanelPort: 8080, HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	instance := domain.Instance{ID: "instance-a", AcceleratorEnabled: true}
	if err := manager.Ensure(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	managedPath := filepath.Join(gameRoot, "addons", "sourcemod", "extensions", "accelerator.autoload")
	if err := os.WriteFile(managedPath, []byte("external"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(context.Background(), instance.ID); !errors.Is(err, ErrManagedConflict) {
		t.Fatalf("remove error=%v", err)
	}
	got, err := os.ReadFile(managedPath)
	if err != nil || string(got) != "external" {
		t.Fatalf("external file=%q err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(root, "instances", "instance-a", "accelerator-manifest.json")); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureRejectsMissingTokenAndUnsafeInstanceID(t *testing.T) {
	manager, err := New(Config{InstancesRoot: t.TempDir(), CacheRoot: filepath.Join(t.TempDir(), "cache"), PanelPort: 8080})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Ensure(context.Background(), domain.Instance{ID: "instance-a", AcceleratorEnabled: true}); err == nil {
		t.Fatal("enabled Accelerator without token or URL")
	}
	if err := manager.Ensure(context.Background(), domain.Instance{ID: "..", AcceleratorEnabled: false}); err == nil {
		t.Fatal("accepted unsafe instance id")
	}
}

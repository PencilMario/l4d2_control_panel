package updates

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestHotUpdateAppliesAllowedFilesAndPrivateLast(t *testing.T) {
	root := t.TempDir()
	archive := zipFile(t, map[string]string{"cfg/plugin.cfg": "package", "addons/sourcemod/plugins/x.smx": "binary"})
	private := filepath.Join(root, "instances", "abc", "private", "cfg")
	_ = os.MkdirAll(private, 0750)
	_ = os.WriteFile(filepath.Join(private, "plugin.cfg"), []byte("private"), 0640)
	pipeline := New(root)
	if err := pipeline.Apply(context.Background(), "abc", archive, "v1", Hot); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(root, "instances", "abc", "game", "left4dead2", "cfg", "plugin.cfg"))
	if string(raw) != "private" {
		t.Fatalf("got %q", raw)
	}
}
func TestUpdateStripsReleaseWrapperDirectory(t *testing.T) {
	root := t.TempDir()
	archive := zipFile(t, map[string]string{"release/cfg/plugin.cfg": "new"})
	if err := New(root).Apply(context.Background(), "abc", archive, "v1", Hot); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "instances", "abc", "game", "left4dead2", "cfg", "plugin.cfg")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "instances", "abc", "game", "left4dead2", "release")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("wrapper directory exists: %v", err)
	}
}
func TestHotUpdateRejectsBinaryOutsideAllowlist(t *testing.T) {
	pipeline := New(t.TempDir())
	if err := pipeline.Apply(context.Background(), "abc", zipFile(t, map[string]string{"bin/server.so": "x"}), "v1", Hot); err == nil {
		t.Fatal("unsafe hot update accepted")
	}
}
func TestHotUpdateFiltersNonHotFilesFromMixedRelease(t *testing.T) {
	root := t.TempDir()
	archive := zipFile(t, map[string]string{
		"release/cfg/plugin.cfg": "new",
		"release/bin/server.so":  "binary",
		"release/README.md":      "docs",
	})
	if err := New(root).Apply(context.Background(), "abc", archive, "v2", Hot); err != nil {
		t.Fatal(err)
	}
	game := filepath.Join(root, "instances", "abc", "game", "left4dead2")
	if raw, err := os.ReadFile(filepath.Join(game, "cfg", "plugin.cfg")); err != nil || string(raw) != "new" {
		t.Fatalf("hot file=%q err=%v", raw, err)
	}
	if _, err := os.Stat(filepath.Join(game, "bin", "server.so")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("binary was applied: %v", err)
	}
}
func TestFailureRollsBackReplacedFiles(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "instances", "abc", "game", "left4dead2", "cfg")
	_ = os.MkdirAll(target, 0750)
	_ = os.WriteFile(filepath.Join(target, "plugin.cfg"), []byte("old"), 0640)
	pipeline := New(root)
	pipeline.AfterDeploy = func() error { return errors.New("injected failure") }
	err := pipeline.Apply(context.Background(), "abc", zipFile(t, map[string]string{"cfg/plugin.cfg": "new"}), "v2", Full)
	if err == nil {
		t.Fatal("expected failure")
	}
	raw, _ := os.ReadFile(filepath.Join(target, "plugin.cfg"))
	if string(raw) != "old" {
		t.Fatalf("rollback got %q", raw)
	}
}

func TestRecoverRollsBackUncommittedDeployment(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "instances", "abc")
	target := filepath.Join(base, "game", "left4dead2", "cfg")
	if err := os.MkdirAll(target, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "plugin.cfg"), []byte("old"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(filepath.Join(base, "package-manifest.json"), manifest{Version: "v1", Files: map[string]string{"cfg/plugin.cfg": "old-hash"}}); err != nil {
		t.Fatal(err)
	}

	deployment, err := New(root).Begin(context.Background(), "abc", zipFile(t, map[string]string{"cfg/plugin.cfg": "new"}), "v2", Full)
	if err != nil {
		t.Fatal(err)
	}
	if deployment == nil {
		t.Fatal("missing deployment transaction")
	}
	raw, _ := os.ReadFile(filepath.Join(target, "plugin.cfg"))
	if string(raw) != "new" {
		t.Fatalf("deployed=%q", raw)
	}
	if journals, _ := filepath.Glob(filepath.Join(base, "backups", "update-*", "journal.json")); len(journals) != 1 {
		t.Fatalf("journals=%v", journals)
	}

	if err := New(root).Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(filepath.Join(target, "plugin.cfg"))
	if string(raw) != "old" {
		t.Fatalf("recovered=%q", raw)
	}
	if got := readManifest(filepath.Join(base, "package-manifest.json")); got.Version != "v1" {
		t.Fatalf("manifest=%#v", got)
	}
	if journals, _ := filepath.Glob(filepath.Join(base, "backups", "update-*", "journal.json")); len(journals) != 0 {
		t.Fatalf("stale journals=%v", journals)
	}
}
func zipFile(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "package.zip")
	file, _ := os.Create(path)
	writer := zip.NewWriter(file)
	for name, value := range files {
		entry, _ := writer.Create(name)
		_, _ = entry.Write([]byte(value))
	}
	_ = writer.Close()
	_ = file.Close()
	return path
}

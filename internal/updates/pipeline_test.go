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
func TestHotUpdateRejectsBinaryOutsideAllowlist(t *testing.T) {
	pipeline := New(t.TempDir())
	if err := pipeline.Apply(context.Background(), "abc", zipFile(t, map[string]string{"bin/server.so": "x"}), "v1", Hot); err == nil {
		t.Fatal("unsafe hot update accepted")
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

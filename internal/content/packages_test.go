package content

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPackageUploadStoresValidatedVersionAndManifest(t *testing.T) {
	root := t.TempDir()
	raw := packageZip(t, map[string]string{"cfg/plugin.cfg": "x"})
	manager, _ := NewPackageManager(root)
	version, err := manager.AddUpload("plugins.zip", "v1.2.3", bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if version.Version != "v1.2.3" || version.Hash == "" || !version.HotCompatible || len(version.Files) != 1 {
		t.Fatalf("version=%#v", version)
	}
	loaded, err := manager.Get(version.ID)
	if err != nil || loaded.ArchivePath == "" {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
}
func TestPackageUploadRejectsTraversal(t *testing.T) {
	raw := packageZip(t, map[string]string{"../escape": "x"})
	manager, _ := NewPackageManager(t.TempDir())
	if _, err := manager.AddUpload("bad.zip", "v1", bytes.NewReader(raw), int64(len(raw))); err == nil {
		t.Fatal("malicious package accepted")
	}
}
func packageZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "p.zip")
	file, _ := os.Create(path)
	writer := zip.NewWriter(file)
	for name, value := range files {
		entry, _ := writer.Create(name)
		_, _ = entry.Write([]byte(value))
	}
	_ = writer.Close()
	_ = file.Close()
	raw, _ := os.ReadFile(path)
	return raw
}

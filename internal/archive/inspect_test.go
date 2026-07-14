package archive

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectZipRejectsTraversalAndDisallowedHotPath(t *testing.T) {
	bad := makeZip(t, map[string]string{"../escape": "x"})
	if _, err := InspectZip(bad, Limits{MaxFiles: 10, MaxBytes: 100}); err == nil {
		t.Fatal("accepted traversal")
	}
	cold := makeZip(t, map[string]string{"bin/server.so": "x"})
	m, err := InspectZip(cold, Limits{MaxFiles: 10, MaxBytes: 100})
	if err != nil {
		t.Fatal(err)
	}
	if m.HotCompatible {
		t.Fatal("binary path marked hot compatible")
	}
	hot := makeZip(t, map[string]string{"addons/sourcemod/plugins/test.smx": "x", "cfg/test.cfg": "y"})
	m, err = InspectZip(hot, Limits{MaxFiles: 10, MaxBytes: 100})
	if err != nil || !m.HotCompatible {
		t.Fatalf("manifest=%#v err=%v", m, err)
	}
}

func TestInspectZipRejectsSingleFileAndCompressionBomb(t *testing.T) {
	archive := makeZip(t, map[string]string{"cfg/huge.cfg": strings.Repeat("a", 4096)})
	if _, err := InspectZip(archive, Limits{MaxFiles: 10, MaxBytes: 10000, MaxFileBytes: 1024, MaxCompressionRatio: 1000}); err == nil {
		t.Fatal("oversized file accepted")
	}
	if _, err := InspectZip(archive, Limits{MaxFiles: 10, MaxBytes: 10000, MaxFileBytes: 10000, MaxCompressionRatio: 2}); err == nil {
		t.Fatal("compression bomb accepted")
	}
}
func makeZip(t *testing.T, files map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "x.zip")
	f, _ := os.Create(p)
	w := zip.NewWriter(f)
	for n, v := range files {
		x, _ := w.Create(n)
		_, _ = x.Write([]byte(v))
	}
	_ = w.Close()
	_ = f.Close()
	return p
}

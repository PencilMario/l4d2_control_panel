package config

import (
	"path/filepath"
	"testing"
)

func TestLoadCreatesPersistentLayout(t *testing.T) {
	root := t.TempDir()
	t.Setenv("L4D2_PANEL_DATA_ROOT", filepath.Join(root, "panel-data"))
	t.Setenv("L4D2_PANEL_LISTEN", "")
	t.Setenv("L4D2_PANEL_GAME_HOST", "192.0.2.10")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddress != ":8080" {
		t.Fatalf("ListenAddress = %q", cfg.ListenAddress)
	}
	for _, path := range []string{cfg.PanelDir, cfg.PackagesDir, cfg.InstancesDir, cfg.SharedVPKDir} {
		if !isDirectory(path) {
			t.Fatalf("expected directory %s", path)
		}
	}
}

func TestLoadRequiresGameHost(t *testing.T) {
	t.Setenv("L4D2_PANEL_DATA_ROOT", t.TempDir())
	t.Setenv("L4D2_PANEL_GAME_HOST", "")
	if _, err := Load(); err == nil {
		t.Fatal("missing game host accepted")
	}
}

func TestLoadRejectsRelativeDataRoot(t *testing.T) {
	t.Setenv("L4D2_PANEL_DATA_ROOT", "relative")
	if _, err := Load(); err == nil {
		t.Fatal("expected relative data root to be rejected")
	}
}

package config

import (
	"path/filepath"
	"reflect"
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
	for _, path := range []string{cfg.PanelDir, cfg.PackagesDir, cfg.InstancesDir, cfg.SharedVPKDir, cfg.GameDir, cfg.GameReleasesDir, cfg.GameStagingDir} {
		if !isDirectory(path) {
			t.Fatalf("expected directory %s", path)
		}
	}
	if cfg.GameCurrentPath != filepath.Join(cfg.GameDir, "current") {
		t.Fatalf("GameCurrentPath = %q", cfg.GameCurrentPath)
	}
	wantOverlay := filepath.Join(cfg.InstancesDir, "abc", "overlay")
	if got := cfg.InstanceOverlayDir("abc"); got != wantOverlay {
		t.Fatalf("InstanceOverlayDir = %q, want %q", got, wantOverlay)
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

func TestLoadConfiguresCrashReportDefaults(t *testing.T) {
	root := t.TempDir()
	t.Setenv("L4D2_PANEL_DATA_ROOT", filepath.Join(root, "panel-data"))
	t.Setenv("L4D2_PANEL_GAME_HOST", "192.0.2.10")
	t.Setenv("L4D2_PANEL_CRASH_REPORT_TOKEN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CrashReportsDir != filepath.Join(cfg.PanelDir, "crash-dumps") {
		t.Fatalf("CrashReportsDir=%q", cfg.CrashReportsDir)
	}
	if cfg.CrashReportToken != "" {
		t.Fatalf("CrashReportToken=%q", cfg.CrashReportToken)
	}
}

func TestLoadParsesCrashReportTokenWithoutRetentionConfiguration(t *testing.T) {
	t.Setenv("L4D2_PANEL_DATA_ROOT", t.TempDir())
	t.Setenv("L4D2_PANEL_GAME_HOST", "192.0.2.10")
	t.Setenv("L4D2_PANEL_CRASH_REPORT_TOKEN", "crash-secret")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CrashReportToken != "crash-secret" {
		t.Fatalf("config=%#v", cfg)
	}
}

func TestLoadParsesPanelPortAndStackwalkPath(t *testing.T) {
	t.Setenv("L4D2_PANEL_DATA_ROOT", t.TempDir())
	t.Setenv("L4D2_PANEL_GAME_HOST", "192.0.2.10")
	t.Setenv("L4D2_PANEL_LISTEN", "127.0.0.1:9090")
	t.Setenv("L4D2_PANEL_STACKWALK_PATH", "/opt/tools/minidump_stackwalk")
	t.Setenv("L4D2_PANEL_DUMP_SYMS_PATH", "/opt/tools/dump_syms")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PanelPort != 9090 || cfg.StackwalkPath != "/opt/tools/minidump_stackwalk" || cfg.DumpSymsPath != "/opt/tools/dump_syms" {
		t.Fatalf("config=%+v", cfg)
	}
}

func TestLoadUsesDedicatedAcceleratorPort(t *testing.T) {
	t.Setenv("L4D2_PANEL_DATA_ROOT", t.TempDir())
	t.Setenv("L4D2_PANEL_GAME_HOST", "192.0.2.10")
	t.Setenv("L4D2_PANEL_LISTEN", "0.0.0.0:8080")
	t.Setenv("L4D2_PANEL_ACCELERATOR_PORT", "18081")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	field := reflect.ValueOf(cfg).FieldByName("AcceleratorPort")
	if !field.IsValid() {
		t.Fatal("Config is missing AcceleratorPort")
	}
	if got := int(field.Int()); got != 18081 {
		t.Fatalf("AcceleratorPort=%d, want 18081", got)
	}
}

func TestLoadRejectsInvalidAcceleratorPort(t *testing.T) {
	t.Setenv("L4D2_PANEL_DATA_ROOT", t.TempDir())
	t.Setenv("L4D2_PANEL_GAME_HOST", "192.0.2.10")
	t.Setenv("L4D2_PANEL_ACCELERATOR_PORT", "70000")

	if _, err := Load(); err == nil {
		t.Fatal("accepted invalid Accelerator port")
	}
}

func TestLoadUsesStackwalkDefaultAndRejectsInvalidListenOrToolPath(t *testing.T) {
	for _, test := range []struct {
		name    string
		listen  string
		tool    string
		valid   bool
		port    int
		toolOut string
	}{
		{name: "default", listen: ":8080", valid: true, port: 8080, toolOut: "/usr/local/bin/minidump_stackwalk"},
		{name: "bad listen", listen: "not-a-listen-address"},
		{name: "bad port", listen: ":70000"},
		{name: "url tool", listen: ":8080", tool: "https://example.test/stackwalk"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("L4D2_PANEL_DATA_ROOT", t.TempDir())
			t.Setenv("L4D2_PANEL_GAME_HOST", "192.0.2.10")
			t.Setenv("L4D2_PANEL_LISTEN", test.listen)
			t.Setenv("L4D2_PANEL_STACKWALK_PATH", test.tool)
			cfg, err := Load()
			if !test.valid {
				if err == nil {
					t.Fatalf("accepted invalid config: %+v", cfg)
				}
				return
			}
			if err != nil || cfg.PanelPort != test.port || cfg.StackwalkPath != test.toolOut || cfg.DumpSymsPath != "/usr/local/bin/dump_syms" {
				t.Fatalf("config=%+v err=%v", cfg, err)
			}
		})
	}
}

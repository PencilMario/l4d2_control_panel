package config

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/not0721here/l4d2-control-panel/internal/crashreports"
)

type Config struct {
	ListenAddress      string
	PanelPort          int
	AcceleratorPort    int
	StackwalkPath      string
	GameHost           string
	DataRoot           string
	PanelDir           string
	PackagesDir        string
	InstancesDir       string
	SharedVPKDir       string
	GameDir            string
	GameReleasesDir    string
	GameStagingDir     string
	GameCurrentPath    string
	DatabasePath       string
	CrashReportsDir    string
	CrashRetentionDays int
	CrashReportToken   string
}

func (c Config) InstanceOverlayDir(instanceID string) string {
	return filepath.Join(c.InstancesDir, instanceID, "overlay")
}

func Load() (Config, error) {
	root := os.Getenv("L4D2_PANEL_DATA_ROOT")
	if root == "" {
		root = "/srv/l4d2-panel"
	}
	if !filepath.IsAbs(root) {
		return Config{}, errors.New("L4D2_PANEL_DATA_ROOT must be absolute")
	}
	listen := os.Getenv("L4D2_PANEL_LISTEN")
	if listen == "" {
		listen = ":8080"
	}
	panelPort, err := listenPort(listen)
	if err != nil {
		return Config{}, err
	}
	acceleratorPort := panelPort
	if raw := os.Getenv("L4D2_PANEL_ACCELERATOR_PORT"); raw != "" {
		acceleratorPort, err = strconv.Atoi(raw)
		if err != nil || acceleratorPort < 1 || acceleratorPort > 65535 {
			return Config{}, errors.New("L4D2_PANEL_ACCELERATOR_PORT must be between 1 and 65535")
		}
	}
	gameHost := os.Getenv("L4D2_PANEL_GAME_HOST")
	if gameHost == "" {
		return Config{}, errors.New("L4D2_PANEL_GAME_HOST is required and must be an address SRCDS answers on")
	}
	retentionDays := crashreports.DefaultRetentionDays
	if raw := os.Getenv("L4D2_PANEL_CRASH_RETENTION_DAYS"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < crashreports.MinRetentionDays || parsed > crashreports.MaxRetentionDays {
			return Config{}, errors.New("L4D2_PANEL_CRASH_RETENTION_DAYS must be an integer between 1 and 3650")
		}
		retentionDays = parsed
	}
	stackwalkPath := os.Getenv("L4D2_PANEL_STACKWALK_PATH")
	if stackwalkPath == "" {
		stackwalkPath = "/usr/local/bin/minidump_stackwalk"
	}
	if !plainFilesystemPath(stackwalkPath) {
		return Config{}, errors.New("L4D2_PANEL_STACKWALK_PATH must be an absolute filesystem path")
	}
	c := Config{
		ListenAddress:      listen,
		PanelPort:          panelPort,
		AcceleratorPort:    acceleratorPort,
		StackwalkPath:      stackwalkPath,
		GameHost:           gameHost,
		DataRoot:           filepath.Clean(root),
		CrashRetentionDays: retentionDays,
		CrashReportToken:   os.Getenv("L4D2_PANEL_CRASH_REPORT_TOKEN"),
	}
	c.PanelDir = filepath.Join(c.DataRoot, "panel")
	c.PackagesDir = filepath.Join(c.DataRoot, "packages")
	c.InstancesDir = filepath.Join(c.DataRoot, "instances")
	c.SharedVPKDir = filepath.Join(c.DataRoot, "shared-vpk")
	c.GameDir = filepath.Join(c.DataRoot, "game")
	c.GameReleasesDir = filepath.Join(c.GameDir, "releases")
	c.GameStagingDir = filepath.Join(c.GameDir, "staging")
	c.GameCurrentPath = filepath.Join(c.GameDir, "current")
	c.DatabasePath = filepath.Join(c.PanelDir, "panel.db")
	c.CrashReportsDir = filepath.Join(c.PanelDir, "crash-dumps")
	for _, p := range []string{c.PanelDir, filepath.Join(c.PackagesDir, "uploads"), filepath.Join(c.PackagesDir, "releases"), c.InstancesDir, c.SharedVPKDir, c.GameReleasesDir, c.GameStagingDir} {
		if err := os.MkdirAll(p, 0o750); err != nil {
			return Config{}, err
		}
	}
	return c, nil
}

func listenPort(listen string) (int, error) {
	_, rawPort, err := net.SplitHostPort(listen)
	if err != nil {
		return 0, errors.New("L4D2_PANEL_LISTEN must be a host:port address")
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil || port < 1 || port > 65535 {
		return 0, errors.New("L4D2_PANEL_LISTEN port must be between 1 and 65535")
	}
	return port, nil
}

func plainFilesystemPath(value string) bool {
	if value == "" || strings.ContainsAny(value, "\x00\r\n") || strings.Contains(value, "://") {
		return false
	}
	if filepath.IsAbs(value) || strings.HasPrefix(value, "/") {
		return true
	}
	return len(value) >= 3 && value[1] == ':' && (value[2] == '\\' || value[2] == '/')
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

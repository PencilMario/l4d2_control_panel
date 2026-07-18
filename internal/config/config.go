package config

import (
	"errors"
	"os"
	"path/filepath"
)

type Config struct {
	ListenAddress   string
	GameHost        string
	DataRoot        string
	PanelDir        string
	PackagesDir     string
	InstancesDir    string
	SharedVPKDir    string
	GameDir         string
	GameReleasesDir string
	GameStagingDir  string
	GameCurrentPath string
	DatabasePath    string
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
	gameHost := os.Getenv("L4D2_PANEL_GAME_HOST")
	if gameHost == "" {
		return Config{}, errors.New("L4D2_PANEL_GAME_HOST is required and must be an address SRCDS answers on")
	}
	c := Config{ListenAddress: listen, GameHost: gameHost, DataRoot: filepath.Clean(root)}
	c.PanelDir = filepath.Join(c.DataRoot, "panel")
	c.PackagesDir = filepath.Join(c.DataRoot, "packages")
	c.InstancesDir = filepath.Join(c.DataRoot, "instances")
	c.SharedVPKDir = filepath.Join(c.DataRoot, "shared-vpk")
	c.GameDir = filepath.Join(c.DataRoot, "game")
	c.GameReleasesDir = filepath.Join(c.GameDir, "releases")
	c.GameStagingDir = filepath.Join(c.GameDir, "staging")
	c.GameCurrentPath = filepath.Join(c.GameDir, "current")
	c.DatabasePath = filepath.Join(c.PanelDir, "panel.db")
	for _, p := range []string{c.PanelDir, filepath.Join(c.PackagesDir, "uploads"), filepath.Join(c.PackagesDir, "releases"), c.InstancesDir, c.SharedVPKDir, c.GameReleasesDir, c.GameStagingDir} {
		if err := os.MkdirAll(p, 0o750); err != nil {
			return Config{}, err
		}
	}
	return c, nil
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

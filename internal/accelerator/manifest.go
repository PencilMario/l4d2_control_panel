package accelerator

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const manifestVersion = 1

const manifestCoreConfigPath = "game/left4dead2/addons/sourcemod/configs/core.cfg"

var (
	ErrManagedConflict = errors.New("Accelerator managed files were modified outside Panel ownership")
	ErrInvalidManifest = errors.New("invalid Accelerator manifest")
)

type ManagedFile struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Manifest struct {
	Version           int                         `json:"version"`
	Enabled           bool                        `json:"enabled"`
	ArchiveSHA256     string                      `json:"archive_sha256"`
	SourceURL         string                      `json:"source_url"`
	ResolvedURL       string                      `json:"resolved_url,omitempty"`
	InstalledAt       time.Time                   `json:"installed_at"`
	UpdatedAt         time.Time                   `json:"updated_at"`
	Files             map[string]ManagedFile      `json:"files"`
	PreservedFiles    map[string]bool             `json:"preserved_files,omitempty"`
	CoreConfigPath    string                      `json:"core_config_path"`
	CoreConfigSHA256  string                      `json:"core_config_sha256"`
	CoreConfigChanges map[string]CoreConfigChange `json:"core_config_changes"`
}

type ConflictError struct {
	Paths []string
}

func (e *ConflictError) Error() string {
	if len(e.Paths) == 0 {
		return ErrManagedConflict.Error()
	}
	paths := append([]string(nil), e.Paths...)
	sort.Strings(paths)
	return fmt.Sprintf("%s: %s", ErrManagedConflict, strings.Join(paths, ", "))
}

func (e *ConflictError) Unwrap() error { return ErrManagedConflict }

func manifestPath(instanceRoot string) string {
	return filepath.Join(instanceRoot, "accelerator-manifest.json")
}

func (m *Manager) loadManifest(instanceRoot string) (*Manifest, error) {
	path := manifestPath(instanceRoot)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, ErrInvalidManifest
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	if err := validateManifest(manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func validateManifest(manifest Manifest) error {
	if manifest.Version != manifestVersion || !manifest.Enabled || !validHash(manifest.ArchiveSHA256) || !validHash(manifest.CoreConfigSHA256) {
		return ErrInvalidManifest
	}
	if manifest.CoreConfigPath != manifestCoreConfigPath || filepath.IsAbs(manifest.CoreConfigPath) {
		return ErrInvalidManifest
	}
	if manifest.Files == nil || len(manifest.CoreConfigChanges) != len(managedCoreKeys) {
		return ErrInvalidManifest
	}
	for relative, file := range manifest.Files {
		normalized, err := normalizeArchivePath(relative)
		if err != nil || normalized != relative || !validHash(file.SHA256) || file.Size < 0 {
			return ErrInvalidManifest
		}
	}
	for relative, preserved := range manifest.PreservedFiles {
		if !preserved {
			return ErrInvalidManifest
		}
		if _, ok := manifest.Files[relative]; !ok {
			return ErrInvalidManifest
		}
	}
	for key, change := range manifest.CoreConfigChanges {
		if !containsManagedCoreKey(key) || change.Written == "" {
			return ErrInvalidManifest
		}
	}
	for _, key := range managedCoreKeys {
		if _, ok := manifest.CoreConfigChanges[key]; !ok {
			return ErrInvalidManifest
		}
	}
	return nil
}

func containsManagedCoreKey(key string) bool {
	for _, managed := range managedCoreKeys {
		if key == managed {
			return true
		}
	}
	return false
}

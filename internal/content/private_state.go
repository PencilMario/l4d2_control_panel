package content

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const privateManifestVersion = 1

type PrivateEntry struct {
	Path      string    `json:"path"`
	Kind      string    `json:"kind"`
	Hash      string    `json:"hash,omitempty"`
	Size      int64     `json:"size,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PrivateChange struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	BeforeHash string `json:"before_hash,omitempty"`
	AfterHash  string `json:"after_hash,omitempty"`
}

type DiffSummary struct {
	Added    int `json:"added"`
	Modified int `json:"modified"`
	Deleted  int `json:"deleted"`
}

type PrivateDiff struct {
	Changes []PrivateChange `json:"changes"`
	Summary DiffSummary     `json:"summary"`
}

type privateManifest struct {
	Version   int                      `json:"version"`
	AppliedAt time.Time                `json:"applied_at"`
	Entries   map[string]manifestEntry `json:"entries"`
}

type manifestEntry struct {
	Kind      string    `json:"kind"`
	Hash      string    `json:"hash,omitempty"`
	Size      int64     `json:"size,omitempty"`
	UpdatedAt time.Time `json:"-"`
}

func scanPrivateTree(root string) (map[string]manifestEntry, error) {
	result := make(map[string]manifestEntry)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("symbolic links are forbidden")
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		item := manifestEntry{UpdatedAt: info.ModTime()}
		if info.IsDir() {
			item.Kind = "directory"
		} else if info.Mode().IsRegular() {
			item.Kind = "file"
			item.Size = info.Size()
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			digest := sha256.Sum256(raw)
			item.Hash = hex.EncodeToString(digest[:])
		} else {
			return errors.New("only regular files and directories are allowed")
		}
		result[filepath.ToSlash(relative)] = item
		return nil
	})
	return result, err
}

func (m *PrivateManager) manifestPath(instanceID string) (string, error) {
	if err := validateInstanceID(instanceID); err != nil {
		return "", err
	}
	return filepath.Join(m.root, "instances", instanceID, "private-applied.json"), nil
}

func (m *PrivateManager) readPrivateManifest(instanceID string) (privateManifest, error) {
	path, err := m.manifestPath(instanceID)
	if err != nil {
		return privateManifest{}, err
	}
	if err := rejectSymlinkParents(m.root, path); err != nil {
		return privateManifest{}, err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return privateManifest{Version: privateManifestVersion, Entries: map[string]manifestEntry{}}, nil
	}
	if err != nil {
		return privateManifest{}, err
	}
	var manifest privateManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return privateManifest{}, err
	}
	if manifest.Version != privateManifestVersion {
		return privateManifest{}, errors.New("unsupported private manifest version")
	}
	if manifest.Entries == nil {
		manifest.Entries = map[string]manifestEntry{}
	}
	return manifest, nil
}

func (m *PrivateManager) writePrivateManifest(instanceID string, manifest privateManifest) error {
	path, err := m.manifestPath(instanceID)
	if err != nil {
		return err
	}
	if err := rejectSymlinkParents(m.root, path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	manifest.Version = privateManifestVersion
	raw, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".private-manifest-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0640); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return replacePath(temporaryName, path, true)
}

func isEmptyDirectory(path string, entries map[string]manifestEntry) bool {
	prefix := path + "/"
	for candidate := range entries {
		if strings.HasPrefix(candidate, prefix) {
			return false
		}
	}
	return true
}

func isDiffResource(path string, entry manifestEntry, entries map[string]manifestEntry) bool {
	return entry.Kind == "file" || isEmptyDirectory(path, entries)
}

func (m *PrivateManager) Diff(_ context.Context, instanceID string) (PrivateDiff, error) {
	root, err := m.privateRoot(instanceID)
	if err != nil {
		return PrivateDiff{}, err
	}
	current, err := scanPrivateTree(root)
	if err != nil {
		return PrivateDiff{}, err
	}
	applied, err := m.readPrivateManifest(instanceID)
	if err != nil {
		return PrivateDiff{}, err
	}
	result := PrivateDiff{Changes: []PrivateChange{}}
	paths := make(map[string]struct{}, len(current)+len(applied.Entries))
	for path := range current {
		paths[path] = struct{}{}
	}
	for path := range applied.Entries {
		paths[path] = struct{}{}
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	for _, path := range ordered {
		after, hasAfter := current[path]
		before, hasBefore := applied.Entries[path]
		beforeIsResource := hasBefore && isDiffResource(path, before, applied.Entries)
		afterIsResource := hasAfter && isDiffResource(path, after, current)
		if !beforeIsResource && !afterIsResource {
			continue
		}
		switch {
		case !hasBefore && hasAfter:
			result.Changes = append(result.Changes, PrivateChange{Path: path, Kind: "added", AfterHash: after.Hash})
			result.Summary.Added++
		case hasBefore && !hasAfter:
			result.Changes = append(result.Changes, PrivateChange{Path: path, Kind: "deleted", BeforeHash: before.Hash})
			result.Summary.Deleted++
		case before.Kind != after.Kind || before.Hash != after.Hash:
			result.Changes = append(result.Changes, PrivateChange{Path: path, Kind: "modified", BeforeHash: before.Hash, AfterHash: after.Hash})
			result.Summary.Modified++
		}
	}
	return result, nil
}

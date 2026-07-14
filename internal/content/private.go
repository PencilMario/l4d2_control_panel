package content

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/not0721here/l4d2-control-panel/internal/safepath"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type PrivateManager struct {
	root     string
	maxBytes int
}
type PrivateFile struct {
	Path, Hash string
	Size       int64
	UpdatedAt  time.Time
}

func NewPrivateManager(root string, maxBytes int) *PrivateManager {
	return &PrivateManager{root: root, maxBytes: maxBytes}
}

func validateInstanceID(instanceID string) error {
	if filepath.Base(instanceID) != instanceID || instanceID == "" || instanceID == "." || instanceID == ".." {
		return errors.New("invalid instance id")
	}
	return nil
}

func (m *PrivateManager) Save(_ context.Context, instanceID, name string, data []byte) (PrivateFile, error) {
	if err := validateInstanceID(instanceID); err != nil {
		return PrivateFile{}, err
	}
	if len(data) > m.maxBytes {
		return PrivateFile{}, errors.New("private file exceeds editor limit")
	}
	private := filepath.Join(m.root, "instances", instanceID, "private")
	target, err := safepath.Join(private, name)
	if err != nil {
		return PrivateFile{}, err
	}
	if err := rejectSymlinkParents(private, target); err != nil {
		return PrivateFile{}, err
	}
	if old, err := os.ReadFile(target); err == nil {
		digest := sha256.Sum256(old)
		backupRoot := filepath.Join(m.root, "instances", instanceID, "backups", "private")
		backup, joinErr := safepath.Join(backupRoot, name+"."+time.Now().UTC().Format("20060102T150405.000000000")+"."+hex.EncodeToString(digest[:8]))
		if joinErr != nil {
			return PrivateFile{}, joinErr
		}
		if err := os.MkdirAll(filepath.Dir(backup), 0750); err != nil {
			return PrivateFile{}, err
		}
		if err := os.WriteFile(backup, old, 0640); err != nil {
			return PrivateFile{}, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
		return PrivateFile{}, err
	}
	temporary := target + ".tmp"
	if err := os.WriteFile(temporary, data, 0640); err != nil {
		return PrivateFile{}, err
	}
	if err := os.Rename(temporary, target); err != nil {
		return PrivateFile{}, err
	}
	digest := sha256.Sum256(data)
	return PrivateFile{Path: filepath.ToSlash(name), Hash: hex.EncodeToString(digest[:]), Size: int64(len(data)), UpdatedAt: time.Now().UTC()}, nil
}
func (m *PrivateManager) History(_ context.Context, instanceID, name string) ([]PrivateFile, error) {
	if err := validateInstanceID(instanceID); err != nil {
		return nil, err
	}
	root := filepath.Join(m.root, "instances", instanceID, "backups", "private")
	prefix, err := safepath.Join(root, name+".")
	if err != nil {
		return nil, err
	}
	directory := filepath.Dir(prefix)
	base := filepath.Base(prefix)
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []PrivateFile{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []PrivateFile{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), base) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		path, err := filepath.Rel(root, filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, err
		}
		result = append(result, PrivateFile{Path: filepath.ToSlash(path), Size: info.Size(), UpdatedAt: info.ModTime()})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	return result, nil
}
func (m *PrivateManager) Apply(_ context.Context, instanceID string) error {
	if err := validateInstanceID(instanceID); err != nil {
		return err
	}
	source := filepath.Join(m.root, "instances", instanceID, "private")
	if _, err := os.Stat(source); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return ApplyTree(source, filepath.Join(m.root, "instances", instanceID, "game", "left4dead2"))
}
func rejectSymlinkParents(root, target string) error {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("symbolic links are forbidden")
		}
	}
	return nil
}
func (m *PrivateManager) Read(_ context.Context, instanceID, name string) ([]byte, error) {
	if err := validateInstanceID(instanceID); err != nil {
		return nil, err
	}
	root := filepath.Join(m.root, "instances", instanceID, "private")
	target, err := safepath.Join(root, name)
	if err != nil {
		return nil, err
	}
	if err := rejectSymlinkParents(root, target); err != nil {
		return nil, err
	}
	return os.ReadFile(target)
}
func (m *PrivateManager) List(_ context.Context, instanceID string) ([]PrivateFile, error) {
	if err := validateInstanceID(instanceID); err != nil {
		return nil, err
	}
	root := filepath.Join(m.root, "instances", instanceID, "private")
	result := []PrivateFile{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("symbolic links are forbidden")
		}
		if entry.IsDir() {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(raw)
		relative, _ := filepath.Rel(root, path)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		result = append(result, PrivateFile{Path: filepath.ToSlash(relative), Hash: hex.EncodeToString(digest[:]), Size: info.Size(), UpdatedAt: info.ModTime()})
		return nil
	})
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, err
}
func (m *PrivateManager) Delete(ctx context.Context, instanceID, name string) error {
	if err := validateInstanceID(instanceID); err != nil {
		return err
	}
	root := filepath.Join(m.root, "instances", instanceID, "private")
	target, err := safepath.Join(root, name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(target); err != nil {
		return err
	}
	if _, err := m.Save(ctx, instanceID, name, []byte{}); err != nil {
		return err
	}
	return os.Remove(target)
}

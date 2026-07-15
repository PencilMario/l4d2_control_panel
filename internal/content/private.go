package content

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/safepath"
)

type PrivateManager struct {
	root     string
	maxBytes int
	locks    sync.Map
}

func (m *PrivateManager) instanceLock(instanceID string) *sync.RWMutex {
	lock, _ := m.locks.LoadOrStore(instanceID, &sync.RWMutex{})
	return lock.(*sync.RWMutex)
}

type PrivateFile struct {
	Path      string    `json:"path"`
	Hash      string    `json:"hash,omitempty"`
	Size      int64     `json:"size"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewPrivateManager(root string, maxBytes int) *PrivateManager {
	return &PrivateManager{root: root, maxBytes: maxBytes}
}

func (m *PrivateManager) privateRoot(instanceID string) (string, error) {
	if err := validateInstanceID(instanceID); err != nil {
		return "", err
	}
	return filepath.Join(m.root, "instances", instanceID, "private"), nil
}

func validateInstanceID(instanceID string) error {
	if filepath.Base(instanceID) != instanceID || instanceID == "" || instanceID == "." || instanceID == ".." {
		return errors.New("invalid instance id")
	}
	return nil
}

func (m *PrivateManager) Save(_ context.Context, instanceID, name string, data []byte) (PrivateFile, error) {
	lock := m.instanceLock(instanceID)
	lock.Lock()
	defer lock.Unlock()
	return m.save(instanceID, name, data)
}

func (m *PrivateManager) save(instanceID, name string, data []byte) (PrivateFile, error) {
	private, err := m.privateRoot(instanceID)
	if err != nil {
		return PrivateFile{}, err
	}
	if len(data) > m.maxBytes {
		return PrivateFile{}, errors.New("private file exceeds editor limit")
	}
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
	temporaryRoot := filepath.Join(m.root, "instances", instanceID, "backups", "private", "workspace-temp")
	if err := os.MkdirAll(temporaryRoot, 0750); err != nil {
		return PrivateFile{}, err
	}
	temporary, err := os.CreateTemp(temporaryRoot, "save-*")
	if err != nil {
		return PrivateFile{}, err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0640); err != nil {
		temporary.Close()
		return PrivateFile{}, err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return PrivateFile{}, err
	}
	if err := temporary.Close(); err != nil {
		return PrivateFile{}, err
	}
	if err := os.Rename(temporaryName, target); err != nil {
		return PrivateFile{}, err
	}
	digest := sha256.Sum256(data)
	return PrivateFile{Path: filepath.ToSlash(name), Hash: hex.EncodeToString(digest[:]), Size: int64(len(data)), UpdatedAt: time.Now().UTC()}, nil
}

func (m *PrivateManager) MakeDir(_ context.Context, instanceID, name string) error {
	lock := m.instanceLock(instanceID)
	lock.Lock()
	defer lock.Unlock()
	root, err := m.privateRoot(instanceID)
	if err != nil {
		return err
	}
	target, err := safepath.Join(root, name)
	if err != nil {
		return err
	}
	if err := rejectSymlinkParents(root, target); err != nil {
		return err
	}
	return os.MkdirAll(target, 0750)
}

func (m *PrivateManager) Move(_ context.Context, instanceID, from, to string, overwrite bool) error {
	lock := m.instanceLock(instanceID)
	lock.Lock()
	defer lock.Unlock()
	root, err := m.privateRoot(instanceID)
	if err != nil {
		return err
	}
	source, err := safepath.Join(root, from)
	if err != nil {
		return err
	}
	target, err := safepath.Join(root, to)
	if err != nil {
		return err
	}
	if err := rejectSymlinkParents(root, source); err != nil {
		return err
	}
	if err := rejectSymlinkParents(root, target); err != nil {
		return err
	}
	if source == target {
		return errors.New("source and destination are the same")
	}
	sourceInfo, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if sourceInfo.IsDir() {
		relative, err := filepath.Rel(source, target)
		if err != nil {
			return err
		}
		if relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return errors.New("cannot move directory into itself")
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
		return err
	}
	return replacePath(source, target, overwrite)
}

func replacePath(source, target string, overwrite bool) error {
	_, targetErr := os.Lstat(target)
	if errors.Is(targetErr, os.ErrNotExist) {
		return os.Rename(source, target)
	}
	if targetErr != nil {
		return targetErr
	}
	if !overwrite {
		return errors.New("destination exists")
	}

	placeholder, err := os.CreateTemp(filepath.Dir(target), ".private-replaced-*")
	if err != nil {
		return err
	}
	backup := placeholder.Name()
	if err := placeholder.Close(); err != nil {
		os.Remove(backup)
		return err
	}
	removeBackup := true
	defer func() {
		if removeBackup {
			_ = os.RemoveAll(backup)
		}
	}()
	if err := os.Remove(backup); err != nil {
		return err
	}
	if err := os.Rename(target, backup); err != nil {
		return err
	}
	if err := os.Rename(source, target); err != nil {
		if rollbackErr := os.Rename(backup, target); rollbackErr != nil {
			removeBackup = false
			return errors.Join(err, errors.New("restore destination: "+rollbackErr.Error()))
		}
		return err
	}
	if err := os.RemoveAll(backup); err != nil {
		return errors.New("remove replaced destination: " + err.Error())
	}
	return nil
}

func (m *PrivateManager) Tree(_ context.Context, instanceID string) ([]PrivateEntry, error) {
	lock := m.instanceLock(instanceID)
	lock.RLock()
	defer lock.RUnlock()
	root, err := m.privateRoot(instanceID)
	if err != nil {
		return nil, err
	}
	entries, err := scanPrivateTree(root)
	if err != nil {
		return nil, err
	}
	result := make([]PrivateEntry, 0, len(entries))
	for path, entry := range entries {
		result = append(result, PrivateEntry{Path: path, Kind: entry.Kind, Hash: entry.Hash, Size: entry.Size, UpdatedAt: entry.UpdatedAt})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}
func (m *PrivateManager) History(_ context.Context, instanceID, name string) ([]PrivateFile, error) {
	lock := m.instanceLock(instanceID)
	lock.RLock()
	defer lock.RUnlock()
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
	lock := m.instanceLock(instanceID)
	lock.RLock()
	defer lock.RUnlock()
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
	lock := m.instanceLock(instanceID)
	lock.RLock()
	defer lock.RUnlock()
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
	lock := m.instanceLock(instanceID)
	lock.RLock()
	defer lock.RUnlock()
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
func (m *PrivateManager) Delete(_ context.Context, instanceID, name string) error {
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
	lock := m.instanceLock(instanceID)
	lock.Lock()
	defer lock.Unlock()
	if _, err := m.save(instanceID, name, []byte{}); err != nil {
		return err
	}
	return os.Remove(target)
}

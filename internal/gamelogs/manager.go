package gamelogs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/safepath"
)

type Entry struct {
	Kind       string    `json:"kind"`
	Path       string    `json:"path"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
}

type Preview struct {
	Text       string    `json:"text"`
	Truncated  bool      `json:"truncated"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
}

type Options struct {
	Now func() time.Time
}

type Manager struct {
	root      string
	now       func() time.Time
	locksMu   sync.Mutex
	instances map[string]*sync.Mutex
}

func NewManager(root string, options Options) *Manager {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Manager{root: root, now: now, instances: make(map[string]*sync.Mutex)}
}

// Tree returns regular log files ordered first by kind and then by slash path.
func (m *Manager) Tree(ctx context.Context, instanceID string) ([]Entry, error) {
	if err := validateInstanceID(instanceID); err != nil {
		return nil, err
	}
	entries := make([]Entry, 0)
	for _, kind := range []string{"game", "sourcemod"} {
		root := m.logRoot(instanceID, kind)
		exists, err := validateDirectoryPath(m.root, root, true)
		if err != nil {
			return nil, fmt.Errorf("inspect %s log root: %w", kind, err)
		}
		if !exists {
			continue
		}
		err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if path == root {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("log tree contains symlink: %s", path)
			}
			if info.IsDir() {
				return nil
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("log tree contains special file: %s", path)
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			entries = append(entries, Entry{Kind: kind, Path: filepath.ToSlash(relative), Size: info.Size(), ModifiedAt: info.ModTime().UTC()})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk %s logs: %w", kind, err)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind < entries[j].Kind
		}
		return entries[i].Path < entries[j].Path
	})
	return entries, nil
}

func (m *Manager) Preview(ctx context.Context, instanceID, kind, relative string, limit int64) (Preview, error) {
	if limit <= 0 {
		return Preview{}, errors.New("preview limit must be positive")
	}
	if err := ctx.Err(); err != nil {
		return Preview{}, err
	}
	file, info, err := m.ResolveDownload(instanceID, kind, relative)
	if err != nil {
		return Preview{}, err
	}
	defer file.Close()
	return previewOpenFile(file, info, limit)
}

func previewOpenFile(file *os.File, info os.FileInfo, limit int64) (Preview, error) {
	size := info.Size()
	start := int64(0)
	if size > limit {
		start = size - limit
	}
	buffer := make([]byte, size-start)
	n, err := file.ReadAt(buffer, start)
	if err != nil && !errors.Is(err, io.EOF) {
		return Preview{}, fmt.Errorf("read log preview: %w", err)
	}
	buffer = buffer[:n]
	return Preview{Text: strings.ToValidUTF8(string(buffer), "\ufffd"), Truncated: start > 0, Size: size, ModifiedAt: info.ModTime().UTC()}, nil
}

// ResolveDownload validates the complete path and returns an already-open file,
// so callers never need to reopen a checked pathname.
func (m *Manager) ResolveDownload(instanceID, kind, relative string) (*os.File, os.FileInfo, error) {
	if err := validateInstanceID(instanceID); err != nil {
		return nil, nil, err
	}
	root, err := m.kindRoot(instanceID, kind)
	if err != nil {
		return nil, nil, err
	}
	if strings.IndexByte(relative, 0) >= 0 {
		return nil, nil, errors.New("log path contains NUL")
	}
	target, err := safepath.Join(root, relative)
	if err != nil {
		return nil, nil, err
	}
	parentExists, err := validateDirectoryPath(root, filepath.Dir(target), true)
	if err != nil {
		return nil, nil, fmt.Errorf("validate log parent: %w", err)
	}
	if !parentExists {
		return nil, nil, os.ErrNotExist
	}
	leaf, err := os.Lstat(target)
	if err != nil {
		return nil, nil, err
	}
	if !leaf.Mode().IsRegular() {
		return nil, nil, errors.New("log path is not a regular file")
	}
	file, err := os.Open(target)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	current, err := os.Lstat(target)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(info, current) {
		file.Close()
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, errors.New("log file changed during open")
	}
	return file, info, nil
}

func validateInstanceID(instanceID string) error {
	if instanceID == "" || filepath.Base(instanceID) != instanceID || instanceID == "." || instanceID == ".." || strings.IndexByte(instanceID, 0) >= 0 {
		return errors.New("invalid instance id")
	}
	return nil
}

func (m *Manager) logRoot(instanceID, kind string) string {
	return filepath.Join(m.root, "instances", instanceID, "logs", kind)
}

func (m *Manager) kindRoot(instanceID, kind string) (string, error) {
	if kind != "game" && kind != "sourcemod" {
		return "", errors.New("invalid log kind")
	}
	return m.logRoot(instanceID, kind), nil
}

func (m *Manager) Prepare(ctx context.Context, instanceID string) error {
	if err := validateInstanceID(instanceID); err != nil {
		return err
	}
	instanceLock := m.instanceLock(instanceID)
	instanceLock.Lock()
	defer instanceLock.Unlock()
	base := filepath.Join(m.root, "instances", instanceID)
	destinations := []struct {
		source, destination string
	}{
		{filepath.Join(base, "overlay", "merged", "left4dead2", "logs"), filepath.Join(base, "logs", "game")},
		{filepath.Join(base, "overlay", "merged", "left4dead2", "addons", "sourcemod", "logs"), filepath.Join(base, "logs", "sourcemod")},
		{filepath.Join(base, "overlay", "upper", "left4dead2", "logs"), filepath.Join(base, "logs", "game")},
		{filepath.Join(base, "overlay", "upper", "left4dead2", "addons", "sourcemod", "logs"), filepath.Join(base, "logs", "sourcemod")},
	}
	for _, item := range destinations {
		if err := secureMkdirAll(m.root, item.destination); err != nil {
			return fmt.Errorf("prepare persistent log directory: %w", err)
		}
	}
	stamp := m.now().UTC().Format("20060102T150405Z")
	for _, item := range destinations {
		if err := migrateTree(ctx, m.root, item.source, item.destination, stamp); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) instanceLock(instanceID string) *sync.Mutex {
	m.locksMu.Lock()
	defer m.locksMu.Unlock()
	lock := m.instances[instanceID]
	if lock == nil {
		lock = &sync.Mutex{}
		m.instances[instanceID] = lock
	}
	return lock
}

func migrateTree(ctx context.Context, anchor, sourceRoot, destinationRoot, stamp string) error {
	exists, err := validateDirectoryPath(anchor, sourceRoot, true)
	if err != nil {
		return fmt.Errorf("inspect legacy log root: %w", err)
	}
	if !exists {
		return nil
	}
	return filepath.WalkDir(sourceRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk legacy logs: %w", walkErr)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == sourceRoot {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect legacy log entry: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.Mode().IsRegular() && !info.IsDir()) {
			return fmt.Errorf("legacy log entry is not a regular file or directory: %s", path)
		}
		relative, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(destinationRoot, relative)
		if info.IsDir() {
			if err := secureMkdirAll(destinationRoot, destination); err != nil {
				return fmt.Errorf("create migrated log directory: %w", err)
			}
			return nil
		}
		return copyUnique(path, destination, stamp, info.Mode().Perm())
	})
}

func copyUnique(source, destination, stamp string, mode os.FileMode) error {
	for {
		candidate := destination
		destinationExists, err := validateRegularLeaf(destination)
		if err != nil {
			return err
		}
		if destinationExists {
			same, err := sameFileContent(source, destination)
			if err != nil {
				return err
			}
			if same {
				return nil
			}
			match, err := findMatchingConflict(source, destination)
			if err != nil || match {
				return err
			}
			candidate, err = nextConflictName(destination, stamp)
			if err != nil {
				return err
			}
		}
		err = copyAtomic(source, candidate, mode)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return err
	}
}

func validateRegularLeaf(path string) (bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("persistent log leaf is not a regular file: %s", path)
	}
	return true, nil
}

func sameFileContent(left, right string) (bool, error) {
	leftInfo, err := os.Stat(left)
	if err != nil {
		return false, err
	}
	rightInfo, err := os.Stat(right)
	if err != nil {
		return false, err
	}
	if leftInfo.Size() != rightInfo.Size() {
		return false, nil
	}
	leftFile, err := os.Open(left)
	if err != nil {
		return false, err
	}
	defer leftFile.Close()
	rightFile, err := os.Open(right)
	if err != nil {
		return false, err
	}
	defer rightFile.Close()
	leftBuffer := make([]byte, 64*1024)
	rightBuffer := make([]byte, len(leftBuffer))
	remaining := leftInfo.Size()
	for remaining > 0 {
		chunk := int64(len(leftBuffer))
		if remaining < chunk {
			chunk = remaining
		}
		if _, err := io.ReadFull(leftFile, leftBuffer[:chunk]); err != nil {
			return false, err
		}
		if _, err := io.ReadFull(rightFile, rightBuffer[:chunk]); err != nil {
			return false, err
		}
		if !bytes.Equal(leftBuffer[:chunk], rightBuffer[:chunk]) {
			return false, nil
		}
		remaining -= chunk
	}
	return true, nil
}

func findMatchingConflict(source, destination string) (bool, error) {
	extension := filepath.Ext(destination)
	base := strings.TrimSuffix(filepath.Base(destination), extension)
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(destination), base+".migrated-*-*"+extension))
	if err != nil {
		return false, err
	}
	for _, candidate := range matches {
		if _, err := validateRegularLeaf(candidate); err != nil {
			return false, err
		}
		same, err := sameFileContent(source, candidate)
		if err != nil {
			return false, err
		}
		if same {
			return true, nil
		}
	}
	return false, nil
}

func nextConflictName(destination, stamp string) (string, error) {
	extension := filepath.Ext(destination)
	base := strings.TrimSuffix(destination, extension)
	for sequence := 1; ; sequence++ {
		candidate := fmt.Sprintf("%s.migrated-%s-%03d%s", base, stamp, sequence, extension)
		_, err := os.Lstat(candidate)
		if os.IsNotExist(err) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
	}
}

func secureMkdirAll(anchor, destination string) error {
	_, err := validateDirectoryPath(anchor, destination, false)
	return err
}

func validateDirectoryPath(anchor, destination string, allowMissing bool) (bool, error) {
	relative, err := filepath.Rel(anchor, destination)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return false, errors.New("persistent log directory escapes controlled root")
	}
	current := anchor
	if info, err := os.Lstat(current); err != nil {
		return false, err
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("controlled root is not a regular directory: %s", current)
	}
	if relative == "." {
		return true, nil
	}
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			if allowMissing {
				return false, nil
			}
			if err := os.Mkdir(current, 0o750); err != nil {
				if info, statErr := os.Lstat(current); statErr == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
					continue
				}
				return false, err
			}
			continue
		}
		if err != nil {
			return false, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false, fmt.Errorf("persistent log component is not a regular directory: %s", current)
		}
	}
	return true, nil
}

func copyAtomic(source, destination string, mode os.FileMode) (err error) {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open legacy log: %w", err)
	}
	defer input.Close()
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".migrate-*")
	if err != nil {
		return fmt.Errorf("create migrated log: %w", err)
	}
	temporaryName := temporary.Name()
	defer func() {
		temporary.Close()
		_ = os.Remove(temporaryName)
	}()
	if _, err = io.Copy(temporary, input); err != nil {
		return fmt.Errorf("copy legacy log: %w", err)
	}
	if err = temporary.Chmod(mode); err != nil {
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	if err = os.Link(temporaryName, destination); err != nil {
		return fmt.Errorf("publish migrated log: %w", err)
	}
	return nil
}

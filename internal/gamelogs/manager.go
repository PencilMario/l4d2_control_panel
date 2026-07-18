package gamelogs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Options struct {
	Now func() time.Time
}

type Manager struct {
	root string
	now  func() time.Time
}

func NewManager(root string, options Options) *Manager {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Manager{root: root, now: now}
}

func (m *Manager) Prepare(ctx context.Context, instanceID string) error {
	if instanceID == "" || filepath.Base(instanceID) != instanceID || instanceID == "." || instanceID == ".." {
		return errors.New("invalid instance id")
	}
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
		if err := migrateTree(ctx, item.source, item.destination, stamp); err != nil {
			return err
		}
	}
	return nil
}

func migrateTree(ctx context.Context, sourceRoot, destinationRoot, stamp string) error {
	info, err := os.Lstat(sourceRoot)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect legacy log root: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("legacy log root is not a regular directory: %s", sourceRoot)
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
	same, err := sameFileContent(source, destination)
	if err == nil && same {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		match, err := findMatchingConflict(source, destination)
		if err != nil || match {
			return err
		}
		destination, err = nextConflictName(destination, stamp)
		if err != nil {
			return err
		}
	}
	return copyAtomic(source, destination, mode)
}

func sameFileContent(left, right string) (bool, error) {
	a, err := os.ReadFile(left)
	if err != nil {
		return false, err
	}
	b, err := os.ReadFile(right)
	if err != nil {
		return false, err
	}
	return bytes.Equal(a, b), nil
}

func findMatchingConflict(source, destination string) (bool, error) {
	extension := filepath.Ext(destination)
	base := strings.TrimSuffix(filepath.Base(destination), extension)
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(destination), base+".migrated-*-*"+extension))
	if err != nil {
		return false, err
	}
	for _, candidate := range matches {
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
	relative, err := filepath.Rel(anchor, destination)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return errors.New("persistent log directory escapes controlled root")
	}
	current := anchor
	if info, err := os.Lstat(current); err != nil {
		return err
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("controlled root is not a regular directory: %s", current)
	}
	if relative == "." {
		return nil
	}
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			if err := os.Mkdir(current, 0o750); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("persistent log component is not a regular directory: %s", current)
		}
	}
	return nil
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
		if err != nil {
			_ = os.Remove(temporaryName)
		}
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
	if err = os.Rename(temporaryName, destination); err != nil {
		return fmt.Errorf("publish migrated log: %w", err)
	}
	return nil
}

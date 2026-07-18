package metrics

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
)

type DirectoryStorage struct{ Root string }

func (s DirectoryStorage) InstanceStorage(ctx context.Context, id string) (StorageUsage, error) {
	base := filepath.Join(s.Root, "instances", id)
	values := []*uint64{}
	usage := StorageUsage{}
	values = append(values, &usage.Game, &usage.Private, &usage.Backups, &usage.Console)
	paths := []string{instanceGameStoragePath(base), filepath.Join(base, "private"), filepath.Join(base, "backups"), filepath.Join(base, "console")}
	for index, path := range paths {
		size, err := directorySize(ctx, path)
		if err != nil {
			return StorageUsage{}, err
		}
		*values[index] = size
	}
	return usage, nil
}

func instanceGameStoragePath(base string) string {
	gamePath := filepath.Join(base, "game")
	info, err := os.Lstat(gamePath)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return gamePath
	}
	target, err := os.Readlink(gamePath)
	if err != nil {
		return gamePath
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(gamePath), target)
	}
	if filepath.Clean(target) == filepath.Join(base, "overlay", "merged") {
		return filepath.Join(base, "overlay", "upper")
	}
	return gamePath
}

func directorySize(ctx context.Context, root string) (uint64, error) {
	var total uint64
	err := filepath.WalkDir(root, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			total += uint64(info.Size())
		}
		return nil
	})
	if os.IsNotExist(err) {
		return 0, nil
	}
	return total, err
}

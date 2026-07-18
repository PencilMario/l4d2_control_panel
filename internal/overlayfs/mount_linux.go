//go:build linux

package overlayfs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type SystemMounter struct{}

func (SystemMounter) Preflight(context.Context) error {
	raw, err := os.ReadFile("/proc/filesystems")
	if err != nil {
		return err
	}
	if !strings.Contains(string(raw), "overlay") {
		return errors.New("overlay filesystem is unavailable")
	}
	return nil
}

func (m SystemMounter) Ensure(ctx context.Context, mount Mount) error {
	if err := m.Preflight(ctx); err != nil {
		return err
	}
	if info, err := os.Stat(mount.Lower); err != nil || !info.IsDir() {
		return fmt.Errorf("lower release is unavailable: %w", err)
	}
	for _, directory := range []string{mount.Upper, mount.Work, mount.Merged} {
		if err := os.MkdirAll(directory, 0o770); err != nil {
			return err
		}
		if err := os.Chmod(directory, 0o770); err != nil {
			return err
		}
	}
	status, err := m.Inspect(ctx, mount)
	if err != nil {
		return err
	}
	if status.Mounted {
		if status.Lower == mount.Lower {
			return nil
		}
		return errors.New("merged path is mounted with another lower release")
	}
	options := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", mount.Lower, mount.Upper, mount.Work)
	return unix.Mount("overlay", mount.Merged, "overlay", 0, options)
}

func (SystemMounter) Inspect(_ context.Context, mount Mount) (MountStatus, error) {
	file, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return MountStatus{}, err
	}
	defer file.Close()
	merged := filepath.Clean(mount.Merged)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		separator := -1
		for index, field := range fields {
			if field == "-" {
				separator = index
				break
			}
		}
		if separator < 0 || len(fields) <= separator+3 || unescapeMountPath(fields[4]) != merged || fields[separator+1] != "overlay" {
			continue
		}
		return MountStatus{Mounted: true, Lower: overlayOption(fields[separator+3], "lowerdir")}, nil
	}
	return MountStatus{}, scanner.Err()
}

func (m SystemMounter) ResetManagedPaths(ctx context.Context, mount Mount, paths []string) error {
	if err := m.Unmount(ctx, mount); err != nil {
		return err
	}
	for _, managedPath := range paths {
		target := filepath.Join(mount.Upper, filepath.FromSlash(managedPath))
		if err := os.RemoveAll(target); err != nil {
			return err
		}
		whiteout := filepath.Join(filepath.Dir(target), ".wh."+filepath.Base(target))
		if err := os.Remove(whiteout); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return m.Ensure(ctx, mount)
}

func (m SystemMounter) ResetUpper(ctx context.Context, mount Mount) error {
	if err := m.Unmount(ctx, mount); err != nil {
		return err
	}
	if err := os.RemoveAll(mount.Upper); err != nil {
		return err
	}
	if err := os.RemoveAll(mount.Work); err != nil {
		return err
	}
	return m.Ensure(ctx, mount)
}

func (SystemMounter) Unmount(ctx context.Context, mount Mount) error {
	status, err := (SystemMounter{}).Inspect(ctx, mount)
	if err != nil || !status.Mounted {
		return err
	}
	return unix.Unmount(mount.Merged, 0)
}

func overlayOption(options, name string) string {
	for _, option := range strings.Split(options, ",") {
		key, value, found := strings.Cut(option, "=")
		if found && key == name {
			return unescapeMountPath(value)
		}
	}
	return ""
}

func unescapeMountPath(value string) string {
	replacer := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return filepath.Clean(replacer.Replace(value))
}

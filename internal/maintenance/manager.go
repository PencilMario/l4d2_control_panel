package maintenance

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Manager struct{ root string }

func New(root string) *Manager { return &Manager{root: root} }
func (m *Manager) Backup(ctx context.Context, instanceID string) (string, error) {
	if filepath.Base(instanceID) != instanceID || instanceID == "" {
		return "", errors.New("invalid instance id")
	}
	base := filepath.Join(m.root, "instances", instanceID)
	backupDir := filepath.Join(base, "backups")
	if err := os.MkdirAll(backupDir, 0750); err != nil {
		return "", err
	}
	target := filepath.Join(backupDir, "backup-"+time.Now().UTC().Format("20060102T150405.000000000")+".tar.gz")
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		return "", err
	}
	gzipWriter := gzip.NewWriter(file)
	writer := tar.NewWriter(gzipWriter)
	sources := []string{filepath.Join(base, "private"), filepath.Join(base, "package-manifest.json")}
	for _, source := range sources {
		if _, err := os.Stat(source); errors.Is(err, os.ErrNotExist) {
			continue
		}
		err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return errors.New("backup refuses symbolic links")
			}
			relative, err := filepath.Rel(base, path)
			if err != nil {
				return err
			}
			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			header.Name = filepath.ToSlash(relative)
			if err := writer.WriteHeader(header); err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			input, err := os.Open(path)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(writer, input)
			closeErr := input.Close()
			if copyErr != nil {
				return copyErr
			}
			return closeErr
		})
		if err != nil {
			writer.Close()
			gzipWriter.Close()
			file.Close()
			return "", err
		}
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	if err := gzipWriter.Close(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return target, nil
}
func (m *Manager) Cleanup(ctx context.Context, retention time.Duration) (int, error) {
	cutoff := time.Now().Add(-retention)
	removed := 0
	roots := []string{filepath.Join(m.root, "instances"), filepath.Join(m.root, "packages", "uploads")}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if entry.IsDir() {
				return nil
			}
			cleanable := strings.Contains(filepath.ToSlash(path), "/backups/") || strings.HasSuffix(path, ".part") || strings.HasSuffix(path, ".upload")
			if !cleanable {
				return nil
			}
			info, err := entry.Info()
			if err == nil && info.ModTime().Before(cutoff) {
				if os.Remove(path) == nil {
					removed++
				}
			}
			return nil
		})
	}
	return removed, ctx.Err()
}

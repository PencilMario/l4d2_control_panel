package content

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	archivecheck "github.com/not0721here/l4d2-control-panel/internal/archive"
	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
)

type PackageManager struct {
	directory string
	remove    func(string) error
}

type PackageCleanupResult struct {
	Scanned        int
	KeptLatest     int
	KeptReferenced int
	Deleted        int
	ReleasedBytes  int64
	Failed         int
}
type PackageVersion struct {
	ID               string               `json:"id"`
	Filename         string               `json:"filename"`
	Version          string               `json:"version"`
	SourceRepository string               `json:"source_repository,omitempty"`
	Hash             string               `json:"sha256"`
	Size             int64                `json:"size"`
	HotCompatible    bool                 `json:"hot_compatible"`
	Files            []archivecheck.Entry `json:"files"`
	ArchivePath      string               `json:"-"`
	CreatedAt        time.Time            `json:"created_at"`
}

func (m *PackageManager) FindSourceVersion(repository, version, filename string) (PackageVersion, bool, error) {
	items, err := m.List()
	if err != nil {
		return PackageVersion{}, false, err
	}
	for _, item := range items {
		if item.SourceRepository == repository && item.Version == version && item.Filename == filename {
			return item, true, nil
		}
	}
	return PackageVersion{}, false, nil
}

func (m *PackageManager) LatestSourceVersion(repository string) (PackageVersion, error) {
	items, err := m.List()
	if err != nil {
		return PackageVersion{}, err
	}
	var latest PackageVersion
	for _, item := range items {
		if item.SourceRepository != repository || (latest.ID != "" && !item.CreatedAt.After(latest.CreatedAt)) {
			continue
		}
		latest = item
	}
	if latest.ID == "" {
		return PackageVersion{}, os.ErrNotExist
	}
	return latest, nil
}
func NewPackageManager(root string) (*PackageManager, error) {
	directory := filepath.Join(root, "packages", "releases")
	if err := os.MkdirAll(directory, 0750); err != nil {
		return nil, err
	}
	return &PackageManager{directory: directory, remove: os.Remove}, nil
}

func (m *PackageManager) CleanupUnreferencedSourceVersions(ctx context.Context, protected map[string]bool) (PackageCleanupResult, error) {
	items, err := m.List()
	if err != nil {
		return PackageCleanupResult{}, err
	}
	result := PackageCleanupResult{Scanned: len(items)}
	latest := make(map[string]PackageVersion)
	for _, item := range items {
		if item.SourceRepository == "" {
			continue
		}
		current, ok := latest[item.SourceRepository]
		if !ok || item.CreatedAt.After(current.CreatedAt) || (item.CreatedAt.Equal(current.CreatedAt) && item.ID > current.ID) {
			latest[item.SourceRepository] = item
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	var cleanupErr error
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return result, errors.Join(cleanupErr, err)
		}
		if item.SourceRepository == "" {
			continue
		}
		if protected[item.ID] {
			result.KeptReferenced++
			continue
		}
		if latest[item.SourceRepository].ID == item.ID {
			result.KeptLatest++
			continue
		}
		var size int64
		if info, statErr := os.Stat(item.ArchivePath); statErr == nil {
			size = info.Size()
		}
		if err := m.remove(item.ArchivePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			result.Failed++
			safeErr := jobs.SafeError(err, item.ArchivePath, m.directory)
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("delete package %s archive: %s", item.ID, safeErr))
			jobs.Logf(ctx, "cleanup", joblogs.Error, "package delete failed package_id=%s repository=%s version=%s archive=%s error=%q", item.ID, item.SourceRepository, item.Version, filepath.Base(item.ArchivePath), safeErr)
			continue
		}
		result.ReleasedBytes += size
		metadata := filepath.Join(m.directory, item.ID+".json")
		if err := m.remove(metadata); err != nil && !errors.Is(err, os.ErrNotExist) {
			result.Failed++
			safeErr := jobs.SafeError(err, metadata, m.directory)
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("delete package %s metadata: %s", item.ID, safeErr))
			jobs.Logf(ctx, "cleanup", joblogs.Error, "package metadata delete failed package_id=%s repository=%s version=%s error=%q", item.ID, item.SourceRepository, item.Version, safeErr)
			continue
		}
		result.Deleted++
		jobs.Logf(ctx, "cleanup", joblogs.Info, "deleted package package_id=%s repository=%s version=%s archive=%s released=%s", item.ID, item.SourceRepository, item.Version, filepath.Base(item.ArchivePath), jobs.FormatBytes(size))
	}
	return result, cleanupErr
}
func (m *PackageManager) CreateDownloadTemp() (*os.File, error) {
	directory := filepath.Join(filepath.Dir(m.directory), "uploads")
	if err := os.MkdirAll(directory, 0750); err != nil {
		return nil, err
	}
	return os.CreateTemp(directory, "release-*.part")
}
func (m *PackageManager) AddUpload(filename, version string, reader io.Reader, size int64) (PackageVersion, error) {
	if filepath.Base(filename) != filename || strings.ToLower(filepath.Ext(filename)) != ".zip" || version == "" || size < 1 {
		return PackageVersion{}, errors.New("safe ZIP filename, version and size required")
	}
	id := uuid.NewString()
	temporary := filepath.Join(m.directory, id+".upload")
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		return PackageVersion{}, err
	}
	digest := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, digest), io.LimitReader(reader, size+1))
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(temporary)
		return PackageVersion{}, copyErr
	}
	if closeErr != nil {
		return PackageVersion{}, closeErr
	}
	if written != size {
		_ = os.Remove(temporary)
		return PackageVersion{}, errors.New("package size mismatch")
	}
	inspected, err := archivecheck.InspectZip(temporary, archivecheck.Limits{MaxFiles: 20000, MaxBytes: 8 << 30})
	if err != nil {
		_ = os.Remove(temporary)
		return PackageVersion{}, err
	}
	archivePath := filepath.Join(m.directory, id+".zip")
	if err := os.Rename(temporary, archivePath); err != nil {
		return PackageVersion{}, err
	}
	item := PackageVersion{ID: id, Filename: filename, Version: version, Hash: hex.EncodeToString(digest.Sum(nil)), Size: written, HotCompatible: inspected.HotCompatible, Files: inspected.Entries, ArchivePath: archivePath, CreatedAt: time.Now().UTC()}
	if err := m.save(item); err != nil {
		return PackageVersion{}, err
	}
	return item, nil
}
func (m *PackageManager) Get(id string) (PackageVersion, error) {
	if _, err := uuid.Parse(id); err != nil {
		return PackageVersion{}, errors.New("invalid package id")
	}
	raw, err := os.ReadFile(filepath.Join(m.directory, id+".json"))
	if err != nil {
		return PackageVersion{}, err
	}
	var item PackageVersion
	if err := json.Unmarshal(raw, &item); err != nil {
		return PackageVersion{}, err
	}
	item.ArchivePath = filepath.Join(m.directory, id+".zip")
	return item, nil
}
func (m *PackageManager) UpdateMetadata(item PackageVersion) error { return m.save(item) }
func (m *PackageManager) List() ([]PackageVersion, error) {
	entries, err := os.ReadDir(m.directory)
	if err != nil {
		return nil, err
	}
	result := []PackageVersion{}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		item, err := m.Get(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}
func (m *PackageManager) save(item PackageVersion) error {
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	path := filepath.Join(m.directory, item.ID+".json")
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, raw, 0640); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

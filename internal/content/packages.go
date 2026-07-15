package content

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	archivecheck "github.com/not0721here/l4d2-control-panel/internal/archive"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type PackageManager struct{ directory string }
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

func NewPackageManager(root string) (*PackageManager, error) {
	directory := filepath.Join(root, "packages", "releases")
	if err := os.MkdirAll(directory, 0750); err != nil {
		return nil, err
	}
	return &PackageManager{directory: directory}, nil
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

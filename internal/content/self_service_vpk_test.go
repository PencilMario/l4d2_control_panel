package content

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/store"
)

type fakeSelfServiceVPKStore struct {
	settings  store.SelfServiceVPKSettings
	items     map[string]store.SelfServiceVPK
	deleteErr error
}

func (f *fakeSelfServiceVPKStore) SelfServiceVPKSettings() (store.SelfServiceVPKSettings, error) {
	return f.settings, nil
}
func (f *fakeSelfServiceVPKStore) SaveSelfServiceVPK(item store.SelfServiceVPK) error {
	if f.items == nil {
		f.items = map[string]store.SelfServiceVPK{}
	}
	f.items[item.Name] = item
	return nil
}
func (f *fakeSelfServiceVPKStore) ListSelfServiceVPKs(limit, offset int) ([]store.SelfServiceVPK, int, error) {
	return nil, len(f.items), nil
}
func (f *fakeSelfServiceVPKStore) ExpiredSelfServiceVPKs(now time.Time) ([]store.SelfServiceVPK, error) {
	result := []store.SelfServiceVPK{}
	for _, item := range f.items {
		if item.ExpiresAt.Before(now) {
			result = append(result, item)
		}
	}
	return result, nil
}
func (f *fakeSelfServiceVPKStore) DeleteSelfServiceVPK(name string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.items, name)
	return nil
}
func (f *fakeSelfServiceVPKStore) RenameSelfServiceVPK(oldName, newName string) error {
	if item, ok := f.items[oldName]; ok {
		delete(f.items, oldName)
		item.Name = newName
		f.items[newName] = item
	}
	return nil
}
func (f *fakeSelfServiceVPKStore) UpdateSelfServiceVPKSize(name string, size int64) error {
	item := f.items[name]
	item.Size = size
	f.items[name] = item
	return nil
}

func writeManagedVPK(t *testing.T, uploads *UploadManager, name string) {
	t.Helper()
	path := filepath.Join(uploads.shared, name)
	if err := os.WriteFile(path, []byte("vpk"), 0o640); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(SharedVPK{Name: name, Size: 3, Path: path})
	if err := os.WriteFile(path+".json", raw, 0o640); err != nil {
		t.Fatal(err)
	}
}

func TestSelfServiceVPKCleanupDeletesExpiredFilesAndMetadata(t *testing.T) {
	uploads, _ := NewUploadManager(t.TempDir())
	writeManagedVPK(t, uploads, "old.vpk")
	now := time.Now().UTC()
	repo := &fakeSelfServiceVPKStore{settings: store.SelfServiceVPKSettings{AutoDelete: true}, items: map[string]store.SelfServiceVPK{"old.vpk": {Name: "old.vpk", ExpiresAt: now.Add(-time.Hour)}}}
	result, err := NewSelfServiceVPKManager(repo, uploads).CleanupExpired(now)
	if err != nil || result.Deleted != 1 || len(result.Failures) != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if _, err := os.Stat(filepath.Join(uploads.shared, "old.vpk")); !os.IsNotExist(err) {
		t.Fatalf("expired file remains: %v", err)
	}
	if len(repo.items) != 0 {
		t.Fatalf("metadata remains: %+v", repo.items)
	}
}

func TestSelfServiceVPKCleanupPausesAndRetriesFailures(t *testing.T) {
	uploads, _ := NewUploadManager(t.TempDir())
	writeManagedVPK(t, uploads, "old.vpk")
	now := time.Now().UTC()
	repo := &fakeSelfServiceVPKStore{settings: store.SelfServiceVPKSettings{AutoDelete: false}, items: map[string]store.SelfServiceVPK{"old.vpk": {Name: "old.vpk", ExpiresAt: now.Add(-time.Hour)}}}
	result, err := NewSelfServiceVPKManager(repo, uploads).CleanupExpired(now)
	if err != nil || !result.Paused || result.Deleted != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	repo.settings.AutoDelete = true
	repo.deleteErr = errors.New("database unavailable")
	result, err = NewSelfServiceVPKManager(repo, uploads).CleanupExpired(now)
	if err == nil || result.Deleted != 0 || len(result.Failures) != 1 || len(repo.items) != 1 {
		t.Fatalf("result=%+v err=%v items=%+v", result, err, repo.items)
	}
}

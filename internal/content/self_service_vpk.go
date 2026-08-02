package content

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/store"
)

type SelfServiceVPKStore interface {
	SelfServiceVPKSettings() (store.SelfServiceVPKSettings, error)
	SaveSelfServiceVPK(store.SelfServiceVPK) error
	ListSelfServiceVPKs(limit, offset int) ([]store.SelfServiceVPK, int, error)
	ExpiredSelfServiceVPKs(time.Time) ([]store.SelfServiceVPK, error)
	DeleteSelfServiceVPK(name string) error
	RenameSelfServiceVPK(oldName, newName string) error
	UpdateSelfServiceVPKSize(name string, size int64) error
}

type SelfServiceVPKManager struct {
	store   SelfServiceVPKStore
	uploads *UploadManager
}

type SelfServiceVPKCleanupResult struct {
	Scanned  int      `json:"scanned"`
	Deleted  int      `json:"deleted"`
	Paused   bool     `json:"paused"`
	Failures []string `json:"failures"`
}

func NewSelfServiceVPKManager(repo SelfServiceVPKStore, uploads *UploadManager) *SelfServiceVPKManager {
	return &SelfServiceVPKManager{store: repo, uploads: uploads}
}

func (m *SelfServiceVPKManager) Complete(id string, clean bool, now time.Time) (SharedVPK, error) {
	item, duplicate, err := m.uploads.Complete(id)
	if err != nil {
		return SharedVPK{}, err
	}
	if duplicate {
		return SharedVPK{}, errors.New("VPK content already exists")
	}
	if clean {
		result, cleanErr := m.uploads.Clean(item.Name)
		if cleanErr != nil {
			_ = m.uploads.Delete(item.Name)
			return SharedVPK{}, cleanErr
		}
		item.Size = result.AfterSize
		items, listErr := m.uploads.List()
		if listErr == nil {
			for _, current := range items {
				if current.Name == item.Name {
					item = current
					break
				}
			}
		}
	}
	settings, err := m.store.SelfServiceVPKSettings()
	if err != nil {
		_ = m.uploads.Delete(item.Name)
		return SharedVPK{}, err
	}
	meta := store.SelfServiceVPK{Name: item.Name, Size: item.Size, UploadedAt: now.UTC(), ExpiresAt: now.UTC().Add(time.Duration(settings.RetentionDays) * 24 * time.Hour)}
	if err = m.store.SaveSelfServiceVPK(meta); err != nil {
		_ = m.uploads.Delete(item.Name)
		return SharedVPK{}, err
	}
	return item, nil
}

func (m *SelfServiceVPKManager) List(limit, offset int) ([]store.SelfServiceVPK, int, error) {
	return m.store.ListSelfServiceVPKs(limit, offset)
}

func (m *SelfServiceVPKManager) Rename(oldName, newName string) (SharedVPK, error) {
	item, err := m.uploads.Rename(oldName, newName)
	if err != nil {
		return SharedVPK{}, err
	}
	if err = m.store.RenameSelfServiceVPK(oldName, newName); err != nil {
		_, _ = m.uploads.Rename(newName, oldName)
		return SharedVPK{}, err
	}
	return item, nil
}

func (m *SelfServiceVPKManager) Clean(name string) (VPKCleanupResult, error) {
	result, err := m.uploads.Clean(name)
	if err != nil {
		return VPKCleanupResult{}, err
	}
	if err = m.store.UpdateSelfServiceVPKSize(name, result.AfterSize); err != nil {
		return VPKCleanupResult{}, err
	}
	return result, nil
}

func (m *SelfServiceVPKManager) Delete(name string) error {
	if err := m.uploads.Delete(name); err != nil {
		return err
	}
	return m.store.DeleteSelfServiceVPK(name)
}

func (m *SelfServiceVPKManager) CleanupExpired(now time.Time) (SelfServiceVPKCleanupResult, error) {
	settings, err := m.store.SelfServiceVPKSettings()
	if err != nil {
		return SelfServiceVPKCleanupResult{}, err
	}
	if !settings.AutoDelete {
		return SelfServiceVPKCleanupResult{Paused: true, Failures: []string{}}, nil
	}
	items, err := m.store.ExpiredSelfServiceVPKs(now)
	if err != nil {
		return SelfServiceVPKCleanupResult{}, err
	}
	result := SelfServiceVPKCleanupResult{Scanned: len(items), Failures: []string{}}
	for _, item := range items {
		deleteErr := m.uploads.Delete(item.Name)
		if deleteErr != nil && !os.IsNotExist(deleteErr) {
			result.Failures = append(result.Failures, fmt.Sprintf("%s: %v", item.Name, deleteErr))
			continue
		}
		if deleteErr = m.store.DeleteSelfServiceVPK(item.Name); deleteErr != nil {
			result.Failures = append(result.Failures, fmt.Sprintf("%s metadata: %v", item.Name, deleteErr))
			continue
		}
		result.Deleted++
	}
	if len(result.Failures) > 0 {
		return result, errors.New("one or more expired self-service VPKs could not be deleted")
	}
	return result, nil
}

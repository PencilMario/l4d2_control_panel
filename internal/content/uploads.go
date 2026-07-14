package content

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type UploadManager struct {
	root, shared, temp string
	mu                 sync.Mutex
}
type UploadSession struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Hash   string `json:"hash"`
	Size   int64  `json:"size"`
	Offset int64  `json:"offset"`
}
type SharedVPK struct {
	Name string `json:"name"`
	Hash string `json:"hash"`
	Path string `json:"-"`
	Size int64  `json:"size"`
}

func NewUploadManager(root string) (*UploadManager, error) {
	m := &UploadManager{root: root, shared: filepath.Join(root, "shared-vpk"), temp: filepath.Join(root, "packages", "uploads")}
	for _, path := range []string{m.shared, m.temp} {
		if err := os.MkdirAll(path, 0750); err != nil {
			return nil, err
		}
	}
	return m, nil
}
func (m *UploadManager) Begin(name string, size int64, hash string) (UploadSession, error) {
	if filepath.Base(name) != name || strings.ToLower(filepath.Ext(name)) != ".vpk" || size < 1 {
		return UploadSession{}, errors.New("safe .vpk filename and positive size required")
	}
	decoded, err := hex.DecodeString(hash)
	if err != nil || len(decoded) != sha256.Size {
		return UploadSession{}, errors.New("valid SHA-256 required")
	}
	session := UploadSession{ID: uuid.NewString(), Name: name, Size: size, Hash: strings.ToLower(hash)}
	if err := m.save(session); err != nil {
		return UploadSession{}, err
	}
	file, err := os.OpenFile(m.part(session.ID), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		return UploadSession{}, err
	}
	_ = file.Close()
	return session, nil
}
func (m *UploadManager) Write(id string, offset int64, reader io.Reader) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, err := m.load(id)
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(m.part(id))
	if err != nil {
		return 0, err
	}
	if offset != info.Size() || offset != session.Offset {
		return 0, fmt.Errorf("expected offset %d", session.Offset)
	}
	file, err := os.OpenFile(m.part(id), os.O_WRONLY|os.O_APPEND, 0640)
	if err != nil {
		return 0, err
	}
	limited := io.LimitReader(reader, session.Size-session.Offset+1)
	written, copyErr := io.Copy(file, limited)
	closeErr := file.Close()
	if copyErr != nil {
		return written, copyErr
	}
	if closeErr != nil {
		return written, closeErr
	}
	session.Offset += written
	if session.Offset > session.Size {
		return written, errors.New("upload exceeds declared size")
	}
	return written, m.save(session)
}
func (m *UploadManager) Complete(id string) (SharedVPK, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session, err := m.load(id)
	if err != nil {
		return SharedVPK{}, false, err
	}
	if session.Offset != session.Size {
		return SharedVPK{}, false, errors.New("upload incomplete")
	}
	file, err := os.Open(m.part(id))
	if err != nil {
		return SharedVPK{}, false, err
	}
	digest := sha256.New()
	written, err := io.Copy(digest, file)
	_ = file.Close()
	if err != nil {
		return SharedVPK{}, false, err
	}
	actual := hex.EncodeToString(digest.Sum(nil))
	if actual != session.Hash {
		return SharedVPK{}, false, errors.New("SHA-256 mismatch")
	}
	entries, _ := os.ReadDir(m.shared)
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".json" {
			var item SharedVPK
			raw, _ := os.ReadFile(filepath.Join(m.shared, entry.Name()))
			if json.Unmarshal(raw, &item) == nil && item.Hash == actual {
				m.cleanup(id)
				return item, true, nil
			}
		}
	}
	target := filepath.Join(m.shared, session.Name)
	if _, err := os.Stat(target); err == nil {
		return SharedVPK{}, false, errors.New("target VPK already exists")
	}
	if err := os.Rename(m.part(id), target); err != nil {
		return SharedVPK{}, false, err
	}
	item := SharedVPK{Name: session.Name, Hash: actual, Size: written, Path: target}
	raw, _ := json.Marshal(item)
	if err := os.WriteFile(target+".json", raw, 0640); err != nil {
		return SharedVPK{}, false, err
	}
	_ = os.Remove(m.meta(id))
	return item, false, nil
}
func (m *UploadManager) meta(id string) string { return filepath.Join(m.temp, id+".json") }
func (m *UploadManager) part(id string) string { return filepath.Join(m.temp, id+".part") }
func (m *UploadManager) save(session UploadSession) error {
	raw, err := json.Marshal(session)
	if err != nil {
		return err
	}
	temporary := m.meta(session.ID) + ".tmp"
	if err := os.WriteFile(temporary, raw, 0640); err != nil {
		return err
	}
	return os.Rename(temporary, m.meta(session.ID))
}
func (m *UploadManager) load(id string) (UploadSession, error) {
	if _, err := uuid.Parse(id); err != nil {
		return UploadSession{}, errors.New("invalid upload id")
	}
	raw, err := os.ReadFile(m.meta(id))
	if err != nil {
		return UploadSession{}, err
	}
	var session UploadSession
	err = json.Unmarshal(raw, &session)
	return session, err
}
func (m *UploadManager) cleanup(id string) { _ = os.Remove(m.meta(id)); _ = os.Remove(m.part(id)) }
func (m *UploadManager) List() ([]SharedVPK, error) {
	entries, err := os.ReadDir(m.shared)
	if err != nil {
		return nil, err
	}
	result := []SharedVPK{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".vpk.json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(m.shared, entry.Name()))
		if err != nil {
			return nil, err
		}
		var item SharedVPK
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		item.Path = filepath.Join(m.shared, item.Name)
		result = append(result, item)
	}
	return result, nil
}
func (m *UploadManager) Rename(oldName, newName string) (SharedVPK, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !safeVPKName(oldName) || !safeVPKName(newName) {
		return SharedVPK{}, errors.New("safe .vpk names required")
	}
	oldPath, newPath := filepath.Join(m.shared, oldName), filepath.Join(m.shared, newName)
	if _, err := os.Stat(newPath); err == nil {
		return SharedVPK{}, errors.New("target exists")
	}
	raw, err := os.ReadFile(oldPath + ".json")
	if err != nil {
		return SharedVPK{}, err
	}
	var item SharedVPK
	if err := json.Unmarshal(raw, &item); err != nil {
		return SharedVPK{}, err
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return SharedVPK{}, err
	}
	item.Name, item.Path = newName, newPath
	updated, _ := json.Marshal(item)
	if err := os.WriteFile(newPath+".json", updated, 0640); err != nil {
		_ = os.Rename(newPath, oldPath)
		return SharedVPK{}, err
	}
	_ = os.Remove(oldPath + ".json")
	return item, nil
}
func (m *UploadManager) Delete(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !safeVPKName(name) {
		return errors.New("safe .vpk name required")
	}
	path := filepath.Join(m.shared, name)
	if err := os.Remove(path); err != nil {
		return err
	}
	return os.Remove(path + ".json")
}
func safeVPKName(name string) bool {
	return filepath.Base(name) == name && strings.ToLower(filepath.Ext(name)) == ".vpk"
}

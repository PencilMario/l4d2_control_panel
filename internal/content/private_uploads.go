package content

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/not0721here/l4d2-control-panel/internal/disk"
	"github.com/not0721here/l4d2-control-panel/internal/safepath"
)

const privateUploadLifetime = 24 * time.Hour

type PrivateUploadSession struct {
	ID         string    `json:"id"`
	InstanceID string    `json:"instance_id"`
	Path       string    `json:"path"`
	Hash       string    `json:"sha256"`
	Size       int64     `json:"size"`
	Offset     int64     `json:"offset"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type PrivateUploadManager struct {
	root     string
	maxBytes int64
	private  *PrivateManager
}

func NewPrivateUploadManager(root string, maxBytes int64) *PrivateUploadManager {
	return &PrivateUploadManager{root: root, maxBytes: maxBytes, private: NewPrivateManager(root, 1<<20)}
}

func (m *PrivateUploadManager) Begin(instanceID, name string, size int64, hash string) (PrivateUploadSession, error) {
	_ = m.Cleanup()
	if err := validateInstanceID(instanceID); err != nil {
		return PrivateUploadSession{}, err
	}
	private, err := m.private.privateRoot(instanceID)
	if err != nil {
		return PrivateUploadSession{}, err
	}
	if _, err = safepath.Join(private, name); err != nil {
		return PrivateUploadSession{}, fmt.Errorf("unsafe upload path: %w", err)
	}
	if size < 0 || size > m.maxBytes {
		return PrivateUploadSession{}, errors.New("upload exceeds declared maximum")
	}
	if len(hash) != sha256.Size*2 {
		return PrivateUploadSession{}, errors.New("invalid sha256")
	}
	if _, err = hex.DecodeString(hash); err != nil {
		return PrivateUploadSession{}, errors.New("invalid sha256")
	}
	lock := m.private.instanceLock(instanceID)
	lock.Lock()
	defer lock.Unlock()
	uploadRoot := filepath.Join(m.root, "instances", instanceID, "backups", "private", "uploads")
	if err = rejectSymlinkParents(m.root, uploadRoot); err != nil {
		return PrivateUploadSession{}, err
	}
	if err = os.MkdirAll(uploadRoot, 0750); err != nil {
		return PrivateUploadSession{}, err
	}
	available, err := disk.Available(uploadRoot)
	if err != nil {
		return PrivateUploadSession{}, err
	}
	if uint64(size) > available {
		return PrivateUploadSession{}, errors.New("insufficient disk space")
	}
	s := PrivateUploadSession{ID: uuid.NewString(), InstanceID: instanceID, Path: filepath.ToSlash(name), Hash: strings.ToLower(hash), Size: size, ExpiresAt: time.Now().UTC().Add(privateUploadLifetime)}
	part, meta := m.sessionPaths(uploadRoot, s.ID)
	f, err := os.OpenFile(part, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		return PrivateUploadSession{}, err
	}
	if err = f.Close(); err != nil {
		_ = os.Remove(part)
		return PrivateUploadSession{}, err
	}
	if err = writeUploadMetadata(meta, s); err != nil {
		_ = os.Remove(part)
		return PrivateUploadSession{}, err
	}
	return s, nil
}

func (m *PrivateUploadManager) Write(id string, offset int64, reader io.Reader) (int64, error) {
	s, part, meta, err := m.load(id)
	if err != nil {
		return 0, err
	}
	lock := m.private.instanceLock(s.InstanceID)
	lock.Lock()
	defer lock.Unlock()
	info, err := os.Stat(part)
	if err != nil {
		return 0, err
	}
	if offset != info.Size() || offset != s.Offset {
		return 0, errors.New("upload offset mismatch")
	}
	f, err := os.OpenFile(part, os.O_WRONLY|os.O_APPEND, 0640)
	if err != nil {
		return 0, err
	}
	limited := io.LimitReader(reader, s.Size-offset+1)
	n, copyErr := io.Copy(f, limited)
	if copyErr == nil {
		copyErr = f.Sync()
	}
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Truncate(part, offset)
		_ = syncFile(part)
		return 0, copyErr
	}
	if closeErr != nil {
		_ = os.Truncate(part, offset)
		_ = syncFile(part)
		return 0, closeErr
	}
	if offset+n > s.Size {
		_ = os.Truncate(part, offset)
		_ = syncFile(part)
		return 0, errors.New("upload exceeds declared size")
	}
	s.Offset += n
	if err = writeUploadMetadata(meta, s); err != nil {
		_ = os.Truncate(part, offset)
		return 0, err
	}
	return n, nil
}

func (m *PrivateUploadManager) Recover(id string) (PrivateUploadSession, error) {
	s, _, _, err := m.load(id)
	return s, err
}

func (m *PrivateUploadManager) Complete(id string) error {
	s, part, meta, err := m.load(id)
	if err != nil {
		return err
	}
	lock := m.private.instanceLock(s.InstanceID)
	lock.Lock()
	defer lock.Unlock()
	if s.Offset != s.Size {
		return errors.New("upload incomplete")
	}
	f, err := os.Open(part)
	if err != nil {
		return err
	}
	digest := sha256.New()
	_, copyErr := io.Copy(digest, f)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if hex.EncodeToString(digest.Sum(nil)) != s.Hash {
		return errors.New("upload hash mismatch")
	}
	private, err := m.private.privateRoot(s.InstanceID)
	if err != nil {
		return err
	}
	target, err := safepath.Join(private, s.Path)
	if err != nil {
		return err
	}
	if err = rejectSymlinkParents(m.root, target); err != nil {
		return err
	}
	if _, err = os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return errors.New("destination exists")
		}
		return err
	}
	if err = os.MkdirAll(filepath.Dir(target), 0750); err != nil {
		return err
	}
	partFile, err := os.OpenFile(part, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if err = partFile.Sync(); err == nil {
		err = partFile.Close()
	} else {
		_ = partFile.Close()
	}
	if err != nil {
		return err
	}
	if err = os.Link(part, target); err != nil {
		return fmt.Errorf("destination exists or cannot be published: %w", err)
	}
	rollback := func(cause error) error { _ = os.Remove(target); return cause }
	published, err := os.OpenFile(target, os.O_RDWR, 0)
	if err != nil {
		return rollback(err)
	}
	if err = published.Sync(); err == nil {
		err = published.Close()
	} else {
		_ = published.Close()
	}
	if err != nil {
		return rollback(err)
	}
	if err = syncDirectory(filepath.Dir(target)); err != nil {
		return rollback(err)
	}
	if err = os.Remove(meta); err != nil {
		return rollback(err)
	}
	if err = syncDirectory(filepath.Dir(meta)); err != nil {
		_ = writeUploadMetadata(meta, s)
		return rollback(err)
	}
	_ = os.Remove(part)
	_ = syncDirectory(filepath.Dir(part))
	return nil
}

func (m *PrivateUploadManager) Open(instanceID, name string) (*os.File, os.FileInfo, error) {
	private, err := m.private.privateRoot(instanceID)
	if err != nil {
		return nil, nil, err
	}
	target, err := safepath.Join(private, name)
	if err != nil {
		return nil, nil, err
	}
	if err = rejectSymlinkParents(m.root, target); err != nil {
		return nil, nil, err
	}
	f, err := os.Open(target)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		if err == nil {
			err = errors.New("not a regular file")
		}
		return nil, nil, err
	}
	return f, info, nil
}

func (m *PrivateUploadManager) load(id string) (PrivateUploadSession, string, string, error) {
	if _, err := uuid.Parse(id); err != nil {
		return PrivateUploadSession{}, "", "", errors.New("invalid upload id")
	}
	instances := filepath.Join(m.root, "instances")
	entries, err := os.ReadDir(instances)
	if err != nil {
		return PrivateUploadSession{}, "", "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() || validateInstanceID(entry.Name()) != nil {
			continue
		}
		uploadRoot := filepath.Join(instances, entry.Name(), "backups", "private", "uploads")
		part, meta := m.sessionPaths(uploadRoot, id)
		if rejectSymlinkParents(m.root, part) != nil || rejectSymlinkParents(m.root, meta) != nil {
			return PrivateUploadSession{}, "", "", errors.New("unsafe upload metadata")
		}
		raw, readErr := os.ReadFile(meta)
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return PrivateUploadSession{}, "", "", readErr
		}
		var s PrivateUploadSession
		if json.Unmarshal(raw, &s) != nil || s.ID != id || s.InstanceID != entry.Name() || validateInstanceID(s.InstanceID) != nil {
			return PrivateUploadSession{}, "", "", errors.New("invalid upload metadata")
		}
		private, joinErr := m.private.privateRoot(s.InstanceID)
		if joinErr != nil {
			return PrivateUploadSession{}, "", "", errors.New("invalid upload metadata")
		}
		if _, joinErr = safepath.Join(private, s.Path); joinErr != nil || s.Size < 0 || s.Size > m.maxBytes || len(s.Hash) != sha256.Size*2 || time.Now().After(s.ExpiresAt) {
			return PrivateUploadSession{}, "", "", errors.New("invalid or expired upload metadata")
		}
		info, statErr := os.Stat(part)
		if statErr != nil || info.Size() > s.Size {
			return PrivateUploadSession{}, "", "", errors.New("invalid upload data")
		}
		if info.Size() < s.Offset {
			return PrivateUploadSession{}, "", "", errors.New("upload data shorter than durable metadata")
		}
		if info.Size() > s.Offset {
			if err = os.Truncate(part, s.Offset); err != nil {
				return PrivateUploadSession{}, "", "", err
			}
		}
		return s, part, meta, nil
	}
	return PrivateUploadSession{}, "", "", os.ErrNotExist
}

func (m *PrivateUploadManager) sessionPaths(root, id string) (string, string) {
	return filepath.Join(root, id+".part"), filepath.Join(root, id+".json")
}

func writeUploadMetadata(path string, session PrivateUploadSession) error {
	raw, err := json.Marshal(session)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".upload-meta-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0640); err == nil {
		_, err = tmp.Write(raw)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	err = dir.Sync()
	if runtime.GOOS == "windows" && err != nil {
		return nil
	}
	return err
}

func syncFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func (m *PrivateUploadManager) Cleanup() error {
	instances := filepath.Join(m.root, "instances")
	entries, err := os.ReadDir(instances)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var result error
	for _, instance := range entries {
		if !instance.IsDir() || validateInstanceID(instance.Name()) != nil {
			continue
		}
		lock := m.private.instanceLock(instance.Name())
		lock.Lock()
		uploadRoot := filepath.Join(instances, instance.Name(), "backups", "private", "uploads")
		var files []os.DirEntry
		var readErr error
		if rejectSymlinkParents(m.root, uploadRoot) == nil {
			files, readErr = os.ReadDir(uploadRoot)
		} else {
			readErr = errors.New("unsafe upload root")
		}
		if readErr == nil {
			for _, file := range files {
				ext := filepath.Ext(file.Name())
				id := strings.TrimSuffix(file.Name(), ext)
				if (ext != ".json" && ext != ".part") || uuid.Validate(id) != nil {
					continue
				}
				part, meta := m.sessionPaths(uploadRoot, id)
				if rejectSymlinkParents(m.root, part) != nil || rejectSymlinkParents(m.root, meta) != nil {
					continue
				}
				raw, metaErr := os.ReadFile(meta)
				var s PrivateUploadSession
				if metaErr != nil || json.Unmarshal(raw, &s) != nil || s.ID != id || s.InstanceID != instance.Name() || time.Now().After(s.ExpiresAt) {
					if removeErr := os.Remove(part); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
						result = errors.Join(result, removeErr)
					}
					if removeErr := os.Remove(meta); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
						result = errors.Join(result, removeErr)
					}
				}
			}
		}
		lock.Unlock()
	}
	return result
}

func (m *PrivateUploadManager) RecoverAll() error { return m.Cleanup() }

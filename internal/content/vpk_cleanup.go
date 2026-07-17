package content

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/BenLubar/vpk"
)

type VPKCleanupResult struct {
	Name       string `json:"name"`
	Removed    int    `json:"removed"`
	BeforeSize int64  `json:"before_size"`
	AfterSize  int64  `json:"after_size"`
}

func (m *UploadManager) Clean(name string) (VPKCleanupResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !safeVPKName(name) {
		return VPKCleanupResult{}, errors.New("safe .vpk name required")
	}
	target := filepath.Join(m.shared, name)
	info, err := os.Stat(target)
	if err != nil {
		return VPKCleanupResult{}, err
	}
	pak, err := vpk.Open(vpk.SingleVPK(target))
	if err != nil {
		return VPKCleanupResult{}, err
	}
	kept := make([]vpk.Entry, 0, len(pak.Paths()))
	removed := 0
	for _, rel := range pak.Paths() {
		base, ext := path.Base(rel), strings.ToLower(path.Ext(rel))
		if ext == "" || ext == ".vtf" || ext == ".mp3" || ext == ".wav" || ext == ".vmf" || ext == ".vmx" || base == "" {
			removed++
			continue
		}
		kept = append(kept, pak.Entry(rel))
	}
	temporary, err := os.CreateTemp(m.shared, ".clean-*.vpk")
	if err != nil {
		return VPKCleanupResult{}, err
	}
	tempPath := temporary.Name()
	_ = temporary.Close()
	defer os.Remove(tempPath)
	if err := vpk.Create(vpk.SingleVPKCreator(tempPath), kept, -1); err != nil {
		return VPKCleanupResult{}, err
	}
	cleaned, err := os.Open(tempPath)
	if err != nil {
		return VPKCleanupResult{}, err
	}
	h := sha256.New()
	after, err := io.Copy(h, cleaned)
	closeErr := cleaned.Close()
	if err != nil {
		return VPKCleanupResult{}, err
	}
	if closeErr != nil {
		return VPKCleanupResult{}, closeErr
	}
	if _, err := vpk.Open(vpk.SingleVPK(tempPath)); err != nil {
		return VPKCleanupResult{}, err
	}
	if err := atomicReplaceFile(tempPath, target); err != nil {
		return VPKCleanupResult{}, err
	}
	item := SharedVPK{Name: name, Hash: hex.EncodeToString(h.Sum(nil)), Path: target, Size: after}
	raw, _ := json.Marshal(item)
	if err := os.WriteFile(target+".json.tmp", raw, 0640); err != nil {
		return VPKCleanupResult{}, err
	}
	if err := atomicReplaceFile(target+".json.tmp", target+".json"); err != nil {
		return VPKCleanupResult{}, err
	}
	return VPKCleanupResult{Name: name, Removed: removed, BeforeSize: info.Size(), AfterSize: after}, nil
}

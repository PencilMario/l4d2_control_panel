package accelerator

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const MaxArchiveBytes int64 = 128 << 20

var (
	ErrInvalidDownloadURL = errors.New("invalid Accelerator download URL")
	ErrArchiveTooLarge    = errors.New("Accelerator archive exceeds size limit")
)

type DownloadURLProvider func(context.Context) (string, error)
type GitHubProxyProvider func(context.Context) (string, error)

type Config struct {
	InstancesRoot       string
	CacheRoot           string
	DownloadURLProvider DownloadURLProvider
	GitHubProxyProvider GitHubProxyProvider
	Token               string
	PanelPort           int
	HTTPClient          *http.Client
	Now                 func() time.Time
	TargetPlatform      string
	TargetArchitecture  string
}

type Manager struct {
	instancesRoot       string
	cacheRoot           string
	downloadURLProvider DownloadURLProvider
	githubProxyProvider GitHubProxyProvider
	token               string
	panelPort           int
	httpClient          *http.Client
	now                 func() time.Time
	targetPlatform      string
	targetArchitecture  string
	cacheMu             sync.Mutex
}

type DownloadedArchive struct {
	Path   string
	SHA256 string
}

type ArchiveEntry struct {
	Path string
	Size uint64
}

type bootstrapFile struct {
	Path string
	Data []byte
}

//go:embed accelerator-2.6.games.txt
var accelerator26Gamedata []byte

var staticExtensionBootstrapFiles = []bootstrapFile{
	{Path: "addons/sourcemod/extensions/accelerator.autoload", Data: nil},
	{Path: "addons/sourcemod/gamedata/accelerator.games.txt", Data: accelerator26Gamedata},
}

type cacheIndex struct {
	URLs map[string]string `json:"urls"`
}

func New(config Config) (*Manager, error) {
	if strings.TrimSpace(config.InstancesRoot) == "" || strings.TrimSpace(config.CacheRoot) == "" {
		return nil, errors.New("Accelerator instances and cache roots are required")
	}
	if err := os.MkdirAll(config.CacheRoot, 0o750); err != nil {
		return nil, fmt.Errorf("create Accelerator cache: %w", err)
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Minute}
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	platform := strings.ToLower(strings.TrimSpace(config.TargetPlatform))
	if platform == "" {
		platform = "linux"
	}
	architecture := strings.ToLower(strings.TrimSpace(config.TargetArchitecture))
	if architecture == "" {
		// L4D2's Linux dedicated server and its SourceMod extension loader are x86.
		architecture = "x86"
	}
	return &Manager{
		instancesRoot: config.InstancesRoot, cacheRoot: config.CacheRoot,
		downloadURLProvider: config.DownloadURLProvider, githubProxyProvider: config.GitHubProxyProvider,
		token: config.Token, panelPort: config.PanelPort, httpClient: client, now: now,
		targetPlatform: platform, targetArchitecture: architecture,
	}, nil
}

func resolveDownloadURL(raw, proxy string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", ErrInvalidDownloadURL
	}
	proxy = strings.TrimSpace(proxy)
	if proxy == "" || !isGitHubHost(parsed.Hostname()) {
		return parsed.String(), nil
	}
	proxyURL, err := url.Parse(proxy)
	if err != nil || proxyURL.Scheme != "https" || proxyURL.Host == "" || proxyURL.User != nil || proxyURL.RawQuery != "" || proxyURL.Fragment != "" {
		return "", ErrInvalidDownloadURL
	}
	return strings.TrimRight(proxy, "/") + "/" + parsed.String(), nil
}

func isGitHubHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	return host == "github.com" || host == "www.github.com"
}

func (m *Manager) downloadArchive(ctx context.Context, rawURL, proxy string) (DownloadedArchive, error) {
	resolved, err := resolveDownloadURL(rawURL, proxy)
	if err != nil {
		return DownloadedArchive{}, err
	}
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	index, err := m.readCacheIndex()
	if err != nil {
		return DownloadedArchive{}, err
	}
	urlKey := hashString(resolved)
	if archiveHash := index.URLs[urlKey]; validHash(archiveHash) {
		cached := filepath.Join(m.cacheRoot, archiveHash+".zip")
		if info, statErr := os.Lstat(cached); statErr == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return DownloadedArchive{Path: cached, SHA256: archiveHash}, nil
		}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, resolved, nil)
	if err != nil {
		return DownloadedArchive{}, err
	}
	response, err := m.httpClient.Do(request)
	if err != nil {
		return DownloadedArchive{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return DownloadedArchive{}, fmt.Errorf("Accelerator archive download returned %s", response.Status)
	}
	if response.ContentLength > MaxArchiveBytes {
		return DownloadedArchive{}, ErrArchiveTooLarge
	}
	temporary, err := os.CreateTemp(m.cacheRoot, ".accelerator-archive-*")
	if err != nil {
		return DownloadedArchive{}, err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return DownloadedArchive{}, err
	}
	digest := sha256.New()
	written, err := io.Copy(io.MultiWriter(temporary, digest), io.LimitReader(response.Body, MaxArchiveBytes+1))
	if err != nil {
		return DownloadedArchive{}, err
	}
	if written > MaxArchiveBytes {
		return DownloadedArchive{}, ErrArchiveTooLarge
	}
	if err := temporary.Sync(); err != nil {
		return DownloadedArchive{}, err
	}
	if err := temporary.Close(); err != nil {
		return DownloadedArchive{}, err
	}
	archiveHash := hex.EncodeToString(digest.Sum(nil))
	cached := filepath.Join(m.cacheRoot, archiveHash+".zip")
	if info, statErr := os.Lstat(cached); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return DownloadedArchive{}, errors.New("Accelerator cache object is not a regular file")
		}
	} else if os.IsNotExist(statErr) {
		if err := os.Rename(temporaryPath, cached); err != nil {
			return DownloadedArchive{}, err
		}
	} else {
		return DownloadedArchive{}, statErr
	}
	index.URLs[urlKey] = archiveHash
	if err := m.writeCacheIndex(index); err != nil {
		return DownloadedArchive{}, err
	}
	return DownloadedArchive{Path: cached, SHA256: archiveHash}, nil
}

func validateArchive(archivePath, platform, architecture string) ([]ArchiveEntry, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("open Accelerator archive: %w", err)
	}
	defer reader.Close()
	platform = strings.ToLower(strings.TrimSpace(platform))
	architecture = strings.ToLower(strings.TrimSpace(architecture))
	if platform == "" || architecture == "" {
		return nil, errors.New("Accelerator archive target is required")
	}
	seen := make(map[string]struct{})
	entries := make([]ArchiveEntry, 0, len(reader.File))
	hasAutoload, hasGamedata, hasExtension := false, false, false
	for _, file := range reader.File {
		isDirectory := file.FileInfo().IsDir()
		if file.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("Accelerator archive contains symlink %q", file.Name)
		}
		normalized, err := normalizeArchiveEntryPath(file.Name, platform, isDirectory)
		if err != nil {
			return nil, err
		}
		if normalized == "" {
			if isDirectory {
				continue
			}
			return nil, fmt.Errorf("invalid Accelerator archive path %q", file.Name)
		}
		if _, exists := seen[normalized]; exists {
			return nil, fmt.Errorf("duplicate Accelerator archive entry %q", normalized)
		}
		seen[normalized] = struct{}{}
		if isDirectory {
			continue
		}
		if !allowedArchivePath(normalized) {
			return nil, fmt.Errorf("Accelerator archive path is not allowlisted: %q", normalized)
		}
		if normalized == "addons/sourcemod/extensions/accelerator.autoload" {
			hasAutoload = true
		}
		if normalized == "addons/sourcemod/gamedata/accelerator.games.txt" {
			hasGamedata = true
		}
		if isAcceleratorExtension(normalized) {
			if !extensionMatchesTarget(normalized, platform, architecture) {
				continue
			}
			hasExtension = true
		}
		entries = append(entries, ArchiveEntry{Path: normalized, Size: file.UncompressedSize64})
	}
	if !hasExtension || hasAutoload != hasGamedata {
		return nil, errors.New("Accelerator archive is missing autoload, extension, or gamedata")
	}
	if !hasAutoload {
		for _, file := range staticExtensionBootstrapFiles {
			entries = append(entries, ArchiveEntry{Path: file.Path, Size: uint64(len(file.Data))})
		}
	}
	return entries, nil
}

func normalizeArchiveEntryPath(value, platform string, isDirectory bool) (string, error) {
	value = strings.ReplaceAll(value, "\\", "/")
	if value == "" || strings.HasPrefix(value, "/") || (len(value) >= 2 && value[1] == ':') {
		return "", fmt.Errorf("invalid Accelerator archive path %q", value)
	}
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform != "" {
		prefix := platform + "/"
		if strings.HasPrefix(value, prefix) {
			value = strings.TrimPrefix(value, prefix)
		}
	}
	if isDirectory && strings.HasSuffix(value, "/") {
		value = strings.TrimSuffix(value, "/")
	}
	if value == "" {
		if isDirectory {
			return "", nil
		}
		return "", fmt.Errorf("invalid Accelerator archive path %q", value)
	}
	return normalizeArchivePath(value)
}

func normalizeArchivePath(value string) (string, error) {
	value = strings.ReplaceAll(value, "\\", "/")
	if value == "" || strings.HasPrefix(value, "/") || (len(value) >= 2 && value[1] == ':') {
		return "", fmt.Errorf("invalid Accelerator archive path %q", value)
	}
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if part == ".." || part == "" {
			return "", fmt.Errorf("invalid Accelerator archive path %q", value)
		}
	}
	clean := path.Clean(value)
	if clean != value || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("invalid Accelerator archive path %q", value)
	}
	return clean, nil
}

func allowedArchivePath(value string) bool {
	for _, prefix := range []string{
		"addons/sourcemod/extensions/",
		"addons/sourcemod/gamedata/",
		"addons/sourcemod/plugins/",
		"addons/sourcemod/scripting/",
	} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func isAcceleratorExtension(value string) bool {
	base := path.Base(value)
	return strings.HasPrefix(base, "accelerator.ext.")
}

func isAcceleratorManagedPath(value string) bool {
	return allowedArchivePath(value) && strings.HasPrefix(path.Base(value), "accelerator.")
}

func extensionMatchesTarget(value, platform, architecture string) bool {
	base := path.Base(value)
	platformSuffix := map[string]string{"linux": ".so", "windows": ".dll"}[platform]
	if platformSuffix == "" || !strings.HasSuffix(base, platformSuffix) {
		return false
	}
	architecture = strings.ToLower(architecture)
	if architecture == "x86_64" || architecture == "amd64" {
		return strings.HasPrefix(value, "addons/sourcemod/extensions/x64/")
	}
	return !strings.HasPrefix(value, "addons/sourcemod/extensions/x64/")
}

func (m *Manager) readCacheIndex() (cacheIndex, error) {
	index := cacheIndex{URLs: make(map[string]string)}
	path := filepath.Join(m.cacheRoot, "index.json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return index, nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return index, errors.New("invalid Accelerator cache index")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return index, err
	}
	if err := json.Unmarshal(raw, &index); err != nil {
		return index, err
	}
	if index.URLs == nil {
		index.URLs = make(map[string]string)
	}
	return index, nil
}

func (m *Manager) writeCacheIndex(index cacheIndex) error {
	raw, err := json.Marshal(index)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(m.cacheRoot, ".accelerator-cache-index-*")
	if err != nil {
		return err
	}
	path := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(path)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(path, filepath.Join(m.cacheRoot, "index.json")); err != nil {
		if _, statErr := os.Lstat(filepath.Join(m.cacheRoot, "index.json")); statErr != nil {
			return err
		}
		if err := os.Remove(filepath.Join(m.cacheRoot, "index.json")); err != nil {
			return err
		}
		return os.Rename(path, filepath.Join(m.cacheRoot, "index.json"))
	}
	return nil
}

func hashString(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func validHash(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}

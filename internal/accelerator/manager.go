package accelerator

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

const (
	maxArchiveUncompressedBytes int64 = 256 << 20
	maxArchiveFileBytes         int64 = 128 << 20
)

type stagedFile struct {
	Relative string
	Source   string
	Hash     string
	Size     int64
	Mode     os.FileMode
}

type fileBackup struct {
	Target string
	Backup string
}

func (m *Manager) Ensure(ctx context.Context, instance domain.Instance) error {
	return m.install(ctx, instance, false)
}

// Reinstall refreshes Accelerator after a Panel-controlled full deployment has
// replaced SourceMod files. It must only be called by deployment workflows;
// ordinary lifecycle operations use Ensure so external edits remain protected.
func (m *Manager) Reinstall(ctx context.Context, instance domain.Instance) error {
	return m.install(ctx, instance, true)
}

func (m *Manager) install(ctx context.Context, instance domain.Instance, replacePanelDeployment bool) error {
	if !instance.AcceleratorEnabled {
		return m.Remove(ctx, instance.ID)
	}
	if strings.TrimSpace(m.token) == "" {
		return errors.New("Accelerator token is not configured")
	}
	if m.panelPort < 1 || m.panelPort > 65535 {
		return errors.New("invalid Panel port")
	}
	if m.downloadURLProvider == nil {
		return errors.New("Accelerator download URL is not configured")
	}
	archiveURL, err := m.downloadURLProvider(ctx)
	if err != nil {
		return fmt.Errorf("resolve Accelerator download URL: %w", err)
	}
	proxy := ""
	if m.githubProxyProvider != nil {
		proxy, err = m.githubProxyProvider(ctx)
		if err != nil {
			return fmt.Errorf("resolve Accelerator GitHub proxy: %w", err)
		}
	}
	archive, err := m.downloadArchive(ctx, archiveURL, proxy)
	if err != nil {
		return err
	}
	return m.ensureArchive(ctx, instance, archive, archiveURL, proxy, replacePanelDeployment)
}

func (m *Manager) ensureArchive(ctx context.Context, instance domain.Instance, archive DownloadedArchive, sourceURL, proxy string, replacePanelDeployment bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	instanceRoot, gameRoot, err := m.instanceGameRoot(instance.ID)
	if err != nil {
		return err
	}
	manifest, err := m.loadManifest(instanceRoot)
	if err != nil {
		return err
	}
	entries, err := validateArchive(archive.Path, m.targetPlatform, m.targetArchitecture)
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp(instanceRoot, ".accelerator-stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	staged, err := extractArchive(archive.Path, stage, entries, m.targetPlatform)
	if err != nil {
		return err
	}
	corePath := filepath.Join(gameRoot, "addons", "sourcemod", "configs", "core.cfg")
	if err := ensureDirectoryChain(gameRoot, filepath.Dir(corePath)); err != nil {
		return fmt.Errorf("prepare SourceMod core.cfg path: %w", err)
	}
	if err := ensureRegularPath(corePath); err != nil {
		return fmt.Errorf("read SourceMod core.cfg: %w", err)
	}
	coreOriginal, err := os.ReadFile(corePath)
	if err != nil {
		return err
	}
	if !replacePanelDeployment && manifest != nil && manifest.CoreConfigSHA256 != hashBytes(coreOriginal) {
		return &ConflictError{Paths: []string{manifest.CoreConfigPath}}
	}
	patchedCore, currentChanges, err := patchCoreConfig(coreOriginal, m.panelPort, m.token)
	if err != nil {
		return err
	}
	coreChanges := currentChanges
	if manifest != nil && !replacePanelDeployment {
		coreChanges = mergeCoreConfigChanges(manifest.CoreConfigChanges, currentChanges, patchedCore)
	}
	preservedFiles, err := validateExistingManagedFiles(gameRoot, manifest, staged, replacePanelDeployment)
	if err != nil {
		return err
	}
	files := make(map[string]ManagedFile, len(staged))
	for _, file := range staged {
		files[file.Relative] = ManagedFile{SHA256: file.Hash, Size: file.Size}
	}
	if manifest != nil && !replacePanelDeployment {
		for relative := range manifest.PreservedFiles {
			if _, stillPresent := files[relative]; stillPresent {
				preservedFiles[relative] = true
			}
		}
	}
	newManifest := Manifest{
		Version:           manifestVersion,
		Enabled:           true,
		ArchiveSHA256:     archive.SHA256,
		SourceURL:         strings.TrimSpace(sourceURL),
		ResolvedURL:       resolvedSourceURL(sourceURL, proxy),
		InstalledAt:       m.now(),
		UpdatedAt:         m.now(),
		Files:             files,
		PreservedFiles:    preservedFiles,
		CoreConfigPath:    manifestCoreConfigPath,
		CoreConfigSHA256:  hashBytes(patchedCore),
		CoreConfigChanges: coreChanges,
	}
	if manifest != nil && !manifest.InstalledAt.IsZero() {
		newManifest.InstalledAt = manifest.InstalledAt
	}
	if err := m.applyInstallation(ctx, instanceRoot, gameRoot, corePath, coreOriginal, patchedCore, manifest, stage, newManifest); err != nil {
		return err
	}
	return nil
}

func (m *Manager) Remove(ctx context.Context, instanceID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	instanceRoot, gameRoot, err := m.instanceGameRoot(instanceID)
	if err != nil {
		return err
	}
	manifest, err := m.loadManifest(instanceRoot)
	if err != nil {
		return err
	}
	if manifest == nil {
		return nil
	}
	corePath := filepath.Join(gameRoot, "addons", "sourcemod", "configs", "core.cfg")
	conflicts := make([]string, 0)
	for relative, expected := range manifest.Files {
		if manifest.PreservedFiles[relative] {
			continue
		}
		target, pathErr := safeGamePath(gameRoot, relative)
		if pathErr != nil {
			return pathErr
		}
		info, statErr := os.Lstat(target)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			conflicts = append(conflicts, relative)
			continue
		}
		if digest, hashErr := hashFile(target); hashErr != nil || digest != expected.SHA256 {
			conflicts = append(conflicts, relative)
		}
	}
	coreCurrent, err := os.ReadFile(corePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		conflicts = append(conflicts, manifest.CoreConfigPath)
	} else if hashBytes(coreCurrent) != manifest.CoreConfigSHA256 {
		conflicts = append(conflicts, manifest.CoreConfigPath)
	}
	if len(conflicts) > 0 {
		return &ConflictError{Paths: conflicts}
	}
	if err := restoreAndWriteCore(corePath, coreCurrent, manifest.CoreConfigChanges); err != nil {
		return err
	}
	for relative := range manifest.Files {
		if manifest.PreservedFiles[relative] {
			continue
		}
		target, pathErr := safeGamePath(gameRoot, relative)
		if pathErr != nil {
			return pathErr
		}
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return os.Remove(manifestPath(instanceRoot))
}

func extractArchive(archivePath, stage string, entries []ArchiveEntry, platform string) ([]stagedFile, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	wanted := make(map[string]ArchiveEntry, len(entries))
	for _, entry := range entries {
		wanted[entry.Path] = entry
	}
	staged := make([]stagedFile, 0)
	extracted := make(map[string]struct{}, len(entries))
	var total int64
	for _, file := range reader.File {
		isDirectory := file.FileInfo().IsDir()
		normalized, err := normalizeArchiveEntryPath(file.Name, platform, isDirectory)
		if err != nil {
			return nil, err
		}
		if normalized == "" {
			if isDirectory {
				continue
			}
			return nil, errors.New("archive validation entry mismatch")
		}
		if isDirectory {
			continue
		}
		if _, ok := wanted[normalized]; !ok {
			if isAcceleratorExtension(normalized) {
				continue
			}
			return nil, errors.New("archive validation entry mismatch")
		}
		if file.UncompressedSize64 > uint64(maxArchiveFileBytes) || total+int64(file.UncompressedSize64) > maxArchiveUncompressedBytes {
			return nil, ErrArchiveTooLarge
		}
		target := filepath.Join(stage, filepath.FromSlash(normalized))
		if err := makeSafeDirectory(filepath.Dir(target)); err != nil {
			return nil, err
		}
		output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return nil, err
		}
		input, openErr := file.Open()
		if openErr != nil {
			output.Close()
			return nil, openErr
		}
		digest := sha256.New()
		written, copyErr := io.Copy(io.MultiWriter(output, digest), io.LimitReader(input, maxArchiveFileBytes+1))
		closeInputErr := input.Close()
		closeOutputErr := output.Close()
		if copyErr != nil || closeInputErr != nil || closeOutputErr != nil {
			return nil, errors.Join(copyErr, closeInputErr, closeOutputErr)
		}
		if written != int64(file.UncompressedSize64) || written > maxArchiveFileBytes {
			return nil, ErrArchiveTooLarge
		}
		mode := file.Mode().Perm() & 0o755
		if mode == 0 {
			mode = 0o644
		}
		if err := os.Chmod(target, mode); err != nil {
			return nil, err
		}
		total += written
		staged = append(staged, stagedFile{Relative: normalized, Source: target, Hash: hex.EncodeToString(digest.Sum(nil)), Size: written, Mode: mode})
		extracted[normalized] = struct{}{}
	}
	for _, file := range staticExtensionBootstrapFiles {
		if _, wanted := wanted[file.Path]; !wanted {
			continue
		}
		if _, exists := extracted[file.Path]; exists {
			continue
		}
		if int64(len(file.Data)) > maxArchiveFileBytes || total+int64(len(file.Data)) > maxArchiveUncompressedBytes {
			return nil, ErrArchiveTooLarge
		}
		target := filepath.Join(stage, filepath.FromSlash(file.Path))
		if err := makeSafeDirectory(filepath.Dir(target)); err != nil {
			return nil, err
		}
		output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return nil, err
		}
		written, writeErr := output.Write(file.Data)
		closeErr := output.Close()
		if writeErr != nil || closeErr != nil {
			return nil, errors.Join(writeErr, closeErr)
		}
		if written != len(file.Data) {
			return nil, io.ErrShortWrite
		}
		if err := os.Chmod(target, 0o644); err != nil {
			return nil, err
		}
		digest := sha256.Sum256(file.Data)
		total += int64(written)
		staged = append(staged, stagedFile{Relative: file.Path, Source: target, Hash: hex.EncodeToString(digest[:]), Size: int64(written), Mode: 0o644})
		extracted[file.Path] = struct{}{}
	}
	if len(extracted) != len(wanted) {
		return nil, errors.New("archive validation entry mismatch")
	}
	return staged, nil
}

func (m *Manager) applyInstallation(ctx context.Context, instanceRoot, gameRoot, corePath string, originalCore, patchedCore []byte, previous *Manifest, stage string, manifest Manifest) error {
	backup, err := os.MkdirTemp(instanceRoot, ".accelerator-backup-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(backup)
	backups := make([]fileBackup, 0)
	rollback := func() {
		_ = restoreFileBackups(backups)
		_ = writeAtomicFile(corePath, originalCore, 0o640)
	}
	if previous != nil {
		for relative := range previous.Files {
			if _, stillManaged := manifest.Files[relative]; stillManaged {
				continue
			}
			if previous.PreservedFiles[relative] {
				continue
			}
			target, pathErr := safeGamePath(gameRoot, relative)
			if pathErr != nil {
				return pathErr
			}
			if _, statErr := os.Lstat(target); statErr == nil {
				backupPath := filepath.Join(backup, filepath.FromSlash(relative))
				if err := makeSafeDirectory(filepath.Dir(backupPath)); err != nil {
					return err
				}
				if err := backupFile(target, backupPath, os.Rename); err != nil {
					return err
				}
				backups = append(backups, fileBackup{Target: target, Backup: backupPath})
			}
		}
	}
	stagedFiles, err := stagedFilesFromDirectory(stage, manifest.Files)
	if err != nil {
		return err
	}
	for _, file := range stagedFiles {
		if err := ctx.Err(); err != nil {
			rollback()
			return err
		}
		target, pathErr := safeGamePath(gameRoot, file.Relative)
		if pathErr != nil {
			rollback()
			return pathErr
		}
		if info, statErr := os.Lstat(target); statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				rollback()
				return &ConflictError{Paths: []string{file.Relative}}
			}
			backupPath := filepath.Join(backup, filepath.FromSlash(file.Relative))
			if err := makeSafeDirectory(filepath.Dir(backupPath)); err != nil {
				rollback()
				return err
			}
			if err := backupFile(target, backupPath, os.Rename); err != nil {
				rollback()
				return err
			}
			backups = append(backups, fileBackup{Target: target, Backup: backupPath})
		} else if !os.IsNotExist(statErr) {
			rollback()
			return statErr
		}
		if err := copyFile(file.Source, target, file.Mode); err != nil {
			rollback()
			return err
		}
	}
	if err := writeAtomicFile(corePath, patchedCore, 0o640); err != nil {
		rollback()
		return err
	}
	if err := writeManifestAtomic(instanceRoot, manifest); err != nil {
		rollback()
		return err
	}
	return nil
}

func stagedFilesFromDirectory(stage string, manifestFiles map[string]ManagedFile) ([]stagedFile, error) {
	files := make([]stagedFile, 0, len(manifestFiles))
	for relative, expected := range manifestFiles {
		source := filepath.Join(stage, filepath.FromSlash(relative))
		info, err := os.Lstat(source)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, errors.New("staged Accelerator file is unavailable")
		}
		hash, err := hashFile(source)
		if err != nil || hash != expected.SHA256 || info.Size() != expected.Size {
			return nil, errors.New("staged Accelerator file hash mismatch")
		}
		files = append(files, stagedFile{Relative: relative, Source: source, Hash: hash, Size: info.Size(), Mode: info.Mode().Perm()})
	}
	return files, nil
}

func validateExistingManagedFiles(gameRoot string, previous *Manifest, staged []stagedFile, replacePanelDeployment bool) (map[string]bool, error) {
	allowed := make(map[string]stagedFile, len(staged))
	preserved := make(map[string]bool)
	for _, file := range staged {
		allowed[file.Relative] = file
	}
	conflicts := make([]string, 0)
	for _, file := range staged {
		target, err := safeGamePath(gameRoot, file.Relative)
		if err != nil {
			return nil, err
		}
		info, statErr := os.Lstat(target)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			conflicts = append(conflicts, file.Relative)
			continue
		}
		owned, ownedByPanel := ManagedFile{}, false
		if previous != nil {
			owned, ownedByPanel = previous.Files[file.Relative]
		}
		if !ownedByPanel {
			hash, hashErr := hashFile(target)
			// Plugin packages may carry an older Accelerator payload; hand off only
			// the explicitly named Accelerator paths and preserve them on removal.
			if hashErr != nil || (hash != file.Hash && !isAcceleratorManagedPath(file.Relative)) {
				conflicts = append(conflicts, file.Relative)
				continue
			}
			if replacePanelDeployment {
				continue
			}
			preserved[file.Relative] = true
			continue
		}
		if previous != nil && previous.PreservedFiles[file.Relative] {
			if replacePanelDeployment {
				continue
			}
			preserved[file.Relative] = true
			continue
		}
		hash, hashErr := hashFile(target)
		if hashErr != nil || (!replacePanelDeployment && hash != owned.SHA256) {
			conflicts = append(conflicts, file.Relative)
		}
	}
	if previous != nil {
		for relative := range previous.Files {
			if _, stillManaged := allowed[relative]; stillManaged {
				continue
			}
			target, err := safeGamePath(gameRoot, relative)
			if err != nil {
				return nil, err
			}
			if previous.PreservedFiles[relative] && !replacePanelDeployment {
				continue
			}
			info, statErr := os.Lstat(target)
			if os.IsNotExist(statErr) {
				continue
			}
			if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				conflicts = append(conflicts, relative)
				continue
			}
			hash, hashErr := hashFile(target)
			if hashErr != nil || (!replacePanelDeployment && hash != previous.Files[relative].SHA256) {
				conflicts = append(conflicts, relative)
			}
		}
	}
	if len(conflicts) > 0 {
		return nil, &ConflictError{Paths: conflicts}
	}
	return preserved, nil
}

func mergeCoreConfigChanges(previous, current map[string]CoreConfigChange, patched []byte) map[string]CoreConfigChange {
	merged := make(map[string]CoreConfigChange, len(current))
	for key, change := range current {
		if old, ok := previous[key]; ok {
			old.Written = change.Written
			if !old.Present {
				prefix := ""
				if strings.HasPrefix(old.InsertedText, "\n") {
					prefix = "\n"
				}
				old.InsertedText = prefix + insertedCoreLine(patched, key, change.Written)
			}
			merged[key] = old
			continue
		}
		merged[key] = change
	}
	return merged
}

func insertedCoreLine(raw []byte, key, value string) string {
	needle := quoteCoreValue(key) + " " + quoteCoreValue(value)
	position := bytes.Index(raw, []byte(needle))
	if position < 0 {
		return ""
	}
	start := bytes.LastIndexByte(raw[:position], '\n') + 1
	end := bytes.IndexByte(raw[position:], '\n')
	if end < 0 {
		return string(raw[start:])
	}
	return string(raw[start : position+end+1])
}

func restoreAndWriteCore(path string, current []byte, changes map[string]CoreConfigChange) error {
	restored, err := restoreCoreConfig(current, changes)
	if err != nil {
		return err
	}
	return writeAtomicFile(path, restored, 0o640)
}

func (m *Manager) instanceGameRoot(instanceID string) (string, string, error) {
	if instanceID == "" || filepath.Base(instanceID) != instanceID || strings.ContainsAny(instanceID, `/\\`) || instanceID == "." || instanceID == ".." {
		return "", "", errors.New("invalid instance ID")
	}
	instancesRoot, err := filepath.Abs(m.instancesRoot)
	if err != nil {
		return "", "", err
	}
	instanceRoot := filepath.Join(instancesRoot, instanceID)
	info, err := os.Lstat(instanceRoot)
	if err != nil {
		return "", "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", "", errors.New("instance root is not a regular directory")
	}
	instanceRootResolved, err := filepath.EvalSymlinks(instanceRoot)
	if err != nil {
		return "", "", err
	}
	game := filepath.Join(instanceRoot, "game")
	gameResolved, err := filepath.EvalSymlinks(game)
	if err != nil {
		return "", "", err
	}
	gameResolved, err = filepath.Abs(gameResolved)
	if err != nil {
		return "", "", err
	}
	if !pathWithin(instanceRootResolved, gameResolved) {
		return "", "", errors.New("instance game path escapes instance root")
	}
	gameRoot := filepath.Join(gameResolved, "left4dead2")
	gameInfo, err := os.Stat(gameRoot)
	if err != nil {
		return "", "", err
	}
	if !gameInfo.IsDir() {
		return "", "", errors.New("instance game directory is not a directory")
	}
	return instanceRoot, gameRoot, nil
}

func safeGamePath(gameRoot, relative string) (string, error) {
	normalized, err := normalizeArchivePath(relative)
	if err != nil || normalized != relative {
		return "", errors.New("invalid Accelerator managed path")
	}
	target := filepath.Join(gameRoot, filepath.FromSlash(relative))
	if !pathWithin(gameRoot, target) {
		return "", errors.New("Accelerator managed path escapes game root")
	}
	parent := filepath.Dir(target)
	if err := ensureDirectoryChain(gameRoot, parent); err != nil {
		return "", err
	}
	return target, nil
}

func ensureRegularPath(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("path is not a regular file")
	}
	return nil
}

func ensureDirectoryPath(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("path is not a regular directory")
	}
	return nil
}

func ensureDirectoryChain(root, target string) error {
	if !pathWithin(root, target) {
		return errors.New("directory path escapes game root")
	}
	if err := ensureDirectoryPath(root); err != nil {
		return err
	}
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	if relative == "." {
		return nil
	}
	current := root
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		if err := ensureDirectoryPath(current); err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
	}
	return nil
}

func makeSafeDirectory(path string) error {
	if err := os.MkdirAll(path, 0o750); err != nil {
		return err
	}
	current := path
	for {
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("directory path contains unsafe entry")
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return nil
}

func pathWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func copyFile(source, target string, mode os.FileMode) error {
	if err := makeSafeDirectory(filepath.Dir(target)); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	temporary, err := os.CreateTemp(filepath.Dir(target), ".accelerator-file-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(mode.Perm()); err != nil {
		return err
	}
	if _, err := io.Copy(temporary, input); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return replacePath(temporaryPath, target)
}

func backupFile(target, backup string, rename func(string, string) error) error {
	if err := rename(target, backup); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	info, err := os.Lstat(target)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("cannot back up non-regular file")
	}
	if err := copyFile(target, backup, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Remove(target)
}

func writeAtomicFile(path string, content []byte, mode os.FileMode) error {
	if err := makeSafeDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".accelerator-write-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(mode.Perm()); err != nil {
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return replacePath(temporaryPath, path)
}

func replacePath(source, target string) error {
	if err := os.Rename(source, target); err == nil {
		return nil
	}
	if err := os.Remove(target); err != nil {
		return err
	}
	return os.Rename(source, target)
}

func writeManifestAtomic(instanceRoot string, manifest Manifest) error {
	raw, err := json.MarshalIndent(manifest, "", "\t")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return writeAtomicFile(manifestPath(instanceRoot), raw, 0o600)
}

func restoreFileBackups(backups []fileBackup) error {
	var joined error
	for index := len(backups) - 1; index >= 0; index-- {
		backup := backups[index]
		_ = os.Remove(backup.Target)
		if err := makeSafeDirectory(filepath.Dir(backup.Target)); err != nil {
			joined = errors.Join(joined, err)
			continue
		}
		info, err := os.Lstat(backup.Backup)
		if err != nil {
			joined = errors.Join(joined, err)
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			joined = errors.Join(joined, errors.New("invalid Accelerator backup file"))
			continue
		}
		if err := copyFile(backup.Backup, backup.Target, info.Mode().Perm()); err != nil {
			joined = errors.Join(joined, err)
			continue
		}
		if err := os.Remove(backup.Backup); err != nil {
			joined = errors.Join(joined, err)
		}
	}
	return joined
}

func hashBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func resolvedSourceURL(source, proxy string) string {
	resolved, err := resolveDownloadURL(source, proxy)
	if err != nil {
		return strings.TrimSpace(source)
	}
	return resolved
}

package updates

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/google/uuid"
	archivecheck "github.com/not0721here/l4d2-control-panel/internal/archive"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/safepath"
)

type Mode string

const (
	Hot  Mode = "hot"
	Full Mode = "full"
)

type Pipeline struct {
	root        string
	AfterDeploy func() error
}

type manifest struct {
	Version string            `json:"version"`
	Files   map[string]string `json:"files"`
}

type journalEntry struct {
	Path    string `json:"path"`
	Existed bool   `json:"existed"`
	Kind    string `json:"kind,omitempty"`
}

type updateJournal struct {
	Version                int            `json:"version"`
	InstanceID             string         `json:"instance_id"`
	Mode                   Mode           `json:"mode"`
	Stage                  string         `json:"stage"`
	BackupRoot             string         `json:"backup_root"`
	Affected               []journalEntry `json:"affected"`
	ManifestExisted        bool           `json:"manifest_existed"`
	PrivateManifestExisted bool           `json:"private_manifest_existed"`
	PrivateLowerExisted    bool           `json:"private_lower_existed"`
	PrivateSnapshots       []string       `json:"private_snapshots,omitempty"`
}

type Deployment interface {
	Commit() error
	Rollback() error
}

type deployment struct {
	pipeline    *Pipeline
	journalPath string
	journal     updateJournal
	private     *content.PrivateTransaction
}

func New(root string) *Pipeline { return &Pipeline{root: root} }

func (p *Pipeline) Apply(ctx context.Context, instanceID, archivePath, version string, mode Mode) error {
	transaction, err := p.Begin(ctx, instanceID, archivePath, version, mode)
	if err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return errors.Join(err, transaction.Rollback())
	}
	return nil
}

func (p *Pipeline) Begin(ctx context.Context, instanceID, archivePath, version string, mode Mode) (Deployment, error) {
	inspected, err := archivecheck.InspectZip(archivePath, archivecheck.Limits{MaxFiles: 20000, MaxBytes: 8 << 30})
	if err != nil {
		return nil, err
	}
	if mode == Hot && !inspected.HotCompatible {
		return nil, errors.New("package is not hot-update compatible")
	}
	if mode != Hot && mode != Full {
		return nil, errors.New("unsupported update mode")
	}
	if instanceID == "" || filepath.Base(instanceID) != instanceID {
		return nil, errors.New("invalid instance id")
	}

	base := filepath.Join(p.root, "instances", instanceID)
	game := filepath.Join(base, "game", "left4dead2")
	work := filepath.Join(base, "backups", "update-"+uuid.NewString())
	staging := filepath.Join(work, "staging")
	backup := filepath.Join(work, "replaced")
	manifestPath := filepath.Join(base, "package-manifest.json")
	manifestBackup := filepath.Join(work, "manifest.before")
	privateManifestPath := filepath.Join(base, "private-applied.json")
	privateLowerPath := filepath.Join(base, "backups", "private", "lower")
	if err := os.MkdirAll(staging, 0750); err != nil {
		return nil, err
	}
	keepWork := false
	defer func() {
		if !keepWork {
			_ = os.RemoveAll(work)
		}
	}()

	newManifest := manifest{Version: version, Files: map[string]string{}}
	if err := extract(archivePath, staging, newManifest.Files, mode); err != nil {
		return nil, err
	}
	old := readManifest(manifestPath)
	affected := map[string]bool{}
	for path := range newManifest.Files {
		affected[path] = true
	}
	if mode == Full {
		for path := range old.Files {
			affected[path] = true
		}
	}
	privateManager := content.NewPrivateManager(p.root, 1<<20)
	privateTransaction, err := privateManager.BeginTransaction(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	leaseOwned := true
	defer func() {
		if leaseOwned {
			_ = privateTransaction.Rollback()
		}
	}()
	privateTargets, err := privateTransaction.Targets(ctx)
	if err != nil {
		return nil, err
	}
	for _, target := range privateTargets {
		affected[target.Path] = true
	}
	if err := collectFiles(filepath.Join(base, "private"), affected); err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(affected))
	for path := range affected {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	entries := make([]journalEntry, 0, len(paths))
	for _, path := range paths {
		target, err := safepath.Join(game, path)
		if err != nil {
			return nil, err
		}
		entry := journalEntry{Path: path}
		if err := rejectSymlinkPath(game, target); err != nil {
			return nil, err
		}
		info, statErr := os.Lstat(target)
		if statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return nil, errors.New("symbolic links are forbidden")
			}
			entry.Existed = true
			if info.IsDir() {
				entry.Kind = "directory"
			} else if info.Mode().IsRegular() {
				entry.Kind = "file"
			} else {
				return nil, errors.New("unsupported game target type")
			}
			destination, err := safepath.Join(backup, path)
			if err != nil {
				return nil, err
			}
			if entry.Kind == "directory" {
				if err := copyDirectory(target, destination); err != nil {
					return nil, err
				}
			} else if err := copyFile(target, destination); err != nil {
				return nil, err
			}
		} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return nil, statErr
		}
		entries = append(entries, entry)
	}
	manifestExisted := false
	if info, statErr := os.Stat(manifestPath); statErr == nil && !info.IsDir() {
		manifestExisted = true
		if err := copyFile(manifestPath, manifestBackup); err != nil {
			return nil, err
		}
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, statErr
	}
	privateManifestExisted := false
	if info, statErr := os.Stat(privateManifestPath); statErr == nil && !info.IsDir() {
		privateManifestExisted = true
		if err := copyFile(privateManifestPath, filepath.Join(work, "private-manifest.before")); err != nil {
			return nil, err
		}
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, statErr
	}
	privateLowerExisted := false
	if info, statErr := os.Stat(privateLowerPath); statErr == nil && info.IsDir() {
		privateLowerExisted = true
		if err := copyDirectory(privateLowerPath, filepath.Join(work, "private-lower.before")); err != nil {
			return nil, err
		}
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, statErr
	}
	privateSnapshots, err := directoryNames(filepath.Join(base, "backups", "private", "snapshots"))
	if err != nil {
		return nil, err
	}

	value := updateJournal{
		Version:                1,
		InstanceID:             instanceID,
		Mode:                   mode,
		Stage:                  "prepared",
		BackupRoot:             "replaced",
		Affected:               entries,
		ManifestExisted:        manifestExisted,
		PrivateManifestExisted: privateManifestExisted,
		PrivateLowerExisted:    privateLowerExisted,
		PrivateSnapshots:       privateSnapshots,
	}
	journalPath := filepath.Join(work, "journal.json")
	if err := writeJournal(journalPath, value); err != nil {
		return nil, err
	}
	keepWork = true
	transaction := &deployment{pipeline: p, journalPath: journalPath, journal: value, private: privateTransaction}
	fail := func(cause error) (Deployment, error) {
		return nil, errors.Join(cause, transaction.Rollback())
	}

	transaction.journal.Stage = "applying"
	if err := writeJournal(journalPath, transaction.journal); err != nil {
		return fail(err)
	}
	if mode == Full {
		for oldPath := range old.Files {
			if _, keep := newManifest.Files[oldPath]; !keep {
				target, err := safepath.Join(game, oldPath)
				if err != nil {
					return fail(err)
				}
				if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
					return fail(err)
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if err := content.ApplyTree(staging, game); err != nil {
		return fail(err)
	}
	if p.AfterDeploy != nil {
		if err := p.AfterDeploy(); err != nil {
			return fail(err)
		}
	}
	if err := privateTransaction.RebaseAndApply(ctx); err != nil {
		return fail(err)
	}
	if err := writeManifest(manifestPath, newManifest); err != nil {
		return fail(err)
	}
	transaction.journal.Stage = "deployed"
	if err := writeJournal(journalPath, transaction.journal); err != nil {
		return fail(err)
	}
	leaseOwned = false
	return transaction, nil
}

func (d *deployment) Commit() error {
	d.journal.Stage = "committed"
	if err := writeJournal(d.journalPath, d.journal); err != nil {
		return err
	}
	_ = os.RemoveAll(filepath.Dir(d.journalPath))
	if d.private != nil {
		return d.private.Commit()
	}
	return nil
}

func (d *deployment) Rollback() error {
	d.journal.Stage = "rolling_back"
	if err := validateUpdateJournalPathAndValue(d.pipeline.root, d.journalPath, d.journal); err != nil {
		return err
	}
	stageErr := writeJournal(d.journalPath, d.journal)
	rollbackErr := d.pipeline.rollbackJournal(d.journalPath, d.journal)
	if rollbackErr != nil {
		if d.private != nil {
			_ = d.private.Rollback()
		}
		return errors.Join(stageErr, rollbackErr)
	}
	cleanupErr := os.RemoveAll(filepath.Dir(d.journalPath))
	var privateErr error
	if d.private != nil {
		privateErr = d.private.Rollback()
	}
	return errors.Join(stageErr, cleanupErr, privateErr)
}

func (p *Pipeline) Recover(ctx context.Context) error {
	if err := content.NewPrivateManager(p.root, 1<<20).Recover(ctx); err != nil {
		return err
	}
	pattern := filepath.Join(p.root, "instances", "*", "backups", "update-*", "journal.json")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	var result error
	for _, journalPath := range paths {
		if err := ctx.Err(); err != nil {
			return errors.Join(result, err)
		}
		value, err := readJournal(journalPath)
		if err != nil {
			result = errors.Join(result, fmt.Errorf("read update journal %s: %w", journalPath, err))
			continue
		}
		if err := validateUpdateJournalPathAndValue(p.root, journalPath, value); err != nil {
			result = errors.Join(result, fmt.Errorf("validate update journal %s: %w", journalPath, err))
			continue
		}
		if value.Stage == "committed" {
			result = errors.Join(result, os.RemoveAll(filepath.Dir(journalPath)))
			continue
		}
		transaction := &deployment{pipeline: p, journalPath: journalPath, journal: value}
		if err := transaction.Rollback(); err != nil {
			result = errors.Join(result, fmt.Errorf("rollback update journal %s: %w", journalPath, err))
		}
	}
	return result
}

func (p *Pipeline) rollbackJournal(journalPath string, value updateJournal) error {
	if err := validateUpdateJournalPathAndValue(p.root, journalPath, value); err != nil {
		return err
	}
	work := filepath.Dir(journalPath)
	expectedInstanceID := filepath.Base(filepath.Dir(filepath.Dir(work)))
	if value.Version != 1 || value.InstanceID == "" || value.InstanceID != expectedInstanceID || filepath.Base(value.InstanceID) != value.InstanceID {
		return errors.New("invalid update journal identity")
	}
	if value.BackupRoot != "replaced" {
		return errors.New("invalid update journal backup root")
	}
	backup, err := safepath.Join(work, value.BackupRoot)
	if err != nil {
		return err
	}
	base := filepath.Join(p.root, "instances", value.InstanceID)
	game := filepath.Join(base, "game", "left4dead2")
	var result error
	entries := append([]journalEntry(nil), value.Affected...)
	sort.Slice(entries, func(i, j int) bool { return strings.Count(entries[i].Path, "/") > strings.Count(entries[j].Path, "/") })
	for _, entry := range entries {
		target, err := safepath.Join(game, entry.Path)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		if err = os.RemoveAll(target); err != nil {
			result = errors.Join(result, err)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return strings.Count(entries[i].Path, "/") < strings.Count(entries[j].Path, "/") })
	for _, entry := range entries {
		target, err := safepath.Join(game, entry.Path)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		if entry.Existed {
			source, sourceErr := safepath.Join(backup, entry.Path)
			if sourceErr == nil && entry.Kind == "directory" {
				sourceErr = copyDirectory(source, target)
			} else if sourceErr == nil {
				sourceErr = copyFile(source, target)
			}
			result = errors.Join(result, sourceErr)
		} else if removeErr := os.Remove(target); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			result = errors.Join(result, removeErr)
		}
	}
	manifestPath := filepath.Join(base, "package-manifest.json")
	if value.ManifestExisted {
		result = errors.Join(result, copyFile(filepath.Join(work, "manifest.before"), manifestPath))
	} else if err := os.Remove(manifestPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		result = errors.Join(result, err)
	}
	privateManifestPath := filepath.Join(base, "private-applied.json")
	if value.PrivateManifestExisted {
		result = errors.Join(result, copyFile(filepath.Join(work, "private-manifest.before"), privateManifestPath))
	} else if err := os.Remove(privateManifestPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		result = errors.Join(result, err)
	}
	privateLowerPath := filepath.Join(base, "backups", "private", "lower")
	if err := os.RemoveAll(privateLowerPath); err != nil {
		result = errors.Join(result, err)
	} else if value.PrivateLowerExisted {
		result = errors.Join(result, copyDirectory(filepath.Join(work, "private-lower.before"), privateLowerPath))
	}
	snapshotRoot := filepath.Join(base, "backups", "private", "snapshots")
	before := make(map[string]struct{}, len(value.PrivateSnapshots))
	for _, name := range value.PrivateSnapshots {
		before[name] = struct{}{}
	}
	if names, err := directoryNames(snapshotRoot); err != nil {
		result = errors.Join(result, err)
	} else {
		for _, name := range names {
			if _, keep := before[name]; !keep {
				result = errors.Join(result, os.RemoveAll(filepath.Join(snapshotRoot, name)))
			}
		}
	}
	return result
}

func validateUpdateJournalPathAndValue(root, journalPath string, value updateJournal) error {
	if value.Version != 1 {
		return errors.New("invalid update journal version")
	}
	if value.InstanceID == "" || filepath.Base(value.InstanceID) != value.InstanceID || value.InstanceID == "." || value.InstanceID == ".." {
		return errors.New("invalid update journal instance")
	}
	if value.Mode != Hot && value.Mode != Full {
		return errors.New("invalid update journal mode")
	}
	allowedStages := map[string]bool{"prepared": true, "applying": true, "deployed": true, "committed": true, "rolling_back": true}
	if !allowedStages[value.Stage] {
		return errors.New("invalid update journal stage")
	}
	if value.BackupRoot != "replaced" {
		return errors.New("invalid update journal backup root")
	}
	work := filepath.Dir(journalPath)
	backups := filepath.Dir(work)
	base := filepath.Dir(backups)
	expectedBase := filepath.Join(root, "instances", value.InstanceID)
	if filepath.Clean(base) != filepath.Clean(expectedBase) || filepath.Base(backups) != "backups" {
		return errors.New("update journal outside instance backups")
	}
	workName := filepath.Base(work)
	if !strings.HasPrefix(workName, "update-") {
		return errors.New("invalid update journal work name")
	}
	if _, err := uuid.Parse(strings.TrimPrefix(workName, "update-")); err != nil {
		return errors.New("invalid update journal work id")
	}
	if filepath.Clean(journalPath) != filepath.Join(work, "journal.json") {
		return errors.New("invalid update journal path")
	}
	trustedRoot := filepath.Clean(root)
	if err := validateUpdatePathNoSymlink(trustedRoot, work, true); err != nil {
		return err
	}
	if err := validateUpdatePathNoSymlink(trustedRoot, journalPath, false); err != nil {
		return err
	}
	for _, name := range []string{"replaced", "manifest.before", "private-manifest.before", "private-lower.before"} {
		path := filepath.Join(work, name)
		if _, err := os.Lstat(path); err == nil {
			if err = validateUpdatePathNoSymlink(trustedRoot, path, name == "replaced" || name == "private-lower.before"); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for _, backup := range []struct {
		path          string
		required, dir bool
	}{{filepath.Join(work, "manifest.before"), value.ManifestExisted, false}, {filepath.Join(work, "private-manifest.before"), value.PrivateManifestExisted, false}, {filepath.Join(work, "private-lower.before"), value.PrivateLowerExisted, true}} {
		_, statErr := os.Lstat(backup.path)
		if backup.required && errors.Is(statErr, os.ErrNotExist) {
			return errors.New("required update metadata backup is missing")
		}
		if !backup.required && statErr == nil {
			return errors.New("unexpected update metadata backup")
		}
		if statErr == nil {
			if err := validateUpdatePathNoSymlink(trustedRoot, backup.path, backup.dir); err != nil {
				return err
			}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
	}
	seen := map[string]journalEntry{}
	for _, entry := range value.Affected {
		if entry.Path == "" || strings.Contains(entry.Path, "\\") || filepath.IsAbs(entry.Path) || filepath.ToSlash(filepath.Clean(filepath.FromSlash(entry.Path))) != entry.Path || entry.Path == "." || strings.HasPrefix(entry.Path, "../") {
			return errors.New("invalid update journal affected path")
		}
		if _, ok := seen[entry.Path]; ok {
			return errors.New("duplicate update journal affected path")
		}
		if entry.Existed {
			if entry.Kind != "file" && entry.Kind != "directory" {
				return errors.New("invalid update journal affected kind")
			}
		} else if entry.Kind != "" {
			return errors.New("unexpected kind for absent update target")
		}
		seen[entry.Path] = entry
	}
	for path, entry := range seen {
		parts := strings.Split(path, "/")
		for i := 1; i < len(parts); i++ {
			ancestor := strings.Join(parts[:i], "/")
			if parent, ok := seen[ancestor]; ok && parent.Existed && parent.Kind == "file" {
				return errors.New("file update target has affected descendant")
			}
		}
		_ = entry
	}
	snapshotSeen := map[string]bool{}
	for _, id := range value.PrivateSnapshots {
		if !content.ValidPrivateSnapshotID(id) {
			return errors.New("invalid private snapshot id")
		}
		if snapshotSeen[id] {
			return errors.New("duplicate private snapshot id")
		}
		snapshotSeen[id] = true
		snapshotPath := filepath.Join(base, "backups", "private", "snapshots", id)
		if err := validateUpdatePathNoSymlink(trustedRoot, snapshotPath, true); err != nil {
			return err
		}
	}
	return nil
}

func validateUpdatePathNoSymlink(root, path string, wantDirectory bool) error {
	if err := rejectSymlinkPath(root, path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("symbolic links are forbidden")
	}
	if wantDirectory && !info.IsDir() {
		return errors.New("expected update journal directory")
	}
	if !wantDirectory && !info.Mode().IsRegular() {
		return errors.New("expected update journal file")
	}
	return nil
}

func directoryNames(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, errors.New("symbolic links are forbidden")
		}
		if entry.IsDir() {
			if !content.ValidPrivateSnapshotID(entry.Name()) {
				if strings.HasPrefix(entry.Name(), ".snapshot-") {
					continue
				}
				return nil, errors.New("invalid private snapshot id")
			}
			result = append(result, entry.Name())
		}
	}
	sort.Strings(result)
	return result, nil
}

func copyDirectory(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("symbolic links are forbidden")
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0750)
		}
		if !entry.Type().IsRegular() {
			return errors.New("only regular files are allowed")
		}
		return copyFile(path, destination)
	})
}

func collectFiles(root string, affected map[string]bool) error {
	if _, err := os.Lstat(root); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info.Type()&os.ModeSymlink != 0 {
			return errors.New("symbolic links are forbidden")
		}
		if info.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		affected[filepath.ToSlash(relative)] = true
		return nil
	})
}

func rejectSymlinkPath(root, target string) error {
	if info, err := os.Lstat(root); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New("symbolic links are forbidden")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("symbolic links are forbidden")
		}
	}
	return nil
}

func readJournal(path string) (updateJournal, error) {
	var value updateJournal
	raw, err := os.ReadFile(path)
	if err != nil {
		return value, err
	}
	err = json.Unmarshal(raw, &value)
	return value, err
}

func writeJournal(path string, value updateJournal) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, raw, 0640)
}

func extract(path, destination string, hashes map[string]string, mode Mode) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer reader.Close()
	root := archivecheck.CommonRoot(reader.File)
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		name := archivecheck.NormalizePath(entry.Name, root)
		if mode == Hot && !archivecheck.IsHotPath(name) {
			continue
		}
		target, err := safepath.Join(destination, name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
			return err
		}
		source, err := entry.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
		if err != nil {
			source.Close()
			return err
		}
		digest := sha256.New()
		_, copyErr := io.Copy(io.MultiWriter(output, digest), source)
		source.Close()
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		hashes[filepath.ToSlash(name)] = hex.EncodeToString(digest.Sum(nil))
	}
	return nil
}

func copyFile(source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	temporary := target + ".tmp"
	output, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	if copyErr == nil {
		copyErr = output.Sync()
	}
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(temporary, target); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(target))
}

func readManifest(path string) manifest {
	raw, err := os.ReadFile(path)
	if err != nil {
		return manifest{Files: map[string]string{}}
	}
	var value manifest
	if json.Unmarshal(raw, &value) != nil || value.Files == nil {
		return manifest{Files: map[string]string{}}
	}
	return value
}

func writeManifest(path string, value manifest) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, raw, 0640)
}

func writeAtomic(path string, raw []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err = file.Write(raw); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

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
}

func New(root string) *Pipeline { return &Pipeline{root: root} }

func (p *Pipeline) Apply(ctx context.Context, instanceID, archivePath, version string, mode Mode) error {
	transaction, err := p.Begin(ctx, instanceID, archivePath, version, mode)
	if err != nil {
		return err
	}
	return transaction.Commit()
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
		info, statErr := os.Stat(target)
		if statErr == nil && !info.IsDir() {
			entry.Existed = true
			destination, err := safepath.Join(backup, path)
			if err != nil {
				return nil, err
			}
			if err := copyFile(target, destination); err != nil {
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
	transaction := &deployment{pipeline: p, journalPath: journalPath, journal: value}
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
	private := content.NewPrivateManager(p.root, 1<<20)
	if err := private.RebaseAndApply(ctx, instanceID); err != nil {
		return fail(err)
	}
	if err := writeManifest(manifestPath, newManifest); err != nil {
		return fail(err)
	}
	transaction.journal.Stage = "deployed"
	if err := writeJournal(journalPath, transaction.journal); err != nil {
		return fail(err)
	}
	return transaction, nil
}

func (d *deployment) Commit() error {
	d.journal.Stage = "committed"
	if err := writeJournal(d.journalPath, d.journal); err != nil {
		return err
	}
	_ = os.RemoveAll(filepath.Dir(d.journalPath))
	return nil
}

func (d *deployment) Rollback() error {
	d.journal.Stage = "rolling_back"
	stageErr := writeJournal(d.journalPath, d.journal)
	rollbackErr := d.pipeline.rollbackJournal(d.journalPath, d.journal)
	if rollbackErr != nil {
		return errors.Join(stageErr, rollbackErr)
	}
	return errors.Join(stageErr, os.RemoveAll(filepath.Dir(d.journalPath)))
}

func (p *Pipeline) Recover(ctx context.Context) error {
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
	for _, entry := range value.Affected {
		target, err := safepath.Join(game, entry.Path)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		if entry.Existed {
			source, sourceErr := safepath.Join(backup, entry.Path)
			if sourceErr == nil {
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
		if entry.IsDir() {
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
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
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

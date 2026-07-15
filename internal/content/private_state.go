package content

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/not0721here/l4d2-control-panel/internal/safepath"
)

const privateManifestVersion = 1

type PrivateEntry struct {
	Path      string    `json:"path"`
	Kind      string    `json:"kind"`
	Hash      string    `json:"hash,omitempty"`
	Size      int64     `json:"size,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type PrivateChange struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	BeforeHash string `json:"before_hash,omitempty"`
	AfterHash  string `json:"after_hash,omitempty"`
}

type DiffSummary struct {
	Added    int `json:"added"`
	Modified int `json:"modified"`
	Deleted  int `json:"deleted"`
}

type PrivateDiff struct {
	Changes []PrivateChange `json:"changes"`
	Summary DiffSummary     `json:"summary"`
}

type PrivateSnapshot struct {
	ID        string      `json:"id"`
	AppliedAt time.Time   `json:"applied_at"`
	Summary   DiffSummary `json:"summary"`
}

type PrivateTarget struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

func (m *PrivateManager) TransactionTargets(_ context.Context, instanceID string) ([]PrivateTarget, error) {
	lock := m.instanceLock(instanceID)
	lock.Lock()
	defer lock.Unlock()
	if err := m.ensureBaselineLocked(instanceID); err != nil {
		return nil, err
	}
	root, err := m.privateRoot(instanceID)
	if err != nil {
		return nil, err
	}
	current, err := scanPrivateTree(root)
	if err != nil {
		return nil, err
	}
	applied, err := m.readPrivateManifest(instanceID)
	if err != nil {
		return nil, err
	}
	union := map[string]string{}
	for path, entry := range applied.Entries {
		union[path] = entry.Kind
	}
	for path, entry := range current {
		union[path] = entry.Kind
	}
	result := make([]PrivateTarget, 0, len(union))
	for path, kind := range union {
		result = append(result, PrivateTarget{Path: path, Kind: kind})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

type lowerManifest struct {
	Version         int               `json:"version"`
	Entries         map[string]bool   `json:"entries"`
	ControlledLinks map[string]string `json:"controlled_links,omitempty"`
}

type applyJournalEntry struct {
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	Existed bool   `json:"existed"`
	Target  string `json:"target,omitempty"`
}

type privateApplyJournal struct {
	Version         int                 `json:"version"`
	InstanceID      string              `json:"instance_id"`
	Stage           string              `json:"stage"`
	ManifestExisted bool                `json:"manifest_existed"`
	LowerExisted    bool                `json:"lower_existed"`
	Affected        []applyJournalEntry `json:"affected"`
	SnapshotID      string              `json:"snapshot_id,omitempty"`
}

type privateRestoreJournal struct {
	Version    int    `json:"version"`
	InstanceID string `json:"instance_id"`
	Stage      string `json:"stage"`
	HadOld     bool   `json:"had_old"`
}

var privateApplyFailureState struct {
	sync.RWMutex
	hook func(int) error
}

var privateSnapshotFailureState struct {
	sync.RWMutex
	hook func() error
}

var privateCleanupFailureState struct {
	sync.RWMutex
	hook func(string) error
}

var privateRestoreFailureState struct {
	sync.RWMutex
	hook func(string) error
}

func setPrivateRestoreFailureHook(hook func(string) error) {
	privateRestoreFailureState.Lock()
	privateRestoreFailureState.hook = hook
	privateRestoreFailureState.Unlock()
}

func runPrivateRestoreFailureHook(phase string) error {
	privateRestoreFailureState.RLock()
	hook := privateRestoreFailureState.hook
	privateRestoreFailureState.RUnlock()
	if hook != nil {
		return hook(phase)
	}
	return nil
}

func setPrivateCleanupFailureHook(hook func(string) error) {
	privateCleanupFailureState.Lock()
	privateCleanupFailureState.hook = hook
	privateCleanupFailureState.Unlock()
}
func runPrivateCleanupFailureHook(phase string) error {
	privateCleanupFailureState.RLock()
	hook := privateCleanupFailureState.hook
	privateCleanupFailureState.RUnlock()
	if hook != nil {
		return hook(phase)
	}
	return nil
}

var privateSnapshotIDPattern = regexp.MustCompile(`^\d{8}T\d{6}\.\d{9}Z-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func ValidPrivateSnapshotID(id string) bool {
	return filepath.Base(id) == id && privateSnapshotIDPattern.MatchString(id)
}

func setPrivateSnapshotFailureHook(hook func() error) {
	privateSnapshotFailureState.Lock()
	privateSnapshotFailureState.hook = hook
	privateSnapshotFailureState.Unlock()
}
func runPrivateSnapshotFailureHook() error {
	privateSnapshotFailureState.RLock()
	hook := privateSnapshotFailureState.hook
	privateSnapshotFailureState.RUnlock()
	if hook != nil {
		return hook()
	}
	return nil
}

func setPrivateApplyFailureHook(hook func(int) error) {
	privateApplyFailureState.Lock()
	privateApplyFailureState.hook = hook
	privateApplyFailureState.Unlock()
}

func runPrivateApplyFailureHook(count int) error {
	privateApplyFailureState.RLock()
	hook := privateApplyFailureState.hook
	privateApplyFailureState.RUnlock()
	if hook != nil {
		return hook(count)
	}
	return nil
}

type privateManifest struct {
	Version   int                      `json:"version"`
	AppliedAt time.Time                `json:"applied_at"`
	Entries   map[string]manifestEntry `json:"entries"`
	Migration string                   `json:"migration,omitempty"`
}

func controlledSharedVPKTarget(path string) (string, bool) {
	if strings.Contains(path, "\\") || filepath.IsAbs(path) || filepath.ToSlash(filepath.Clean(filepath.FromSlash(path))) != path {
		return "", false
	}
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] != "addons" || parts[1] == "" || filepath.Base(parts[1]) != parts[1] || !strings.HasSuffix(strings.ToLower(parts[1]), ".vpk") {
		return "", false
	}
	return "/opt/l4d2/shared-vpk/" + parts[1], true
}

func validateControlledSharedVPK(path, target string) error {
	want, ok := controlledSharedVPKTarget(path)
	if !ok || filepath.ToSlash(target) != want {
		return errors.New("invalid controlled shared VPK symlink")
	}
	return nil
}

type manifestEntry struct {
	Kind      string    `json:"kind"`
	Hash      string    `json:"hash,omitempty"`
	Size      int64     `json:"size,omitempty"`
	UpdatedAt time.Time `json:"-"`
}

func scanPrivateTree(root string) (map[string]manifestEntry, error) {
	result := make(map[string]manifestEntry)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("symbolic links are forbidden")
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		item := manifestEntry{UpdatedAt: info.ModTime()}
		if info.IsDir() {
			item.Kind = "directory"
		} else if info.Mode().IsRegular() {
			item.Kind = "file"
			item.Size = info.Size()
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			digest := sha256.Sum256(raw)
			item.Hash = hex.EncodeToString(digest[:])
		} else {
			return errors.New("only regular files and directories are allowed")
		}
		result[filepath.ToSlash(relative)] = item
		return nil
	})
	return result, err
}

func (m *PrivateManager) manifestPath(instanceID string) (string, error) {
	if err := validateInstanceID(instanceID); err != nil {
		return "", err
	}
	return filepath.Join(m.root, "instances", instanceID, "private-applied.json"), nil
}

func (m *PrivateManager) readPrivateManifest(instanceID string) (privateManifest, error) {
	path, err := m.manifestPath(instanceID)
	if err != nil {
		return privateManifest{}, err
	}
	if err := rejectSymlinkParents(m.root, path); err != nil {
		return privateManifest{}, err
	}
	raw, err := readAtomicFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return privateManifest{Version: privateManifestVersion, Entries: map[string]manifestEntry{}}, nil
	}
	if err != nil {
		return privateManifest{}, err
	}
	var manifest privateManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return privateManifest{}, err
	}
	if manifest.Version != privateManifestVersion {
		return privateManifest{}, errors.New("unsupported private manifest version")
	}
	if manifest.Entries == nil {
		manifest.Entries = map[string]manifestEntry{}
	}
	for path, entry := range manifest.Entries {
		if err := validateManifestEntry(path, entry); err != nil {
			return privateManifest{}, err
		}
	}
	return manifest, nil
}

func validateManifestEntry(path string, entry manifestEntry) error {
	if path == "" || strings.Contains(path, "\\") || filepath.IsAbs(path) || filepath.ToSlash(filepath.Clean(filepath.FromSlash(path))) != path || path == "." || strings.HasPrefix(path, "../") {
		return errors.New("invalid private manifest path")
	}
	switch entry.Kind {
	case "directory":
		if entry.Hash != "" || entry.Size != 0 {
			return errors.New("invalid private manifest directory")
		}
	case "file":
		hash, err := hex.DecodeString(entry.Hash)
		if err != nil || len(hash) != sha256.Size || entry.Size < 0 {
			return errors.New("invalid private manifest file")
		}
	default:
		return errors.New("invalid private manifest kind")
	}
	return nil
}

func (m *PrivateManager) writePrivateManifest(instanceID string, manifest privateManifest) error {
	path, err := m.manifestPath(instanceID)
	if err != nil {
		return err
	}
	if err := rejectSymlinkParents(m.root, path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	manifest.Version = privateManifestVersion
	raw, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".private-manifest-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0640); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return atomicReplaceFile(temporaryName, path)
}

// ensureBaselineLocked migrates instances created before private-applied.json existed.
// The caller must hold the instance write lock so no workspace mutation can race it.
func (m *PrivateManager) ensureBaselineLocked(instanceID string) error {
	manifestPath, err := m.manifestPath(instanceID)
	if err != nil {
		return err
	}
	if _, err = os.Lstat(manifestPath); err == nil {
		_, err = m.readPrivateManifest(instanceID)
		return err
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	workspace, err := m.privateRoot(instanceID)
	if err != nil {
		return err
	}
	entries, err := scanPrivateTree(workspace)
	if err != nil {
		return err
	}
	base := filepath.Join(m.root, "instances", instanceID)
	game := filepath.Join(base, "game", "left4dead2")
	lowerRoot := filepath.Join(base, "backups", "private", "lower")
	lower, err := readLowerManifest(filepath.Join(lowerRoot, "state.json"))
	if err != nil {
		return err
	}
	for path, entry := range entries {
		if entry.Kind != "file" {
			continue
		}
		target, _ := safepath.Join(game, path)
		if err := rejectSymlinkParents(game, filepath.Dir(target)); err != nil {
			return err
		}
		info, statErr := os.Lstat(target)
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			link, readErr := os.Readlink(target)
			if readErr != nil {
				return readErr
			}
			if err := validateControlledSharedVPK(path, link); err != nil {
				return err
			}
			lower.Entries[path] = true
			lower.ControlledLinks[path] = filepath.ToSlash(link)
		}
	}
	if len(lower.Entries) > 0 {
		if err := writeJSONAtomic(filepath.Join(lowerRoot, "state.json"), lower); err != nil {
			return err
		}
	}
	marker := "legacy-v1"
	if len(entries) == 0 {
		marker = "empty-v1"
	}
	return m.writePrivateManifest(instanceID, privateManifest{Version: privateManifestVersion, AppliedAt: time.Now().UTC(), Entries: entries, Migration: marker})
}

func isEmptyDirectory(path string, entries map[string]manifestEntry) bool {
	prefix := path + "/"
	for candidate := range entries {
		if strings.HasPrefix(candidate, prefix) {
			return false
		}
	}
	return true
}

func isDiffResource(path string, entry manifestEntry, entries map[string]manifestEntry) bool {
	return entry.Kind == "file" || isEmptyDirectory(path, entries)
}

func (m *PrivateManager) Diff(_ context.Context, instanceID string) (PrivateDiff, error) {
	lock := m.instanceLock(instanceID)
	lock.Lock()
	defer lock.Unlock()
	if err := m.ensureBaselineLocked(instanceID); err != nil {
		return PrivateDiff{}, err
	}
	root, err := m.privateRoot(instanceID)
	if err != nil {
		return PrivateDiff{}, err
	}
	current, err := scanPrivateTree(root)
	if err != nil {
		return PrivateDiff{}, err
	}
	applied, err := m.readPrivateManifest(instanceID)
	if err != nil {
		return PrivateDiff{}, err
	}
	result := PrivateDiff{Changes: []PrivateChange{}}
	paths := make(map[string]struct{}, len(current)+len(applied.Entries))
	for path := range current {
		paths[path] = struct{}{}
	}
	for path := range applied.Entries {
		paths[path] = struct{}{}
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	for _, path := range ordered {
		after, hasAfter := current[path]
		before, hasBefore := applied.Entries[path]
		beforeIsResource := hasBefore && isDiffResource(path, before, applied.Entries)
		afterIsResource := hasAfter && isDiffResource(path, after, current)
		if !beforeIsResource && !afterIsResource {
			continue
		}
		switch {
		case !hasBefore && hasAfter:
			result.Changes = append(result.Changes, PrivateChange{Path: path, Kind: "added", AfterHash: after.Hash})
			result.Summary.Added++
		case hasBefore && !hasAfter:
			result.Changes = append(result.Changes, PrivateChange{Path: path, Kind: "deleted", BeforeHash: before.Hash})
			result.Summary.Deleted++
		case before.Kind != after.Kind || before.Hash != after.Hash:
			result.Changes = append(result.Changes, PrivateChange{Path: path, Kind: "modified", BeforeHash: before.Hash, AfterHash: after.Hash})
			result.Summary.Modified++
		}
	}
	return result, nil
}

func privateDiff(current map[string]manifestEntry, applied privateManifest) PrivateDiff {
	result := PrivateDiff{Changes: []PrivateChange{}}
	paths := make(map[string]struct{}, len(current)+len(applied.Entries))
	for path := range current {
		paths[path] = struct{}{}
	}
	for path := range applied.Entries {
		paths[path] = struct{}{}
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	for _, path := range ordered {
		a, hasAfter := current[path]
		b, hasBefore := applied.Entries[path]
		beforeResource := hasBefore && isDiffResource(path, b, applied.Entries)
		afterResource := hasAfter && isDiffResource(path, a, current)
		if !beforeResource && !afterResource {
			continue
		}
		switch {
		case !hasBefore && hasAfter:
			result.Changes = append(result.Changes, PrivateChange{Path: path, Kind: "added", AfterHash: a.Hash})
			result.Summary.Added++
		case hasBefore && !hasAfter:
			result.Changes = append(result.Changes, PrivateChange{Path: path, Kind: "deleted", BeforeHash: b.Hash})
			result.Summary.Deleted++
		case b.Kind != a.Kind || b.Hash != a.Hash:
			result.Changes = append(result.Changes, PrivateChange{Path: path, Kind: "modified", BeforeHash: b.Hash, AfterHash: a.Hash})
			result.Summary.Modified++
		}
	}
	return result
}

func (m *PrivateManager) ApplyChanges(ctx context.Context, instanceID string) error {
	return m.applyPrivate(ctx, instanceID, false, true, nil)
}

func (m *PrivateManager) ApplyChangesWithProgress(ctx context.Context, instanceID string, progress func(string)) error {
	return m.applyPrivate(ctx, instanceID, false, true, progress)
}

func (m *PrivateManager) RebaseAndApply(ctx context.Context, instanceID string) error {
	return m.applyPrivate(ctx, instanceID, true, true, nil)
}

func (m *PrivateManager) applyPrivate(ctx context.Context, instanceID string, rebase, prune bool, progress func(string)) error {
	lock := m.instanceLock(instanceID)
	lock.Lock()
	defer lock.Unlock()
	return m.applyPrivateLocked(ctx, instanceID, rebase, prune, progress)
}

func (m *PrivateManager) applyPrivateLocked(ctx context.Context, instanceID string, rebase, prune bool, reporters ...func(string)) error {
	report := func(stage string) {
		if len(reporters) > 0 && reporters[0] != nil {
			reporters[0](stage)
		}
	}
	if err := validateInstanceID(instanceID); err != nil {
		return err
	}
	base := filepath.Join(m.root, "instances", instanceID)
	if err := m.recoverPrivateInstanceLocked(ctx, instanceID, base); err != nil {
		return err
	}
	if err := m.ensureBaselineLocked(instanceID); err != nil {
		return err
	}
	workspace := filepath.Join(base, "private")
	game := filepath.Join(base, "game", "left4dead2")
	if err := rejectSymlinkParents(m.root, workspace); err != nil {
		return err
	}
	current, err := scanPrivateTree(workspace)
	if err != nil {
		return err
	}
	old, err := m.readPrivateManifest(instanceID)
	if err != nil {
		return err
	}
	manifestPath, _ := m.manifestPath(instanceID)
	lowerRoot := filepath.Join(base, "backups", "private", "lower")
	lower, err := readLowerManifest(filepath.Join(lowerRoot, "state.json"))
	if err != nil {
		return err
	}
	pathsMap := map[string]struct{}{}
	for path := range current {
		pathsMap[path] = struct{}{}
	}
	for path := range old.Entries {
		pathsMap[path] = struct{}{}
	}
	paths := make([]string, 0, len(pathsMap))
	for path := range pathsMap {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	work := filepath.Join(base, "backups", "private", "apply-"+uuid.NewString())
	report("snapshot")
	journal := privateApplyJournal{Version: 1, InstanceID: instanceID, Stage: "prepared"}
	if _, statErr := os.Stat(manifestPath); statErr == nil {
		journal.ManifestExisted = true
		if err = copyFileExact(manifestPath, filepath.Join(work, "manifest.before")); err != nil {
			return err
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	if _, statErr := os.Stat(lowerRoot); statErr == nil {
		journal.LowerExisted = true
		if err = copyTreeExact(lowerRoot, filepath.Join(work, "lower.before")); err != nil {
			return err
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	for _, path := range paths {
		target, joinErr := safepath.Join(game, path)
		if joinErr != nil {
			return joinErr
		}
		if err = rejectSymlinkParents(game, filepath.Dir(target)); err != nil {
			return err
		}
		kind := "file"
		if currentEntry, ok := current[path]; ok {
			kind = currentEntry.Kind
		} else if oldEntry, ok := old.Entries[path]; ok {
			kind = oldEntry.Kind
		}
		entry := applyJournalEntry{Path: path, Kind: kind}
		info, statErr := os.Lstat(target)
		if statErr == nil {
			if kind == "directory" {
				if !info.IsDir() {
					return errors.New("private directory target is not a directory")
				}
			} else if info.Mode()&os.ModeSymlink != 0 {
				link, readErr := os.Readlink(target)
				if readErr != nil || validateControlledSharedVPK(path, link) != nil {
					return errors.New("private target is not a regular file or controlled shared VPK link")
				}
				entry.Kind, entry.Target = "controlled_symlink", filepath.ToSlash(link)
			} else if !info.Mode().IsRegular() {
				return errors.New("private target is not a regular file")
			}
			entry.Existed = true
			if kind == "file" && entry.Kind != "controlled_symlink" {
				backup, _ := safepath.Join(filepath.Join(work, "game.before"), path)
				if err = copyFileExact(target, backup); err != nil {
					return err
				}
			}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		journal.Affected = append(journal.Affected, entry)
	}
	if err = writeJSONAtomic(filepath.Join(work, "journal.json"), journal); err != nil {
		return err
	}
	rollback := func(cause error) error {
		journal.Stage = "rolling_back"
		_ = writeJSONAtomic(filepath.Join(work, "journal.json"), journal)
		return errors.Join(cause, rollbackPrivateApply(work, base, journal))
	}
	journal.Stage = "applying"
	if err = writeJSONAtomic(filepath.Join(work, "journal.json"), journal); err != nil {
		return rollback(err)
	}
	if err = ctx.Err(); err != nil {
		return rollback(err)
	}
	if rebase {
		report("restore-lower")
		lower = lowerManifest{Version: 1, Entries: map[string]bool{}, ControlledLinks: map[string]string{}}
		_ = os.RemoveAll(filepath.Join(lowerRoot, "tree"))
		for _, path := range paths {
			entry, ok := current[path]
			if !ok {
				entry = old.Entries[path]
			}
			if err = captureLower(game, lowerRoot, path, entry.Kind, &lower); err != nil {
				return rollback(err)
			}
		}
	} else {
		report("restore-lower")
		for path, entry := range current {
			if _, wasApplied := old.Entries[path]; !wasApplied {
				if _, known := lower.Entries[path]; !known {
					if err = captureLower(game, lowerRoot, path, entry.Kind, &lower); err != nil {
						return rollback(err)
					}
				}
			}
		}
	}
	if err = writeJSONAtomic(filepath.Join(lowerRoot, "state.json"), lower); err != nil {
		return rollback(err)
	}
	mutations := 0
	report("apply-private")
	for _, path := range paths {
		entry, nowPrivate := current[path]
		if nowPrivate && entry.Kind == "directory" {
			target, _ := safepath.Join(game, path)
			err = os.MkdirAll(target, 0750)
		} else if nowPrivate && entry.Kind == "file" {
			source, _ := safepath.Join(workspace, path)
			target, _ := safepath.Join(game, path)
			err = copyFileExact(source, target)
		} else if oldEntry, wasPrivate := old.Entries[path]; wasPrivate && oldEntry.Kind == "file" {
			target, _ := safepath.Join(game, path)
			if link := lower.ControlledLinks[path]; link != "" {
				err = restoreControlledLink(target, path, link)
			} else if lower.Entries[path] {
				source, _ := safepath.Join(filepath.Join(lowerRoot, "tree"), path)
				err = copyFileExact(source, target)
			} else {
				err = os.Remove(target)
				if errors.Is(err, os.ErrNotExist) {
					err = nil
				}
			}
			delete(lower.Entries, path)
			delete(lower.ControlledLinks, path)
		} else if oldEntry, wasPrivate := old.Entries[path]; wasPrivate && oldEntry.Kind == "directory" {
			if !lower.Entries[path] {
				target, _ := safepath.Join(game, path)
				err = os.Remove(target)
				if errors.Is(err, os.ErrNotExist) {
					err = nil
				}
			}
			delete(lower.Entries, path)
		}
		if err != nil {
			return rollback(err)
		}
		mutations++
		if err = runPrivateApplyFailureHook(mutations); err != nil {
			return rollback(err)
		}
	}
	if err = writeJSONAtomic(filepath.Join(lowerRoot, "state.json"), lower); err != nil {
		return rollback(err)
	}
	now := time.Now().UTC()
	report("commit")
	next := privateManifest{Version: privateManifestVersion, AppliedAt: now, Entries: current}
	if err = m.writePrivateManifest(instanceID, next); err != nil {
		return rollback(err)
	}
	diff := privateDiff(current, old)
	journal.Stage = "snapshotting"
	journal.SnapshotID = time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + uuid.NewString()
	if err = writeJSONAtomic(filepath.Join(work, "journal.json"), journal); err != nil {
		return rollback(err)
	}
	if err = createPrivateSnapshot(base, workspace, next, diff.Summary, journal.SnapshotID); err != nil {
		return rollback(err)
	}
	journal.Stage = "committed"
	if err = writeJSONAtomic(filepath.Join(work, "journal.json"), journal); err != nil {
		return rollback(err)
	}
	if prune {
		m.cleanupPrivate(base, "prune", func() error { return prunePrivateSnapshots(base, 20) })
	}
	m.cleanupPrivate(base, "work", func() error { return os.RemoveAll(work) })
	return nil
}

func (m *PrivateManager) cleanupPrivate(base, phase string, operation func() error) {
	err := runPrivateCleanupFailureHook(phase)
	if err == nil {
		err = operation()
	}
	if err != nil {
		_ = recordPrivateDiagnostic(base, phase, err)
	}
}
func recordPrivateDiagnostic(base, phase string, cause error) error {
	value := struct {
		At    time.Time `json:"at"`
		Phase string    `json:"phase"`
		Error string    `json:"error"`
	}{time.Now().UTC(), phase, cause.Error()}
	return writeJSONAtomic(filepath.Join(base, "backups", "private", "diagnostics", time.Now().UTC().Format("20060102T150405.000000000Z")+"-"+uuid.NewString()+".json"), value)
}

type PrivateTransaction struct {
	manager          *PrivateManager
	instanceID, base string
	lock             *sync.RWMutex
	closed           bool
}

func (m *PrivateManager) BeginTransaction(ctx context.Context, instanceID string) (*PrivateTransaction, error) {
	lock := m.instanceLock(instanceID)
	lock.Lock()
	if err := validateInstanceID(instanceID); err != nil {
		lock.Unlock()
		return nil, err
	}
	base := filepath.Join(m.root, "instances", instanceID)
	if err := m.recoverPrivateInstanceLocked(ctx, instanceID, base); err != nil {
		lock.Unlock()
		return nil, err
	}
	if err := m.ensureBaselineLocked(instanceID); err != nil {
		lock.Unlock()
		return nil, err
	}
	return &PrivateTransaction{manager: m, instanceID: instanceID, base: base, lock: lock}, nil
}
func (t *PrivateTransaction) Targets(ctx context.Context) ([]PrivateTarget, error) {
	if t.closed {
		return nil, errors.New("private transaction closed")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := t.manager.privateRoot(t.instanceID)
	if err != nil {
		return nil, err
	}
	current, err := scanPrivateTree(root)
	if err != nil {
		return nil, err
	}
	applied, err := t.manager.readPrivateManifest(t.instanceID)
	if err != nil {
		return nil, err
	}
	union := map[string]string{}
	for path, entry := range applied.Entries {
		union[path] = entry.Kind
	}
	for path, entry := range current {
		union[path] = entry.Kind
	}
	result := make([]PrivateTarget, 0, len(union))
	for path, kind := range union {
		result = append(result, PrivateTarget{Path: path, Kind: kind})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}
func (t *PrivateTransaction) RebaseAndApply(ctx context.Context) error {
	if t.closed {
		return errors.New("private transaction closed")
	}
	return t.manager.applyPrivateLocked(ctx, t.instanceID, true, false)
}
func (t *PrivateTransaction) Commit() error {
	if t.closed {
		return errors.New("private transaction closed")
	}
	t.manager.cleanupPrivate(t.base, "prune", func() error { return prunePrivateSnapshots(t.base, 20) })
	t.closed = true
	t.lock.Unlock()
	return nil
}
func (t *PrivateTransaction) Rollback() error {
	if t.closed {
		return nil
	}
	t.closed = true
	t.lock.Unlock()
	return nil
}

func captureLower(game, lowerRoot, path, kind string, lower *lowerManifest) error {
	target, _ := safepath.Join(game, path)
	if err := rejectSymlinkParents(game, filepath.Dir(target)); err != nil {
		return err
	}
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		lower.Entries[path] = false
		return nil
	}
	if err != nil {
		return err
	}
	if kind == "directory" {
		if !info.IsDir() {
			return errors.New("lower directory target is not a directory")
		}
		lower.Entries[path] = true
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		link, readErr := os.Readlink(target)
		if readErr != nil {
			return readErr
		}
		if err := validateControlledSharedVPK(path, link); err != nil {
			return err
		}
		lower.Entries[path] = true
		if lower.ControlledLinks == nil {
			lower.ControlledLinks = map[string]string{}
		}
		lower.ControlledLinks[path] = filepath.ToSlash(link)
		return nil
	}
	if !info.Mode().IsRegular() {
		return errors.New("lower target is not a regular file")
	}
	source := target
	destination, _ := safepath.Join(filepath.Join(lowerRoot, "tree"), path)
	if err = copyFileExact(source, destination); err != nil {
		return err
	}
	lower.Entries[path] = true
	return nil
}

func rollbackPrivateApply(work, base string, journal privateApplyJournal) error {
	game := filepath.Join(base, "game", "left4dead2")
	var result error
	for i := len(journal.Affected) - 1; i >= 0; i-- {
		entry := journal.Affected[i]
		target, err := safepath.Join(game, entry.Path)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		if entry.Kind == "directory" {
			if !entry.Existed {
				if err = os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
					result = errors.Join(result, err)
				}
			}
		} else if entry.Existed && entry.Kind == "controlled_symlink" {
			result = errors.Join(result, restoreControlledLink(target, entry.Path, entry.Target))
		} else if entry.Existed {
			source, _ := safepath.Join(filepath.Join(work, "game.before"), entry.Path)
			result = errors.Join(result, copyFileExact(source, target))
		} else if err = os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, err)
		}
	}
	manifestPath := filepath.Join(base, "private-applied.json")
	if journal.ManifestExisted {
		result = errors.Join(result, copyFileExact(filepath.Join(work, "manifest.before"), manifestPath))
	} else if err := os.Remove(manifestPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		result = errors.Join(result, err)
	}
	lowerRoot := filepath.Join(base, "backups", "private", "lower")
	_ = os.RemoveAll(lowerRoot)
	if journal.LowerExisted {
		result = errors.Join(result, copyTreeExact(filepath.Join(work, "lower.before"), lowerRoot))
	}
	if journal.SnapshotID != "" {
		result = errors.Join(result, os.RemoveAll(filepath.Join(base, "backups", "private", "snapshots", journal.SnapshotID)))
	}
	if result == nil {
		result = os.RemoveAll(work)
	}
	return result
}

func readLowerManifest(path string) (lowerManifest, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return lowerManifest{Version: 1, Entries: map[string]bool{}, ControlledLinks: map[string]string{}}, nil
	}
	if err != nil {
		return lowerManifest{}, err
	}
	var value lowerManifest
	if err = json.Unmarshal(raw, &value); err != nil {
		return value, err
	}
	if value.Version != 1 || value.Entries == nil {
		return value, errors.New("invalid private lower manifest")
	}
	if value.ControlledLinks == nil {
		value.ControlledLinks = map[string]string{}
	}
	for path := range value.Entries {
		if err = validateManifestEntry(path, manifestEntry{Kind: "file", Hash: strings.Repeat("0", 64)}); err != nil {
			return value, err
		}
	}
	for path, target := range value.ControlledLinks {
		if !value.Entries[path] {
			return value, errors.New("controlled lower link must exist")
		}
		if err := validateControlledSharedVPK(path, target); err != nil {
			return value, err
		}
	}
	return value, nil
}

func restoreControlledLink(path, relative, target string) error {
	if err := validateControlledSharedVPK(relative, target); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	return os.Symlink(target, path)
}

func writeJSONAtomic(path string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".private-state-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err = temp.Chmod(0640); err == nil {
		_, err = temp.Write(raw)
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return atomicReplaceFile(name, path)
}

func copyFileExact(source, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
		return err
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	temp, err := os.CreateTemp(filepath.Dir(target), ".private-copy-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err = temp.Chmod(0640); err == nil {
		_, err = io.Copy(temp, input)
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return atomicReplaceFile(name, target)
}

func copyTreeExact(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("symbolic links are forbidden")
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(dst, 0750)
		}
		if !entry.Type().IsRegular() {
			return errors.New("only regular files and directories are allowed")
		}
		return copyFileExact(path, dst)
	})
}

func copyPrivateRestoreTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("symbolic links are forbidden")
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(target, rel)
		if entry.IsDir() {
			err = os.MkdirAll(dst, 0750)
		} else if !entry.Type().IsRegular() {
			err = errors.New("only regular files and directories are allowed")
		} else {
			err = copyFileExact(path, dst)
		}
		if err != nil {
			return err
		}
		return runPrivateRestoreFailureHook("copy")
	})
}

func createPrivateSnapshot(base, workspace string, manifest privateManifest, summary DiffSummary, id string) error {
	if err := runPrivateSnapshotFailureHook(); err != nil {
		return err
	}
	snapshots := filepath.Join(base, "backups", "private", "snapshots")
	if err := os.MkdirAll(snapshots, 0750); err != nil {
		return err
	}
	temporary := filepath.Join(snapshots, ".snapshot-"+uuid.NewString())
	defer os.RemoveAll(temporary)
	tree := filepath.Join(temporary, "tree")
	var snapshotErr error
	if _, statErr := os.Stat(workspace); errors.Is(statErr, os.ErrNotExist) {
		snapshotErr = os.MkdirAll(tree, 0750)
	} else if statErr != nil {
		snapshotErr = statErr
	} else {
		snapshotErr = copyTreeExact(workspace, tree)
	}
	if snapshotErr != nil {
		return snapshotErr
	}
	meta := struct {
		PrivateSnapshot
		Manifest privateManifest `json:"manifest"`
	}{PrivateSnapshot: PrivateSnapshot{ID: id, AppliedAt: manifest.AppliedAt, Summary: summary}, Manifest: manifest}
	if err := writeJSONAtomic(filepath.Join(temporary, "snapshot.json"), meta); err != nil {
		return err
	}
	return os.Rename(temporary, filepath.Join(snapshots, id))
}

func (m *PrivateManager) Snapshots(_ context.Context, instanceID string) ([]PrivateSnapshot, error) {
	lock := m.instanceLock(instanceID)
	lock.Lock()
	defer lock.Unlock()
	if err := m.ensureBaselineLocked(instanceID); err != nil {
		return nil, err
	}
	if err := validateInstanceID(instanceID); err != nil {
		return nil, err
	}
	root := filepath.Join(m.root, "instances", instanceID, "backups", "private", "snapshots")
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return []PrivateSnapshot{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []PrivateSnapshot{}
	for _, entry := range entries {
		if !entry.IsDir() || filepath.Base(entry.Name()) != entry.Name() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		var value struct{ PrivateSnapshot }
		raw, readErr := os.ReadFile(filepath.Join(root, entry.Name(), "snapshot.json"))
		if readErr != nil {
			return nil, readErr
		}
		if err = json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		if value.ID != entry.Name() {
			return nil, errors.New("invalid private snapshot identity")
		}
		result = append(result, value.PrivateSnapshot)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].AppliedAt.After(result[j].AppliedAt) })
	return result, nil
}

func (m *PrivateManager) Recover(ctx context.Context) error {
	instances := filepath.Join(m.root, "instances")
	entries, err := os.ReadDir(instances)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var result error
	for _, entry := range entries {
		if !entry.IsDir() || validateInstanceID(entry.Name()) != nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			return errors.Join(result, err)
		}
		instanceID := entry.Name()
		base := filepath.Join(instances, instanceID)
		lock := m.instanceLock(instanceID)
		lock.Lock()
		recoverErr := m.recoverPrivateInstanceLocked(ctx, instanceID, base)
		lock.Unlock()
		result = errors.Join(result, recoverErr)
	}
	return result
}

func (m *PrivateManager) recoverPrivateInstanceLocked(ctx context.Context, instanceID, base string) error {
	var result error
	for _, kind := range []string{"apply-*", "restore-*"} {
		paths, err := filepath.Glob(filepath.Join(base, "backups", "private", kind, "journal.json"))
		if err != nil {
			return errors.Join(result, err)
		}
		for _, path := range paths {
			if err := ctx.Err(); err != nil {
				return errors.Join(result, err)
			}
			if strings.HasPrefix(filepath.Base(filepath.Dir(path)), "restore-") {
				result = errors.Join(result, m.recoverPrivateRestoreLocked(path, base, instanceID))
			} else {
				result = errors.Join(result, m.recoverPrivateJournalLocked(path, base, instanceID))
			}
		}
	}
	works, err := filepath.Glob(filepath.Join(base, "backups", "private", "restore-*"))
	if err != nil {
		return errors.Join(result, err)
	}
	for _, work := range works {
		if err := ctx.Err(); err != nil {
			return errors.Join(result, err)
		}
		name := filepath.Base(work)
		if _, err := uuid.Parse(strings.TrimPrefix(name, "restore-")); err != nil {
			continue
		}
		journal := filepath.Join(work, "journal.json")
		if _, err := os.Lstat(journal); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, err)
			continue
		}
		if err := rejectSymlinkParents(filepath.Join(base, "backups", "private"), work); err != nil {
			result = errors.Join(result, err)
			continue
		}
		result = errors.Join(result, os.RemoveAll(work))
	}
	return result
}

func (m *PrivateManager) recoverPrivateJournalLocked(journalPath, base, instanceID string) error {
	work := filepath.Dir(journalPath)
	expectedRoot := filepath.Join(base, "backups", "private")
	relative, relErr := filepath.Rel(expectedRoot, work)
	if relErr != nil || strings.Contains(relative, string(filepath.Separator)) || !strings.HasPrefix(relative, "apply-") {
		return errors.New("invalid private apply journal path")
	}
	if _, err := uuid.Parse(strings.TrimPrefix(relative, "apply-")); err != nil {
		return errors.New("invalid private apply journal id")
	}
	if err := rejectSymlinkParents(expectedRoot, journalPath); err != nil {
		return err
	}
	if info, err := os.Lstat(journalPath); err != nil {
		return err
	} else if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("symbolic links are forbidden")
	}
	raw, err := os.ReadFile(journalPath)
	if err != nil {
		return err
	}
	var journal privateApplyJournal
	if err = json.Unmarshal(raw, &journal); err != nil {
		return err
	}
	if journal.Version != 1 || journal.InstanceID != instanceID || filepath.Base(instanceID) != instanceID {
		return errors.New("invalid private apply journal identity")
	}
	allowed := map[string]bool{"prepared": true, "applying": true, "snapshotting": true, "rolling_back": true, "committed": true}
	if !allowed[journal.Stage] {
		return errors.New("invalid private apply journal stage")
	}
	if journal.SnapshotID != "" && !ValidPrivateSnapshotID(journal.SnapshotID) {
		return errors.New("invalid private snapshot id")
	}
	for _, entry := range journal.Affected {
		if entry.Kind != "file" && entry.Kind != "directory" && entry.Kind != "controlled_symlink" {
			return errors.New("invalid private apply journal kind")
		}
		manifestKind := entry.Kind
		if entry.Kind == "controlled_symlink" {
			manifestKind = "file"
			if !entry.Existed {
				return errors.New("controlled private apply link must exist")
			}
			if err := validateControlledSharedVPK(entry.Path, entry.Target); err != nil {
				return err
			}
		} else if entry.Target != "" {
			return errors.New("unexpected private apply link target")
		}
		if err = validateManifestEntry(entry.Path, manifestEntry{Kind: manifestKind, Hash: func() string {
			if manifestKind == "file" {
				return strings.Repeat("0", 64)
			}
			return ""
		}()}); err != nil {
			return err
		}
	}
	if journal.Stage == "committed" {
		return os.RemoveAll(work)
	}
	return rollbackPrivateApply(work, base, journal)
}

func (m *PrivateManager) RestoreSnapshot(ctx context.Context, instanceID, snapshotID string) error {
	lock := m.instanceLock(instanceID)
	lock.Lock()
	defer lock.Unlock()
	if err := validateInstanceID(instanceID); err != nil {
		return err
	}
	if !ValidPrivateSnapshotID(snapshotID) {
		return errors.New("invalid snapshot id")
	}
	base := filepath.Join(m.root, "instances", instanceID)
	if err := m.recoverPrivateInstanceLocked(ctx, instanceID, base); err != nil {
		return err
	}
	if err := m.ensureBaselineLocked(instanceID); err != nil {
		return err
	}
	snapshot := filepath.Join(base, "backups", "private", "snapshots", snapshotID)
	source := filepath.Join(snapshot, "tree")
	if err := rejectSymlinkParents(base, source); err != nil {
		return err
	}
	var metadata struct {
		PrivateSnapshot
		Manifest privateManifest `json:"manifest"`
	}
	raw, err := os.ReadFile(filepath.Join(snapshot, "snapshot.json"))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, &metadata); err != nil || metadata.ID != snapshotID || metadata.Manifest.Version != privateManifestVersion || metadata.Manifest.Entries == nil {
		return errors.New("invalid private snapshot metadata")
	}
	for path, entry := range metadata.Manifest.Entries {
		if err = validateManifestEntry(path, entry); err != nil {
			return err
		}
	}
	actual, err := scanPrivateTree(source)
	if err != nil {
		return err
	}
	if !samePrivateTree(actual, metadata.Manifest.Entries) {
		return errors.New("private snapshot tree does not match manifest")
	}
	work := filepath.Join(base, "backups", "private", "restore-"+uuid.NewString())
	staging := filepath.Join(work, "staged")
	if err := copyPrivateRestoreTree(source, staging); err != nil {
		m.cleanupPrivate(base, "restore-prejournal", func() error { return os.RemoveAll(work) })
		return err
	}
	return m.replacePrivateWorkspaceLocked(instanceID, base, work, staging)
}

func (m *PrivateManager) replacePrivateWorkspaceLocked(instanceID, base, work, staging string) (err error) {
	preJournal := true
	defer func() {
		if preJournal {
			m.cleanupPrivate(base, "restore-prejournal", func() error { return os.RemoveAll(work) })
		}
	}()
	workspace := filepath.Join(base, "private")
	backup := filepath.Join(work, "old")
	hadOld := false
	if _, err = os.Stat(workspace); err == nil {
		hadOld = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	journal := privateRestoreJournal{Version: 1, InstanceID: instanceID, Stage: "prepared", HadOld: hadOld}
	journalPath := filepath.Join(work, "journal.json")
	if err := runPrivateRestoreFailureHook("journal"); err != nil {
		return err
	}
	if err := writeJSONAtomic(journalPath, journal); err != nil {
		return err
	}
	preJournal = false
	fail := func(cause error) error {
		return errors.Join(cause, m.rollbackPrivateRestore(work, base, journal))
	}
	journal.Stage = "old_moved"
	if err = writeJSONAtomic(journalPath, journal); err != nil {
		return fail(err)
	}
	if hadOld {
		if err = os.Rename(workspace, backup); err != nil {
			return fail(err)
		}
	}
	if err := os.Rename(staging, workspace); err != nil {
		return fail(err)
	}
	journal.Stage = "published"
	if err = writeJSONAtomic(journalPath, journal); err != nil {
		return fail(err)
	}
	journal.Stage = "committed"
	if err = writeJSONAtomic(journalPath, journal); err != nil {
		return fail(err)
	}
	m.cleanupPrivate(base, "restore-work", func() error { return os.RemoveAll(work) })
	return nil
}

func samePrivateTree(actual, expected map[string]manifestEntry) bool {
	if len(actual) != len(expected) {
		return false
	}
	for path, want := range expected {
		got, ok := actual[path]
		if !ok || got.Kind != want.Kind || got.Hash != want.Hash || got.Size != want.Size {
			return false
		}
	}
	return true
}

func (m *PrivateManager) recoverPrivateRestoreLocked(journalPath, base, instanceID string) error {
	work := filepath.Dir(journalPath)
	root := filepath.Join(base, "backups", "private")
	name := filepath.Base(work)
	if filepath.Dir(work) != root || !strings.HasPrefix(name, "restore-") {
		return errors.New("invalid private restore journal path")
	}
	if _, err := uuid.Parse(strings.TrimPrefix(name, "restore-")); err != nil {
		return errors.New("invalid private restore journal id")
	}
	if err := rejectSymlinkParents(root, journalPath); err != nil {
		return err
	}
	raw, err := os.ReadFile(journalPath)
	if err != nil {
		return err
	}
	var journal privateRestoreJournal
	if err = json.Unmarshal(raw, &journal); err != nil {
		return err
	}
	if journal.Version != 1 || journal.InstanceID != instanceID || filepath.Base(instanceID) != instanceID {
		return errors.New("invalid private restore journal identity")
	}
	if !map[string]bool{"prepared": true, "old_moved": true, "published": true, "committed": true}[journal.Stage] {
		return errors.New("invalid private restore journal stage")
	}
	for _, path := range []string{work, journalPath, filepath.Join(work, "old"), filepath.Join(work, "staged")} {
		if _, statErr := os.Lstat(path); statErr == nil {
			if err = rejectSymlinkParents(root, path); err != nil {
				return err
			}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
	}
	if info, statErr := os.Lstat(work); statErr != nil || !info.IsDir() {
		return errors.New("invalid private restore work directory")
	}
	if info, statErr := os.Lstat(journalPath); statErr != nil || !info.Mode().IsRegular() {
		return errors.New("invalid private restore journal file")
	}
	if info, statErr := os.Lstat(filepath.Join(work, "old")); statErr == nil && !info.IsDir() {
		return errors.New("invalid private restore backup")
	} else if journal.Stage == "published" && journal.HadOld && errors.Is(statErr, os.ErrNotExist) {
		return errors.New("private restore backup is missing")
	}
	if journal.Stage == "committed" {
		m.cleanupPrivate(base, "restore-work", func() error { return os.RemoveAll(work) })
		return nil
	}
	return m.rollbackPrivateRestore(work, base, journal)
}

func (m *PrivateManager) rollbackPrivateRestore(work, base string, journal privateRestoreJournal) error {
	workspace := filepath.Join(base, "private")
	old := filepath.Join(work, "old")
	var result error
	if journal.Stage == "old_moved" || journal.Stage == "published" {
		oldInfo, oldErr := os.Lstat(old)
		if oldErr != nil && !errors.Is(oldErr, os.ErrNotExist) {
			result = errors.Join(result, oldErr)
		} else if journal.HadOld && oldErr == nil && oldInfo.IsDir() {
			if err := os.RemoveAll(workspace); err != nil {
				result = errors.Join(result, err)
			} else {
				result = errors.Join(result, os.Rename(old, workspace))
			}
		} else if !journal.HadOld {
			result = errors.Join(result, os.RemoveAll(workspace))
		}
	}
	if result == nil {
		result = os.RemoveAll(work)
	}
	return result
}

func prunePrivateSnapshots(base string, keep int) error {
	root := filepath.Join(base, "backups", "private", "snapshots")
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	names := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	if len(names) <= keep {
		return nil
	}
	var result error
	for _, name := range names[keep:] {
		result = errors.Join(result, os.RemoveAll(filepath.Join(root, name)))
	}
	return result
}

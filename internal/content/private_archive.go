package content

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/not0721here/l4d2-control-panel/internal/disk"
	"github.com/not0721here/l4d2-control-panel/internal/safepath"
)

type PrivateArchiveLimits struct {
	MaxCompressedBytes  int64
	MaxExpandedBytes    uint64
	MaxFileBytes        uint64
	MaxFiles            int
	MaxCompressionRatio float64
}

var DefaultPrivateArchiveLimits = PrivateArchiveLimits{
	MaxCompressedBytes:  2 << 30,
	MaxExpandedBytes:    4 << 30,
	MaxFileBytes:        2 << 30,
	MaxFiles:            10_000,
	MaxCompressionRatio: 400,
}

var (
	ErrPrivateArchiveInvalid     = errors.New("invalid private ZIP archive")
	ErrPrivateArchivePath        = errors.New("invalid private ZIP path")
	ErrPrivateArchiveConflict    = errors.New("conflicting private ZIP paths")
	ErrPrivateArchiveUnsupported = errors.New("unsupported private ZIP entry")
	ErrPrivateArchiveTooLarge    = errors.New("private ZIP archive exceeds limits")
)

type privateArchiveNode struct {
	kind string
	size int64
}

type privateArchiveEntry struct {
	file *zip.File
	path string
	kind string
}

type privateArchivePlan struct {
	entries       []privateArchiveEntry
	nodes         map[string]privateArchiveNode
	expandedBytes uint64
}

func (m *PrivateManager) ExportZIP(ctx context.Context, instanceID string, output io.Writer) error {
	lock := m.instanceLock(instanceID)
	lock.Lock()
	defer lock.Unlock()
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
	root, err := m.privateRoot(instanceID)
	if err != nil {
		return err
	}
	if err = rejectSymlinkParents(m.root, root); err != nil {
		return err
	}
	entries, err := scanPrivateTree(root)
	if err != nil {
		return err
	}
	paths := make([]string, 0, len(entries))
	for name := range entries {
		paths = append(paths, name)
	}
	sort.Strings(paths)

	writer := zip.NewWriter(output)
	for _, name := range paths {
		if err = ctx.Err(); err != nil {
			_ = writer.Close()
			return err
		}
		entry := entries[name]
		header := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: entry.UpdatedAt}
		if entry.Kind == "directory" {
			header.Name += "/"
			header.Method = zip.Store
			header.SetMode(fs.ModeDir | 0750)
			if _, err = writer.CreateHeader(header); err != nil {
				_ = writer.Close()
				return err
			}
			continue
		}
		header.SetMode(0640)
		target, joinErr := safepath.Join(root, name)
		if joinErr != nil {
			_ = writer.Close()
			return joinErr
		}
		if err = rejectSymlinkParents(root, target); err != nil {
			_ = writer.Close()
			return err
		}
		source, openErr := os.Open(target)
		if openErr != nil {
			_ = writer.Close()
			return openErr
		}
		destination, createErr := writer.CreateHeader(header)
		if createErr == nil {
			_, createErr = io.Copy(destination, source)
		}
		closeErr := source.Close()
		if createErr != nil {
			_ = writer.Close()
			return createErr
		}
		if closeErr != nil {
			_ = writer.Close()
			return closeErr
		}
	}
	return writer.Close()
}

func (m *PrivateManager) ImportZIP(ctx context.Context, instanceID string, input io.Reader, limits PrivateArchiveLimits) error {
	if err := validatePrivateArchiveLimits(limits); err != nil {
		return err
	}
	lock := m.instanceLock(instanceID)
	lock.Lock()
	defer lock.Unlock()
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
	privateBackupRoot := filepath.Join(base, "backups", "private")
	if err := rejectSymlinkParents(m.root, privateBackupRoot); err != nil {
		return err
	}
	work := filepath.Join(privateBackupRoot, "restore-"+uuid.NewString())
	staging := filepath.Join(work, "staged")
	if err := os.MkdirAll(work, 0750); err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			m.cleanupPrivate(base, "restore-prejournal", func() error { return os.RemoveAll(work) })
		}
	}()

	archivePath := filepath.Join(work, "archive.zip")
	written, err := writePrivateArchiveInput(archivePath, input, limits.MaxCompressedBytes)
	if err != nil {
		return err
	}
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("%w: malformed ZIP container", ErrPrivateArchiveInvalid)
	}
	defer reader.Close()
	plan, err := inspectPrivateArchive(reader.File, limits)
	if err != nil {
		return err
	}
	available, err := disk.Available(work)
	if err != nil {
		return err
	}
	if plan.expandedBytes > available {
		return fmt.Errorf("%w: insufficient disk space for expanded data", ErrPrivateArchiveTooLarge)
	}
	if err = extractPrivateArchive(ctx, staging, plan); err != nil {
		return err
	}
	if err = syncDirectory(staging); err != nil {
		return err
	}
	actual, err := scanPrivateTree(staging)
	if err != nil {
		return err
	}
	if !samePrivateArchiveTree(actual, plan.nodes) {
		return fmt.Errorf("%w: extracted tree does not match archive", ErrPrivateArchiveInvalid)
	}
	if written == 0 && len(plan.entries) != 0 {
		return fmt.Errorf("%w: empty archive input", ErrPrivateArchiveInvalid)
	}
	if err = reader.Close(); err != nil {
		return err
	}
	cleanup = false
	return m.replacePrivateWorkspaceLocked(instanceID, base, work, staging)
}

func validatePrivateArchiveLimits(limits PrivateArchiveLimits) error {
	if limits.MaxCompressedBytes <= 0 || limits.MaxCompressedBytes == math.MaxInt64 || limits.MaxExpandedBytes == 0 || limits.MaxFileBytes == 0 || limits.MaxFiles <= 0 || limits.MaxCompressionRatio <= 0 {
		return errors.New("invalid private archive limits")
	}
	return nil
}

func writePrivateArchiveInput(target string, input io.Reader, maximum int64) (int64, error) {
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		return 0, err
	}
	written, copyErr := io.Copy(file, io.LimitReader(input, maximum+1))
	if copyErr == nil {
		copyErr = file.Sync()
	}
	closeErr := file.Close()
	if written > maximum || copyErr != nil && written >= maximum {
		return written, fmt.Errorf("%w: compressed data exceeds limit", ErrPrivateArchiveTooLarge)
	}
	if copyErr != nil {
		return written, copyErr
	}
	if closeErr != nil {
		return written, closeErr
	}
	return written, nil
}

func inspectPrivateArchive(files []*zip.File, limits PrivateArchiveLimits) (privateArchivePlan, error) {
	if len(files) > limits.MaxFiles {
		return privateArchivePlan{}, fmt.Errorf("%w: entry count exceeds limit", ErrPrivateArchiveTooLarge)
	}
	plan := privateArchivePlan{nodes: make(map[string]privateArchiveNode)}
	explicit := make(map[string]bool)
	folded := make(map[string]string)
	register := func(name, kind string, size int64, isExplicit bool) error {
		key := strings.ToLower(name)
		if existing, ok := folded[key]; ok && existing != name {
			return fmt.Errorf("%w: case-folded path collision", ErrPrivateArchiveConflict)
		}
		folded[key] = name
		if isExplicit {
			if explicit[name] {
				return fmt.Errorf("%w: duplicate path", ErrPrivateArchiveConflict)
			}
			explicit[name] = true
		}
		if existing, ok := plan.nodes[name]; ok {
			if existing.kind != kind {
				return fmt.Errorf("%w: file and directory share a path", ErrPrivateArchiveConflict)
			}
			return nil
		}
		plan.nodes[name] = privateArchiveNode{kind: kind, size: size}
		return nil
	}

	for _, file := range files {
		if file.Flags&1 != 0 {
			return privateArchivePlan{}, fmt.Errorf("%w: encrypted entries are forbidden", ErrPrivateArchiveUnsupported)
		}
		if file.NonUTF8 || !utf8.ValidString(file.Name) {
			return privateArchivePlan{}, fmt.Errorf("%w: non-UTF-8 entry names are forbidden", ErrPrivateArchiveUnsupported)
		}
		mode := file.Mode()
		if mode&os.ModeSymlink != 0 || mode.Type() != 0 && !mode.IsDir() {
			return privateArchivePlan{}, fmt.Errorf("%w: special entries are forbidden", ErrPrivateArchiveUnsupported)
		}
		isDirectory := file.FileInfo().IsDir() || strings.HasSuffix(file.Name, "/")
		kind := "file"
		if isDirectory {
			kind = "directory"
		}
		name, err := validatePrivateArchivePath(file.Name, isDirectory)
		if err != nil {
			return privateArchivePlan{}, err
		}
		if !isDirectory {
			if file.Method != zip.Store && file.Method != zip.Deflate {
				return privateArchivePlan{}, fmt.Errorf("%w: unsupported compression method", ErrPrivateArchiveUnsupported)
			}
			if file.UncompressedSize64 > limits.MaxFileBytes {
				return privateArchivePlan{}, fmt.Errorf("%w: single file exceeds limit", ErrPrivateArchiveTooLarge)
			}
			compressed := file.CompressedSize64
			if compressed == 0 {
				compressed = 1
			}
			if float64(file.UncompressedSize64)/float64(compressed) > limits.MaxCompressionRatio {
				return privateArchivePlan{}, fmt.Errorf("%w: compression ratio exceeds limit", ErrPrivateArchiveTooLarge)
			}
			if file.UncompressedSize64 > limits.MaxExpandedBytes-plan.expandedBytes {
				return privateArchivePlan{}, fmt.Errorf("%w: expanded data exceeds limit", ErrPrivateArchiveTooLarge)
			}
			plan.expandedBytes += file.UncompressedSize64
		}
		for parent := pathpkg.Dir(name); parent != "."; parent = pathpkg.Dir(parent) {
			if err = register(parent, "directory", 0, false); err != nil {
				return privateArchivePlan{}, err
			}
		}
		if err = register(name, kind, int64(file.UncompressedSize64), true); err != nil {
			return privateArchivePlan{}, err
		}
		plan.entries = append(plan.entries, privateArchiveEntry{file: file, path: name, kind: kind})
	}
	return plan, nil
}

func validatePrivateArchivePath(raw string, isDirectory bool) (string, error) {
	if raw == "" || strings.ContainsRune(raw, 0) || strings.Contains(raw, "\\") || strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("%w: unsafe path syntax", ErrPrivateArchivePath)
	}
	name := raw
	if isDirectory {
		name = strings.TrimSuffix(name, "/")
	}
	if name == "" || name == "." || pathpkg.IsAbs(name) || pathpkg.Clean(name) != name || name == ".." || strings.HasPrefix(name, "../") {
		return "", fmt.Errorf("%w: unsafe path syntax", ErrPrivateArchivePath)
	}
	components := strings.Split(name, "/")
	if len(components[0]) >= 2 && components[0][1] == ':' {
		return "", fmt.Errorf("%w: volume paths are forbidden", ErrPrivateArchivePath)
	}
	for _, component := range components {
		if invalidPrivateArchiveComponent(component) {
			return "", fmt.Errorf("%w: reserved path component", ErrPrivateArchivePath)
		}
	}
	return name, nil
}

func invalidPrivateArchiveComponent(component string) bool {
	if component == "" || strings.ContainsAny(component, `<>:"|?*`) || strings.HasSuffix(component, ".") || strings.HasSuffix(component, " ") {
		return true
	}
	base := strings.ToUpper(strings.SplitN(component, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" {
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9'
}

func extractPrivateArchive(ctx context.Context, staging string, plan privateArchivePlan) error {
	if err := os.MkdirAll(staging, 0750); err != nil {
		return err
	}
	for _, entry := range plan.entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		target, err := safepath.Join(staging, entry.path)
		if err != nil {
			return fmt.Errorf("%w: unsafe extraction target", ErrPrivateArchivePath)
		}
		if err = rejectSymlinkParents(staging, target); err != nil {
			return fmt.Errorf("%w: unsafe extraction target", ErrPrivateArchivePath)
		}
		if entry.kind == "directory" {
			if err = os.MkdirAll(target, 0750); err != nil {
				return err
			}
			continue
		}
		if err = os.MkdirAll(filepath.Dir(target), 0750); err != nil {
			return err
		}
		source, err := entry.file.Open()
		if err != nil {
			return fmt.Errorf("%w: cannot open archive entry", ErrPrivateArchiveInvalid)
		}
		destination, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
		if err != nil {
			source.Close()
			return err
		}
		written, copyErr := io.Copy(destination, io.LimitReader(source, int64(entry.file.UncompressedSize64)+1))
		if copyErr == nil {
			copyErr = destination.Sync()
		}
		destinationCloseErr := destination.Close()
		sourceCloseErr := source.Close()
		if copyErr != nil || sourceCloseErr != nil || written != int64(entry.file.UncompressedSize64) {
			return fmt.Errorf("%w: entry data failed validation", ErrPrivateArchiveInvalid)
		}
		if destinationCloseErr != nil {
			return destinationCloseErr
		}
	}
	return nil
}

func samePrivateArchiveTree(actual map[string]manifestEntry, expected map[string]privateArchiveNode) bool {
	if len(actual) != len(expected) {
		return false
	}
	for name, want := range expected {
		got, ok := actual[name]
		if !ok || got.Kind != want.kind || got.Size != want.size {
			return false
		}
	}
	return true
}

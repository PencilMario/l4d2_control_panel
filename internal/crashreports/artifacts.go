package crashreports

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type ArtifactKind string

const (
	ArtifactKindSymbol ArtifactKind = "symbol"
	ArtifactKindBinary ArtifactKind = "binary"
)

type Artifact struct {
	ID              string       `json:"id"`
	Kind            ArtifactKind `json:"kind"`
	SHA256          string       `json:"sha256"`
	Size            int64        `json:"size"`
	InstanceID      string       `json:"instance_id,omitempty"`
	DebugIdentifier string       `json:"debug_identifier,omitempty"`
	CodeIdentifier  string       `json:"code_identifier,omitempty"`
	DebugFile       string       `json:"debug_file,omitempty"`
	Platform        string       `json:"platform,omitempty"`
	Architecture    string       `json:"architecture,omitempty"`
	Basename        string       `json:"basename,omitempty"`
	Builtin         bool         `json:"builtin,omitempty"`
	Generated       bool         `json:"generated,omitempty"`
	ReceivedAt      string       `json:"received_at"`
}

func (m *Manager) SaveBinary(ctx context.Context, input BinaryInput) (Artifact, error) {
	if input.CodeFile == nil || !validIdentifier(input.DebugIdentifier) || !validOptionalIdentifier(input.CodeIdentifier) {
		return Artifact{}, ErrInvalidBinary
	}
	input.Platform = strings.ToLower(input.Platform)
	input.Architecture = strings.ToLower(input.Architecture)
	basename, err := safeBasename(input.CodeFileName)
	if err != nil {
		return Artifact{}, err
	}
	if err := ctx.Err(); err != nil {
		return Artifact{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.validatePendingBinding(input.PresubmitToken, input.InstanceID, input.ServerID); err != nil {
		return Artifact{}, err
	}
	temporary, size, digest, err := m.writeIncoming(ctx, "binary", input.CodeFile, MaxBinaryBytes, ErrBinaryTooLarge)
	if err != nil {
		return Artifact{}, err
	}
	defer os.Remove(temporary)
	if size == 0 {
		return Artifact{}, ErrInvalidBinary
	}
	id := fmt.Sprintf("%x", digest[:])
	artifact := Artifact{
		ID: id, Kind: ArtifactKindBinary, SHA256: id, Size: size,
		InstanceID:      input.InstanceID,
		DebugIdentifier: input.DebugIdentifier, CodeIdentifier: input.CodeIdentifier,
		DebugFile: basename, Platform: input.Platform, Architecture: input.Architecture,
		Basename: basename, ReceivedAt: m.currentTime().Format(timeFormat),
	}
	target := filepath.Join(m.root, "binaries", id+".bin")
	if info, statErr := os.Lstat(target); os.IsNotExist(statErr) {
		if err := os.Rename(temporary, target); err != nil {
			return Artifact{}, fmt.Errorf("store binary artifact: %w", err)
		}
		if err := os.Chmod(target, 0o600); err != nil {
			return Artifact{}, err
		}
	} else if statErr == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
		return Artifact{}, ErrInvalidBinary
	} else if statErr != nil {
		return Artifact{}, statErr
	}
	if err := m.writeArtifactManifest(artifact); err != nil {
		return Artifact{}, err
	}
	if err := m.backfillArtifactReferences(ctx, artifact); err != nil {
		return Artifact{}, err
	}
	return artifact, nil
}

func (m *Manager) backfillArtifactReferences(ctx context.Context, artifact Artifact) error {
	entries, err := os.ReadDir(filepath.Join(m.root, "reports"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !identifierPattern.MatchString(entry.Name()) {
			continue
		}
		reportPath := filepath.Join(m.root, "reports", entry.Name())
		info, statErr := os.Lstat(reportPath)
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			continue
		}
		report, readErr := readReport(reportPath)
		if errors.Is(readErr, ErrNotFound) {
			continue
		}
		if readErr != nil {
			return readErr
		}
		changed := false
		for index := range report.Modules {
			if !artifactMatchesModule(report.Modules[index], artifact) {
				continue
			}
			if artifact.Kind == ArtifactKindSymbol && report.Modules[index].SymbolArtifact != artifact.ID {
				report.Modules[index].SymbolArtifact = artifact.ID
				changed = true
			}
			if artifact.Kind == ArtifactKindBinary && report.Modules[index].BinaryArtifact != artifact.ID {
				report.Modules[index].BinaryArtifact = artifact.ID
				changed = true
			}
		}
		if report.ParsedSignature != nil {
			for index := range report.ParsedSignature.Modules {
				if !artifactMatchesModule(report.ParsedSignature.Modules[index], artifact) {
					continue
				}
				if artifact.Kind == ArtifactKindSymbol && report.ParsedSignature.Modules[index].SymbolArtifact != artifact.ID {
					report.ParsedSignature.Modules[index].SymbolArtifact = artifact.ID
					changed = true
				}
				if artifact.Kind == ArtifactKindBinary && report.ParsedSignature.Modules[index].BinaryArtifact != artifact.ID {
					report.ParsedSignature.Modules[index].BinaryArtifact = artifact.ID
					changed = true
				}
			}
		}
		if changed {
			report.UpdatedAt = m.currentTime()
			if err := writeJSONAtomic(filepath.Join(reportPath, "report.json"), report, 0o600); err != nil {
				return err
			}
		}
	}
	return nil
}

func artifactMatchesModule(module Module, artifact Artifact) bool {
	if module.DebugIdentifier == "" || module.DebugIdentifier != artifact.DebugIdentifier {
		return false
	}
	if module.CodeIdentifier != "" && artifact.CodeIdentifier != "" && module.CodeIdentifier != artifact.CodeIdentifier {
		return false
	}
	if module.Platform != "" && artifact.Platform != "" && !strings.EqualFold(module.Platform, artifact.Platform) {
		return false
	}
	if module.Architecture != "" && artifact.Architecture != "" && !strings.EqualFold(module.Architecture, artifact.Architecture) {
		return false
	}
	if module.DebugFile != "" && artifact.DebugFile != "" {
		moduleFile, moduleErr := safeBasename(module.DebugFile)
		artifactFile, artifactErr := safeBasename(artifact.DebugFile)
		if moduleErr != nil || artifactErr != nil || !strings.EqualFold(moduleFile, artifactFile) {
			return false
		}
	}
	return true
}

func (m *Manager) OpenArtifact(ctx context.Context, kind ArtifactKind, id string) (*os.File, Artifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, Artifact{}, err
	}
	if (kind != ArtifactKindSymbol && kind != ArtifactKindBinary) || !identifierPattern.MatchString(id) {
		return nil, Artifact{}, ErrNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	manifestPath := m.artifactManifestPath(kind, id)
	manifestInfo, err := os.Lstat(manifestPath)
	if err != nil || manifestInfo.Mode()&os.ModeSymlink != 0 || !manifestInfo.Mode().IsRegular() {
		return nil, Artifact{}, ErrNotFound
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, Artifact{}, err
	}
	var artifact Artifact
	if err := json.Unmarshal(raw, &artifact); err != nil || artifact.ID != id || artifact.Kind != kind {
		return nil, Artifact{}, ErrNotFound
	}
	path := m.artifactPath(kind, id, artifact.Builtin)
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, Artifact{}, ErrNotFound
	}
	if artifact.SHA256 != id {
		return nil, Artifact{}, ErrNotFound
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, Artifact{}, err
	}
	return file, artifact, nil
}

func (m *Manager) writeArtifactManifest(artifact Artifact) error {
	if artifact.ID == "" || (artifact.Kind != ArtifactKindSymbol && artifact.Kind != ArtifactKindBinary) {
		return ErrInvalidBinary
	}
	return writeJSONAtomic(m.artifactManifestPath(artifact.Kind, artifact.ID), artifact, 0o600)
}

func (m *Manager) artifactManifestPath(kind ArtifactKind, id string) string {
	directory := "binary-manifests"
	if kind == ArtifactKindSymbol {
		directory = "symbol-manifests"
	}
	return filepath.Join(m.root, directory, id+".json")
}

func (m *Manager) artifactPath(kind ArtifactKind, id string, builtin bool) string {
	directory, suffix := "binaries", ".bin"
	if kind == ArtifactKindSymbol {
		directory, suffix = filepath.Join("symbols", "uploaded"), ".sym"
		if builtin {
			directory = filepath.Join("symbols", "builtin")
		}
	}
	return filepath.Join(m.root, directory, id+suffix)
}

func (m *Manager) hasMatchingArtifact(module Module, kind ArtifactKind) (Artifact, bool) {
	if module.DebugIdentifier == "" {
		return Artifact{}, false
	}
	directory := filepath.Join(m.root, "binary-manifests")
	if kind == ArtifactKindSymbol {
		directory = filepath.Join(m.root, "symbol-manifests")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return Artifact{}, false
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		manifestPath := filepath.Join(directory, entry.Name())
		manifestInfo, statErr := os.Lstat(manifestPath)
		if statErr != nil || manifestInfo.Mode()&os.ModeSymlink != 0 || !manifestInfo.Mode().IsRegular() {
			continue
		}
		raw, readErr := os.ReadFile(manifestPath)
		if readErr != nil {
			continue
		}
		var artifact Artifact
		if json.Unmarshal(raw, &artifact) != nil || artifact.Kind != kind || artifact.DebugIdentifier != module.DebugIdentifier {
			continue
		}
		if artifact.ID == "" || artifact.SHA256 != artifact.ID || !identifierPattern.MatchString(artifact.ID) {
			continue
		}
		if module.CodeIdentifier != "" && artifact.CodeIdentifier != "" && module.CodeIdentifier != artifact.CodeIdentifier {
			continue
		}
		if artifact.Platform != "" && module.Platform != "" && strings.ToLower(artifact.Platform) != strings.ToLower(module.Platform) {
			continue
		}
		if artifact.Architecture != "" && module.Architecture != "" && strings.ToLower(artifact.Architecture) != strings.ToLower(module.Architecture) {
			continue
		}
		objectInfo, statErr := os.Lstat(m.artifactPath(kind, artifact.ID, artifact.Builtin))
		if statErr != nil || objectInfo.Mode()&os.ModeSymlink != 0 || !objectInfo.Mode().IsRegular() {
			continue
		}
		return artifact, true
	}
	return Artifact{}, false
}

func (m *Manager) ensureSymbolLookup(artifact Artifact, objectPath string) error {
	lookupPath, found, err := m.symbolLookupPath(artifact)
	if err != nil || !found {
		return err
	}
	if err := ensureSymbolDirectory(filepath.Join(m.root, "symbols"), filepath.Dir(lookupPath)); err != nil {
		return err
	}
	raw, err := os.ReadFile(objectPath)
	if err != nil {
		return err
	}
	if info, statErr := os.Lstat(lookupPath); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return ErrInvalidSymbol
		}
		existing, readErr := os.ReadFile(lookupPath)
		if readErr != nil {
			return readErr
		}
		if bytes.Equal(existing, raw) {
			return nil
		}
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	return writeBytesAtomic(lookupPath, raw, 0o600)
}

func (m *Manager) removeSymbolLookup(artifact Artifact) (int64, error) {
	lookupPath, found, err := m.symbolLookupPath(artifact)
	if err != nil || !found {
		return 0, err
	}
	info, err := os.Lstat(lookupPath)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return 0, nil
	}
	if err := os.Remove(lookupPath); err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (m *Manager) symbolLookupPath(artifact Artifact) (string, bool, error) {
	if artifact.Kind != ArtifactKindSymbol || artifact.DebugFile == "" {
		return "", false, nil
	}
	debugFile, err := safeBasename(artifact.DebugFile)
	if err != nil || !validSymbolPathComponent(debugFile) {
		return "", false, ErrInvalidSymbol
	}
	if !validSymbolPathComponent(artifact.DebugIdentifier) {
		return "", false, ErrInvalidSymbol
	}
	return filepath.Join(m.root, "symbols", debugFile, artifact.DebugIdentifier, debugFile+".sym"), true, nil
}

func validSymbolPathComponent(value string) bool {
	return value != "" && value != "." && value != ".." && validIdentifier(value) &&
		!strings.ContainsAny(value, `/\<>:"|?*`)
}

func ensureSymbolDirectory(root, target string) error {
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return ErrInvalidSymbol
	}
	if err := ensureRegularDirectory(root); err != nil {
		return err
	}
	current := root
	if relative == "." {
		return nil
	}
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) {
			if err := os.Mkdir(current, 0o750); err != nil {
				return err
			}
			continue
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return ErrInvalidSymbol
		}
	}
	return nil
}

func ensureRegularDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return ErrInvalidSymbol
	}
	return nil
}

func validIdentifier(value string) bool {
	return value != "" && len(value) <= 256 && utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\r\n") && !strings.Contains(value, "|")
}

func validOptionalIdentifier(value string) bool {
	return value == "" || validIdentifier(value)
}

func parseBreakpadModule(raw []byte) (platform, architecture, debugIdentifier, debugFile string) {
	line := raw
	if index := bytes.IndexByte(line, '\n'); index >= 0 {
		line = line[:index]
	}
	fields := strings.Fields(string(bytes.TrimSuffix(line, []byte{'\r'})))
	if len(fields) < 4 || fields[0] != "MODULE" {
		return "", "", "", ""
	}
	debugFile = ""
	if len(fields) >= 5 {
		debugFile = fields[4]
	}
	return strings.ToLower(fields[1]), strings.ToLower(fields[2]), fields[3], debugFile
}

func safeBasename(value string) (string, error) {
	value = strings.ReplaceAll(value, "\\", "/")
	base := path.Base(value)
	if value == "" || base == "." || base == "/" || base == ".." || strings.ContainsAny(base, "\x00\r\n") || !utf8.ValidString(base) {
		return "", errors.New("invalid binary filename")
	}
	return base, nil
}

const timeFormat = "2006-01-02T15:04:05.999999999Z07:00"

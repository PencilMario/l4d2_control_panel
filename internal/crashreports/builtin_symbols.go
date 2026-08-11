package crashreports

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"os"
	"path"
)

// The embedded files are limited to the AlliedModders SourceMod and Metamod
// symbols needed by the supported L4D2 runtime.
//
//go:embed builtin_symbols/*.sym
var builtinSymbolFS embed.FS

func (m *Manager) installBuiltinSymbols() error {
	entries, err := builtinSymbolFS.ReadDir("builtin_symbols")
	if err != nil {
		return fmt.Errorf("read builtin symbols: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		raw, err := builtinSymbolFS.ReadFile(path.Join("builtin_symbols", entry.Name()))
		if err != nil {
			return fmt.Errorf("read builtin symbol %s: %w", entry.Name(), err)
		}
		platform, architecture, debugIdentifier, debugFile := parseBreakpadModule(raw)
		if platform == "" || architecture == "" || debugIdentifier == "" || debugFile == "" {
			return fmt.Errorf("builtin symbol %s has an invalid MODULE record", entry.Name())
		}
		id := symbolHash(raw)
		artifact := Artifact{
			ID: id, Kind: ArtifactKindSymbol, SHA256: id, Size: int64(len(raw)),
			DebugIdentifier: debugIdentifier, DebugFile: debugFile,
			Platform: platform, Architecture: architecture, Builtin: true,
			ReceivedAt: m.currentTime().Format(timeFormat),
		}
		target := m.artifactPath(ArtifactKindSymbol, id, true)
		if info, statErr := os.Lstat(target); statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return fmt.Errorf("builtin symbol object is not a regular file: %s", entry.Name())
			}
			existing, readErr := os.ReadFile(target)
			if readErr != nil || symbolHash(existing) != id {
				return fmt.Errorf("builtin symbol object hash mismatch: %s", entry.Name())
			}
		} else if os.IsNotExist(statErr) {
			if err := writeBytesAtomic(target, raw, 0o600); err != nil {
				return fmt.Errorf("install builtin symbol %s: %w", entry.Name(), err)
			}
		} else {
			return statErr
		}
		if err := m.ensureSymbolLookup(artifact, target); err != nil {
			return fmt.Errorf("prepare builtin symbol lookup %s: %w", entry.Name(), err)
		}
		if err := m.writeArtifactManifest(artifact); err != nil {
			return fmt.Errorf("write builtin symbol manifest %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func symbolHash(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

package updates

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	archivecheck "github.com/not0721here/l4d2-control-panel/internal/archive"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/safepath"
	"io"
	"os"
	"path/filepath"
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

func New(root string) *Pipeline { return &Pipeline{root: root} }
func (p *Pipeline) Apply(ctx context.Context, instanceID, archivePath, version string, mode Mode) (resultErr error) {
	inspected, err := archivecheck.InspectZip(archivePath, archivecheck.Limits{MaxFiles: 20000, MaxBytes: 8 << 30})
	if err != nil {
		return err
	}
	if mode == Hot && !inspected.HotCompatible {
		return errors.New("package is not hot-update compatible")
	}
	base := filepath.Join(p.root, "instances", instanceID)
	game := filepath.Join(base, "game", "left4dead2")
	work := filepath.Join(base, "backups", "update-"+uuid.NewString())
	staging := filepath.Join(work, "staging")
	backup := filepath.Join(work, "replaced")
	if err := os.MkdirAll(staging, 0750); err != nil {
		return err
	}
	defer func() {
		if resultErr == nil {
			_ = os.RemoveAll(work)
		}
	}()
	newManifest := manifest{Version: version, Files: map[string]string{}}
	if err := extract(archivePath, staging, newManifest.Files); err != nil {
		return err
	}
	old := readManifest(filepath.Join(base, "package-manifest.json"))
	affected := map[string]bool{}
	for path := range newManifest.Files {
		affected[path] = true
	}
	if mode == Full {
		for path := range old.Files {
			affected[path] = true
		}
	}
	existed := map[string]bool{}
	for path := range affected {
		target, err := safepath.Join(game, path)
		if err != nil {
			return err
		}
		if info, statErr := os.Stat(target); statErr == nil && !info.IsDir() {
			existed[path] = true
			destination, _ := safepath.Join(backup, path)
			if err := copyFile(target, destination); err != nil {
				return err
			}
		}
	}
	rollback := func() {
		for path := range affected {
			target, _ := safepath.Join(game, path)
			if existed[path] {
				source, _ := safepath.Join(backup, path)
				_ = copyFile(source, target)
			} else {
				_ = os.Remove(target)
			}
		}
	}
	if mode == Full {
		for oldPath := range old.Files {
			if _, keep := newManifest.Files[oldPath]; !keep {
				target, _ := safepath.Join(game, oldPath)
				_ = os.Remove(target)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		rollback()
		return err
	}
	if err := content.ApplyTree(staging, game); err != nil {
		rollback()
		return err
	}
	if p.AfterDeploy != nil {
		if err := p.AfterDeploy(); err != nil {
			rollback()
			return err
		}
	}
	private := content.NewPrivateManager(p.root, 1<<20)
	if err := private.Apply(ctx, instanceID); err != nil {
		rollback()
		return err
	}
	if err := writeManifest(filepath.Join(base, "package-manifest.json"), newManifest); err != nil {
		rollback()
		return err
	}
	return nil
}
func extract(path, destination string, hashes map[string]string) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		target, err := safepath.Join(destination, entry.Name)
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
		hashes[filepath.ToSlash(entry.Name)] = hex.EncodeToString(digest.Sum(nil))
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
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temporary, target)
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
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, raw, 0640); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

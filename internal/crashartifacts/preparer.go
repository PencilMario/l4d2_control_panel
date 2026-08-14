package crashartifacts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/not0721here/l4d2-control-panel/internal/crashreports"
	"github.com/not0721here/l4d2-control-panel/internal/crashsymbols"
	"github.com/not0721here/l4d2-control-panel/internal/docker"
	"github.com/not0721here/l4d2-control-panel/internal/systemlibs"
)

type ContainerSource interface {
	ListManaged(context.Context) ([]docker.Container, error)
	GetArchive(context.Context, string, string) (io.ReadCloser, error)
}

type ArtifactStore interface {
	SaveBinary(context.Context, crashreports.BinaryInput) (crashreports.Artifact, error)
	SaveGeneratedSymbol(context.Context, string, io.Reader) error
}

type SymbolGenerator interface {
	Generate(context.Context, string) (crashsymbols.Symbol, error)
}

type Preparer struct {
	Containers ContainerSource
	Store      ArtifactStore
	Generator  SymbolGenerator
	TempRoot   string
}

func (p *Preparer) Prepare(ctx context.Context, report crashreports.Report) error {
	if p == nil || p.Containers == nil || p.Store == nil || p.Generator == nil {
		return errors.New("system library artifact preparation is not configured")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if report.ContainerID == "" {
		return errors.New("crash report has no source game container")
	}
	container, err := p.sourceContainer(ctx, report)
	if err != nil {
		return err
	}
	modules := report.Modules
	if len(modules) == 0 && report.ParsedSignature != nil {
		modules = report.ParsedSignature.Modules
	}
	var failures []error
	prepared := 0
	for _, module := range modules {
		if err := ctx.Err(); err != nil {
			return err
		}
		if module.BinaryArtifact != "" || !strings.EqualFold(module.Platform, "linux") || module.DebugIdentifier == "" {
			continue
		}
		candidates := systemlibs.CandidatePaths(module.DebugFile, module.Architecture)
		if len(candidates) == 0 {
			continue
		}
		if err := p.prepareModule(ctx, report, container.ID, module, candidates); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", module.DebugFile, err))
			continue
		}
		prepared++
	}
	if len(failures) > 0 && prepared == 0 {
		return errors.Join(failures...)
	}
	return nil
}

func (p *Preparer) sourceContainer(ctx context.Context, report crashreports.Report) (docker.Container, error) {
	containers, err := p.Containers.ListManaged(ctx)
	if err != nil {
		return docker.Container{}, err
	}
	for _, container := range containers {
		if container.ID == report.ContainerID && container.InstanceID() == report.InstanceID && container.Role() == "game" {
			return container, nil
		}
	}
	return docker.Container{}, errors.New("source game container is no longer managed")
}

func (p *Preparer) prepareModule(ctx context.Context, report crashreports.Report, containerID string, module crashreports.Module, candidates []string) error {
	if strings.TrimSpace(p.TempRoot) == "" {
		return errors.New("system library temporary root is required")
	}
	if err := os.MkdirAll(p.TempRoot, 0o750); err != nil {
		return err
	}
	var failures []error
	for _, candidate := range candidates {
		resolved := candidate
		var entry archiveEntry
		for depth := 0; depth < 4; depth++ {
			archive, err := p.Containers.GetArchive(ctx, containerID, resolved)
			if err != nil {
				failures = append(failures, fmt.Errorf("read %s: %w", resolved, err))
				entry = archiveEntry{}
				break
			}
			entry, err = readArchiveEntry(ctx, archive, filepath.Base(resolved), crashreports.MaxBinaryBytes)
			closeErr := archive.Close()
			if joined := errors.Join(err, closeErr); joined != nil {
				failures = append(failures, fmt.Errorf("read %s: %w", resolved, joined))
				entry = archiveEntry{}
				break
			}
			if entry.Regular {
				break
			}
			link := strings.ReplaceAll(entry.Linkname, "\\", "/")
			if link == "" || path.IsAbs(link) {
				failures = append(failures, fmt.Errorf("resolve %s: unsafe symlink target", resolved))
				entry = archiveEntry{}
				break
			}
			resolved = path.Clean(path.Join(path.Dir(resolved), link))
			if !systemlibs.IsAllowedContainerPath(resolved) {
				failures = append(failures, fmt.Errorf("resolve %s: symlink target is not an allowed system library", resolved))
				entry = archiveEntry{}
				break
			}
		}
		if !entry.Regular {
			if len(failures) == 0 || !strings.Contains(failures[len(failures)-1].Error(), resolved) {
				failures = append(failures, fmt.Errorf("read %s: symlink resolution exceeded limit", resolved))
			}
			continue
		}
		temporaryDirectory, err := os.MkdirTemp(p.TempRoot, ".system-library-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(temporaryDirectory)
		temporaryPath := filepath.Join(temporaryDirectory, filepath.Base(candidate))
		temporary, err := os.OpenFile(temporaryPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(temporary, bytes.NewReader(entry.Content))
		syncErr := temporary.Sync()
		closeFileErr := temporary.Close()
		if joined := errors.Join(copyErr, syncErr, closeFileErr); joined != nil {
			_ = os.Remove(temporaryPath)
			failures = append(failures, fmt.Errorf("read %s: %w", resolved, joined))
			continue
		}

		symbol, generateErr := p.Generator.Generate(ctx, temporaryPath)
		if generateErr != nil {
			_ = os.Remove(temporaryPath)
			failures = append(failures, fmt.Errorf("symbolize %s: %w", candidate, generateErr))
			continue
		}
		if err := verifySymbol(module, symbol); err != nil {
			_ = os.Remove(temporaryPath)
			failures = append(failures, fmt.Errorf("verify %s: %w", candidate, err))
			continue
		}
		binaryFile, err := os.Open(temporaryPath)
		if err != nil {
			_ = os.Remove(temporaryPath)
			return err
		}
		_, binaryErr := p.Store.SaveBinary(ctx, crashreports.BinaryInput{
			InstanceID:      report.InstanceID,
			Platform:        module.Platform,
			Architecture:    module.Architecture,
			DebugIdentifier: module.DebugIdentifier,
			CodeIdentifier:  module.CodeIdentifier,
			CodeFileName:    module.DebugFile,
			CodeFile:        binaryFile,
		})
		closeBinaryErr := binaryFile.Close()
		_ = os.Remove(temporaryPath)
		if joined := errors.Join(binaryErr, closeBinaryErr); joined != nil {
			return joined
		}
		if err := p.Store.SaveGeneratedSymbol(ctx, report.InstanceID, bytes.NewReader(symbol.Content)); err != nil {
			return err
		}
		return nil
	}
	if len(failures) == 0 {
		return errors.New("system library was not found in the source container")
	}
	return errors.Join(failures...)
}

func verifySymbol(module crashreports.Module, symbol crashsymbols.Symbol) error {
	if symbol.DebugIdentifier != module.DebugIdentifier {
		return fmt.Errorf("debug identifier mismatch: got %q want %q", symbol.DebugIdentifier, module.DebugIdentifier)
	}
	if module.Platform != "" && !strings.EqualFold(symbol.Platform, module.Platform) {
		return fmt.Errorf("platform mismatch: got %q want %q", symbol.Platform, module.Platform)
	}
	if module.Architecture != "" && !strings.EqualFold(symbol.Architecture, module.Architecture) {
		return fmt.Errorf("architecture mismatch: got %q want %q", symbol.Architecture, module.Architecture)
	}
	if module.DebugFile != "" && !strings.EqualFold(filepath.Base(symbol.DebugFile), filepath.Base(module.DebugFile)) {
		return fmt.Errorf("debug file mismatch: got %q want %q", symbol.DebugFile, module.DebugFile)
	}
	if len(symbol.Content) == 0 {
		return errors.New("symbol output is empty")
	}
	return nil
}

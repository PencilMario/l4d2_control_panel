package crashsymbols

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultGenerationTimeout = 2 * time.Minute
	defaultMaxOutputBytes    = 32 << 20
	defaultScanInterval      = 6 * time.Hour
	maxFailureRecords        = 64
)

var (
	ErrInvalidConfig        = errors.New("invalid crash symbol generator configuration")
	ErrInvalidSymbolOutput  = errors.New("invalid Breakpad symbol output")
	ErrNoSymbolRecords      = errors.New("Breakpad symbol output has no symbol records")
	ErrZeroDebugIdentifier  = errors.New("Breakpad symbol output has an empty debug identifier")
	ErrSymbolOutputTooLarge = errors.New("Breakpad symbol output exceeds the size limit")
)

type Runner interface {
	Run(context.Context, string) ([]byte, error)
}

type RunnerFunc func(context.Context, string) ([]byte, error)

func (f RunnerFunc) Run(ctx context.Context, path string) ([]byte, error) {
	return f(ctx, path)
}

type Config struct {
	Path           string
	Timeout        time.Duration
	MaxOutputBytes int64
	Runner         Runner
}

type Generator struct {
	runner         Runner
	timeout        time.Duration
	maxOutputBytes int64
}

type CommandRunner struct {
	Path           string
	Timeout        time.Duration
	MaxOutputBytes int64
}

type Symbol struct {
	SourcePath      string
	Platform        string
	Architecture    string
	DebugIdentifier string
	DebugFile       string
	Content         []byte
}

type Root struct {
	Path       string
	InstanceID string
}

type SymbolStore interface {
	SaveGeneratedSymbol(context.Context, string, io.Reader) error
}

type Failure struct {
	Path  string
	Error string
}

type Summary struct {
	Scanned    int
	Candidates int
	Generated  int
	Skipped    int
	Failed     int
	Duplicates int
	Failures   []Failure
}

func New(config Config) (*Generator, error) {
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultGenerationTimeout
	}
	maxOutputBytes := config.MaxOutputBytes
	if maxOutputBytes <= 0 {
		maxOutputBytes = defaultMaxOutputBytes
	}
	runner := config.Runner
	if runner == nil {
		if !plainFilesystemPath(config.Path) {
			return nil, fmt.Errorf("%w: dump_syms path is required", ErrInvalidConfig)
		}
		runner = CommandRunner{Path: config.Path, Timeout: timeout, MaxOutputBytes: maxOutputBytes}
	}
	return &Generator{runner: runner, timeout: timeout, maxOutputBytes: maxOutputBytes}, nil
}

func (r CommandRunner) Run(ctx context.Context, inputPath string) ([]byte, error) {
	if !plainFilesystemPath(r.Path) {
		return nil, fmt.Errorf("%w: dump_syms path is invalid", ErrInvalidConfig)
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = defaultGenerationTimeout
	}
	maxOutputBytes := r.MaxOutputBytes
	if maxOutputBytes <= 0 {
		maxOutputBytes = defaultMaxOutputBytes
	}
	runContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(runContext, r.Path, inputPath)
	stdout := &limitedBuffer{limit: maxOutputBytes}
	stderr := &limitedBuffer{limit: 64 << 10}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		if stdout.exceeded {
			return nil, ErrSymbolOutputTooLarge
		}
		if err := runContext.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("dump_syms failed: %w", err)
	}
	if stdout.exceeded {
		return nil, ErrSymbolOutputTooLarge
	}
	return stdout.Bytes(), nil
}

func (g *Generator) Generate(ctx context.Context, sourcePath string) (Symbol, error) {
	if err := ctx.Err(); err != nil {
		return Symbol{}, err
	}
	if strings.TrimSpace(sourcePath) == "" {
		return Symbol{}, fmt.Errorf("%w: source path is required", ErrInvalidConfig)
	}
	runContext, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()
	raw, err := g.runner.Run(runContext, sourcePath)
	if err != nil {
		if runContext.Err() != nil {
			return Symbol{}, runContext.Err()
		}
		return Symbol{}, err
	}
	if int64(len(raw)) > g.maxOutputBytes {
		return Symbol{}, ErrSymbolOutputTooLarge
	}
	if bytes.IndexByte(raw, 0) >= 0 || !utf8.Valid(raw) {
		return Symbol{}, ErrInvalidSymbolOutput
	}
	symbol, err := parse(raw)
	if err != nil {
		return Symbol{}, err
	}
	symbol.SourcePath = sourcePath
	symbol.Content = append([]byte(nil), raw...)
	return symbol, nil
}

func (g *Generator) Scan(ctx context.Context, roots []Root, store SymbolStore) (Summary, error) {
	var summary Summary
	if store == nil {
		return summary, fmt.Errorf("%w: symbol store is required", ErrInvalidConfig)
	}
	seenRoots := make(map[string]struct{}, len(roots))
	seenModules := make(map[string]struct{})
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		if strings.TrimSpace(root.Path) == "" {
			continue
		}
		absolute, err := filepath.Abs(filepath.Clean(root.Path))
		if err != nil {
			return summary, err
		}
		if _, ok := seenRoots[absolute]; ok {
			continue
		}
		seenRoots[absolute] = struct{}{}
		info, err := os.Lstat(absolute)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return summary, fmt.Errorf("inspect symbol root %s: %w", absolute, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return summary, fmt.Errorf("symbol root is not a regular directory: %s", absolute)
		}
		err = filepath.WalkDir(absolute, func(sourcePath string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				addFailure(&summary, sourcePath, walkErr)
				return nil
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				addFailure(&summary, sourcePath, err)
				return nil
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			summary.Scanned++
			isELF, err := isELFFile(sourcePath)
			if err != nil {
				addFailure(&summary, sourcePath, err)
				return nil
			}
			if !isELF {
				return nil
			}
			summary.Candidates++
			symbol, err := g.Generate(ctx, sourcePath)
			if err != nil {
				if errors.Is(err, ErrNoSymbolRecords) || errors.Is(err, ErrZeroDebugIdentifier) {
					summary.Skipped++
					return nil
				}
				addFailure(&summary, sourcePath, err)
				return nil
			}
			moduleKey := strings.ToLower(strings.Join([]string{symbol.Platform, symbol.Architecture, symbol.DebugIdentifier, symbol.DebugFile}, "\x00"))
			if _, ok := seenModules[moduleKey]; ok {
				summary.Duplicates++
				return nil
			}
			if err := store.SaveGeneratedSymbol(ctx, root.InstanceID, bytes.NewReader(symbol.Content)); err != nil {
				addFailure(&summary, sourcePath, err)
				return nil
			}
			seenModules[moduleKey] = struct{}{}
			summary.Generated++
			return nil
		})
		if err != nil {
			return summary, err
		}
	}
	return summary, nil
}

func parse(raw []byte) (Symbol, error) {
	var moduleFields []string
	hasRecords := false
	for _, line := range bytes.Split(raw, []byte{'\n'}) {
		line = bytes.TrimSuffix(line, []byte{'\r'})
		fields := strings.Fields(string(line))
		if len(fields) > 0 && fields[0] == "MODULE" && moduleFields == nil {
			moduleFields = fields
		}
		if bytes.HasPrefix(line, []byte("FUNC ")) || bytes.HasPrefix(line, []byte("PUBLIC ")) || bytes.HasPrefix(line, []byte("STACK CFI ")) || bytes.HasPrefix(line, []byte("STACK WIN ")) {
			hasRecords = true
		}
	}
	if len(moduleFields) < 5 {
		return Symbol{}, ErrInvalidSymbolOutput
	}
	if !hasRecords {
		return Symbol{}, ErrNoSymbolRecords
	}
	debugIdentifier := moduleFields[3]
	if allZero(debugIdentifier) {
		return Symbol{}, ErrZeroDebugIdentifier
	}
	debugFile := strings.TrimSpace(moduleFields[4])
	debugFile = path.Base(strings.ReplaceAll(debugFile, "\\", "/"))
	if debugFile == "" || debugFile == "." || debugFile == "/" || debugFile == ".." || strings.ContainsAny(debugFile, "\x00\r\n") {
		return Symbol{}, ErrInvalidSymbolOutput
	}
	return Symbol{
		Platform:        strings.ToLower(moduleFields[1]),
		Architecture:    strings.ToLower(moduleFields[2]),
		DebugIdentifier: debugIdentifier,
		DebugFile:       debugFile,
	}, nil
}

func allZero(value string) bool {
	if value == "" {
		return true
	}
	for _, character := range value {
		if character != '0' {
			return false
		}
	}
	return true
}

func isELFFile(sourcePath string) (bool, error) {
	file, err := os.Open(sourcePath)
	if err != nil {
		return false, err
	}
	defer file.Close()
	header := make([]byte, 4)
	if _, err := io.ReadFull(file, header); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return false, nil
		}
		return false, err
	}
	return bytes.Equal(header, []byte{0x7f, 'E', 'L', 'F'}), nil
}

type FilesystemRootSource struct {
	ReleasesRoot  string
	InstancesRoot string
}

func (s FilesystemRootSource) Roots(ctx context.Context) ([]Root, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	roots := make([]Root, 0)
	if s.ReleasesRoot != "" {
		entries, err := os.ReadDir(s.ReleasesRoot)
		if os.IsNotExist(err) {
			entries = nil
		} else if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			roots = append(roots, Root{Path: filepath.Join(s.ReleasesRoot, entry.Name())})
		}
	}
	if s.InstancesRoot != "" {
		entries, err := os.ReadDir(s.InstancesRoot)
		if os.IsNotExist(err) {
			entries = nil
		} else if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			merged := filepath.Join(s.InstancesRoot, entry.Name(), "overlay", "merged")
			info, statErr := os.Lstat(merged)
			if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				continue
			}
			roots = append(roots, Root{Path: merged, InstanceID: entry.Name()})
		}
	}
	sort.Slice(roots, func(i, j int) bool {
		if roots[i].Path == roots[j].Path {
			return roots[i].InstanceID < roots[j].InstanceID
		}
		return roots[i].Path < roots[j].Path
	})
	return roots, nil
}

type Indexer struct {
	Generator *Generator
	Source    interface {
		Roots(context.Context) ([]Root, error)
	}
	Store SymbolStore
}

func (i Indexer) Scan(ctx context.Context) (Summary, error) {
	if i.Generator == nil || i.Source == nil || i.Store == nil {
		return Summary{}, fmt.Errorf("%w: indexer dependencies are required", ErrInvalidConfig)
	}
	roots, err := i.Source.Roots(ctx)
	if err != nil {
		return Summary{}, err
	}
	return i.Generator.Scan(ctx, roots, i.Store)
}

func (i Indexer) Start(parent context.Context, interval time.Duration, report func(Summary, error)) func() {
	if interval <= 0 {
		interval = defaultScanInterval
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		run := func() {
			summary, err := i.Scan(ctx)
			if report != nil {
				report(summary, err)
			}
		}
		run()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

type limitedBuffer struct {
	bytes.Buffer
	limit    int64
	exceeded bool
}

func (b *limitedBuffer) Write(value []byte) (int, error) {
	remaining := b.limit - int64(b.Len())
	if remaining <= 0 {
		b.exceeded = true
		return 0, ErrSymbolOutputTooLarge
	}
	if int64(len(value)) > remaining {
		b.exceeded = true
		_, _ = b.Buffer.Write(value[:remaining])
		return int(remaining), ErrSymbolOutputTooLarge
	}
	return b.Buffer.Write(value)
}

func addFailure(summary *Summary, sourcePath string, err error) {
	summary.Failed++
	if len(summary.Failures) < maxFailureRecords {
		summary.Failures = append(summary.Failures, Failure{Path: sourcePath, Error: err.Error()})
	}
}

func plainFilesystemPath(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.ContainsAny(value, "\x00\r\n") && !strings.Contains(value, "://") && (filepath.IsAbs(value) || (len(value) >= 3 && value[1] == ':' && (value[2] == '\\' || value[2] == '/')))
}

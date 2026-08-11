package crashsymbols

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testSymbolOutput = "MODULE Linux x86_64 ABCDEF1234567890 engine.so\nINFO GENERATOR test\nFUNC 1 2 engine_entry\n"

type recordingStore struct {
	entries []storedSymbol
}

type storedSymbol struct {
	instanceID string
	content    []byte
}

func (s *recordingStore) SaveGeneratedSymbol(_ context.Context, instanceID string, symbol io.Reader) error {
	raw, err := io.ReadAll(symbol)
	if err != nil {
		return err
	}
	s.entries = append(s.entries, storedSymbol{instanceID: instanceID, content: raw})
	return nil
}

func TestGenerateParsesMODULEAndRejectsOutputWithoutSymbolRecords(t *testing.T) {
	var gotPath string
	generator, err := New(Config{Runner: RunnerFunc(func(_ context.Context, path string) ([]byte, error) {
		gotPath = path
		return []byte(testSymbolOutput), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	symbol, err := generator.Generate(context.Background(), "/game/bin/engine.so")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/game/bin/engine.so" {
		t.Fatalf("runner path=%q", gotPath)
	}
	if symbol.Platform != "linux" || symbol.Architecture != "x86_64" || symbol.DebugIdentifier != "ABCDEF1234567890" || symbol.DebugFile != "engine.so" {
		t.Fatalf("symbol identity=%#v", symbol)
	}
	if !bytes.Equal(symbol.Content, []byte(testSymbolOutput)) {
		t.Fatalf("symbol content=%q", symbol.Content)
	}

	generator, err = New(Config{Runner: RunnerFunc(func(context.Context, string) ([]byte, error) {
		return []byte("MODULE Linux x86_64 ABCDEF1234567890 engine.so\nINFO only\n"), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := generator.Generate(context.Background(), "engine.so"); !errors.Is(err, ErrNoSymbolRecords) {
		t.Fatalf("missing records error=%v", err)
	}
}

func TestNewRequiresAFilesystemToolPathWithoutInjectedRunner(t *testing.T) {
	if _, err := New(Config{}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("missing path error=%v", err)
	}
	if _, err := New(Config{Path: "https://example.test/dump_syms"}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("URL path error=%v", err)
	}
}

func TestLimitedBufferStopsAtConfiguredOutputLimit(t *testing.T) {
	buffer := &limitedBuffer{limit: 4}
	if _, err := buffer.Write([]byte("12345")); !errors.Is(err, ErrSymbolOutputTooLarge) {
		t.Fatalf("write error=%v", err)
	}
	if buffer.String() != "1234" || !buffer.exceeded {
		t.Fatalf("buffer=%q exceeded=%v", buffer.String(), buffer.exceeded)
	}
}

func TestScanSkipsNonELFZeroIdentifierAndDuplicateModules(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "engine.so"), []byte{0x7f, 'E', 'L', 'F', 0x02, 'm', 'o', 'd', 'u', 'l', 'e'})
	writeTestFile(t, filepath.Join(root, "duplicate.so"), []byte{0x7f, 'E', 'L', 'F', 0x02, 'd', 'u', 'p', 'l', 'i', 'c', 'a', 't', 'e'})
	writeTestFile(t, filepath.Join(root, "not-a-module.so"), []byte("not an ELF"))
	writeTestFile(t, filepath.Join(root, "zero.so"), []byte{0x7f, 'E', 'L', 'F', 0x02, 'z', 'e', 'r', 'o'})

	generator, err := New(Config{Runner: RunnerFunc(func(_ context.Context, path string) ([]byte, error) {
		switch filepath.Base(path) {
		case "zero.so":
			return []byte("MODULE Linux x86_64 0000000000000000 zero.so\nFUNC 1 2 zero\n"), nil
		default:
			return []byte(testSymbolOutput), nil
		}
	})})
	if err != nil {
		t.Fatal(err)
	}
	store := &recordingStore{}
	summary, err := generator.Scan(context.Background(), []Root{{Path: root, InstanceID: "instance-a"}}, store)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Scanned != 4 || summary.Candidates != 3 || summary.Generated != 1 || summary.Duplicates != 1 || summary.Skipped != 1 || summary.Failed != 0 {
		t.Fatalf("summary=%+v", summary)
	}
	if len(store.entries) != 1 || store.entries[0].instanceID != "instance-a" || string(store.entries[0].content) != testSymbolOutput {
		t.Fatalf("stored=%#v", store.entries)
	}
}

func TestScanContinuesAfterModuleFailure(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "good.so"), []byte{0x7f, 'E', 'L', 'F', 'g', 'o', 'o', 'd'})
	writeTestFile(t, filepath.Join(root, "bad.so"), []byte{0x7f, 'E', 'L', 'F', 'b', 'a', 'd'})
	generator, err := New(Config{Runner: RunnerFunc(func(_ context.Context, path string) ([]byte, error) {
		if strings.HasSuffix(path, "bad.so") {
			return nil, errors.New("dump_syms failed")
		}
		return []byte("MODULE Linux x86_64 GOOD good.so\nFUNC 1 2 good\n"), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	store := &recordingStore{}
	summary, err := generator.Scan(context.Background(), []Root{{Path: root}}, store)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Generated != 1 || summary.Failed != 1 || len(summary.Failures) != 1 {
		t.Fatalf("summary=%+v", summary)
	}
	if len(store.entries) != 1 || !strings.Contains(summary.Failures[0].Path, "bad.so") {
		t.Fatalf("entries=%#v failures=%#v", store.entries, summary.Failures)
	}
}

func TestScanRejectsSymbolStoreFailure(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "engine.so"), []byte{0x7f, 'E', 'L', 'F', 'm', 'o', 'd', 'u', 'l', 'e'})
	generator, err := New(Config{Runner: RunnerFunc(func(context.Context, string) ([]byte, error) {
		return []byte(testSymbolOutput), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("store unavailable")
	summary, err := generator.Scan(context.Background(), []Root{{Path: root}}, failingStore{err: want})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Failed != 1 || len(summary.Failures) != 1 || !strings.Contains(summary.Failures[0].Error, want.Error()) {
		t.Fatalf("summary=%+v", summary)
	}
}

func TestFilesystemRootSourceFindsReleasesAndMergedInstanceOverlays(t *testing.T) {
	root := t.TempDir()
	releases := filepath.Join(root, "game", "releases")
	instances := filepath.Join(root, "instances")
	if err := os.MkdirAll(filepath.Join(releases, "release-a"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(instances, "instance-a", "overlay", "merged"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(instances, "instance-b", "overlay", "upper"), 0o750); err != nil {
		t.Fatal(err)
	}
	source := FilesystemRootSource{ReleasesRoot: releases, InstancesRoot: instances}
	roots, err := source.Roots(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 2 {
		t.Fatalf("roots=%#v", roots)
	}
	if roots[0].Path != filepath.Join(releases, "release-a") || roots[0].InstanceID != "" || roots[1].Path != filepath.Join(instances, "instance-a", "overlay", "merged") || roots[1].InstanceID != "instance-a" {
		t.Fatalf("roots=%#v", roots)
	}
}

type failingStore struct{ err error }

func (s failingStore) SaveGeneratedSymbol(context.Context, string, io.Reader) error { return s.err }

func writeTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

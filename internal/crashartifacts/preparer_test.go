package crashartifacts

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/not0721here/l4d2-control-panel/internal/crashreports"
	"github.com/not0721here/l4d2-control-panel/internal/crashsymbols"
	"github.com/not0721here/l4d2-control-panel/internal/docker"
)

type fakeContainerSource struct {
	containers []docker.Container
	archives   map[string][]byte
}

func (s fakeContainerSource) ListManaged(context.Context) ([]docker.Container, error) {
	return append([]docker.Container(nil), s.containers...), nil
}

func (s fakeContainerSource) GetArchive(_ context.Context, _, path string) (io.ReadCloser, error) {
	content, ok := s.archives[path]
	if !ok {
		return nil, errors.New("archive not found")
	}
	return io.NopCloser(bytes.NewReader(content)), nil
}

type fakeSymbolGenerator struct {
	symbol  crashsymbols.Symbol
	symbols []crashsymbols.Symbol
	paths   []string
}

func (g *fakeSymbolGenerator) Generate(_ context.Context, path string) (crashsymbols.Symbol, error) {
	g.paths = append(g.paths, path)
	result := g.symbol
	if len(g.symbols) > 0 {
		result = g.symbols[0]
		g.symbols = g.symbols[1:]
	}
	result.SourcePath = path
	return result, nil
}

func makeSystemLibraryArchive(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := tar.NewWriter(&output)
	if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func makeSystemLibraryReport(t *testing.T, manager *crashreports.Manager, debugID string) crashreports.Report {
	t.Helper()
	signature := "2|0|Linux|x86|1|SIGSEGV|0|0|M|libc.so.6|" + debugID
	presubmit, err := manager.PreSubmit(crashreports.PreSubmitInput{InstanceID: "instance-a", CrashSignature: signature})
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Split(presubmit, "|")[2]
	report, err := manager.Receive(context.Background(), crashreports.UploadInput{
		InstanceID: "instance-a", PresubmitToken: token,
		Minidump: bytes.NewReader([]byte("MDMPsystem-lib")), Metadata: strings.NewReader("metadata"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func TestPreparerExtractsMatchingLinuxSystemLibraryAndBackfillsArtifacts(t *testing.T) {
	manager, err := crashreports.New(crashreports.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	const debugID = "LIBCDEBUG"
	content := []byte("ELF libc bytes")
	report := makeSystemLibraryReport(t, manager, debugID)
	report.ContainerID = "game-container"
	source := fakeContainerSource{
		containers: []docker.Container{{ID: "game-container", Labels: map[string]string{docker.ManagedLabel: "true", docker.InstanceLabel: "instance-a", docker.RoleLabel: "game"}}},
		archives:   map[string][]byte{"/lib/i386-linux-gnu/libc.so.6": makeSystemLibraryArchive(t, "libc.so.6", content)},
	}
	generator := &fakeSymbolGenerator{symbol: crashsymbols.Symbol{
		Platform: "linux", Architecture: "x86", DebugIdentifier: debugID, DebugFile: "libc.so.6",
		Content: []byte("MODULE Linux x86 LIBCDEBUG libc.so.6\nFUNC 1 2 __libc_start_main\n"),
	}}
	preparer := &Preparer{Containers: source, Store: manager, Generator: generator, TempRoot: filepath.Join(t.TempDir(), "incoming")}
	if err := preparer.Prepare(context.Background(), report); err != nil {
		t.Fatal(err)
	}
	got, err := manager.Get(context.Background(), report.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Modules) != 1 || got.Modules[0].BinaryArtifact == "" || got.Modules[0].SymbolArtifact == "" {
		t.Fatalf("modules=%#v", got.Modules)
	}
	file, artifact, err := manager.OpenArtifact(context.Background(), crashreports.ArtifactKindBinary, got.Modules[0].BinaryArtifact)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	stored, err := io.ReadAll(file)
	if err != nil || !bytes.Equal(stored, content) || artifact.DebugIdentifier != debugID || artifact.DebugFile != "libc.so.6" {
		t.Fatalf("artifact=%#v bytes=%q err=%v", artifact, stored, err)
	}
	if len(generator.paths) != 1 || filepath.Base(generator.paths[0]) != "libc.so.6" {
		t.Fatalf("generator paths=%v", generator.paths)
	}
}

func TestPreparerRejectsDebugIdentifierMismatchWithoutSavingBinary(t *testing.T) {
	manager, err := crashreports.New(crashreports.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	report := makeSystemLibraryReport(t, manager, "EXPECTED")
	report.ContainerID = "game-container"
	preparer := &Preparer{
		Containers: fakeContainerSource{
			containers: []docker.Container{{ID: "game-container", Labels: map[string]string{docker.ManagedLabel: "true", docker.InstanceLabel: "instance-a", docker.RoleLabel: "game"}}},
			archives:   map[string][]byte{"/lib/i386-linux-gnu/libc.so.6": makeSystemLibraryArchive(t, "libc.so.6", []byte("ELF"))},
		},
		Store: manager, Generator: &fakeSymbolGenerator{symbol: crashsymbols.Symbol{Platform: "linux", Architecture: "x86", DebugIdentifier: "OTHER", DebugFile: "libc.so.6", Content: []byte("symbol")}}, TempRoot: t.TempDir(),
	}
	if err := preparer.Prepare(context.Background(), report); err == nil {
		t.Fatal("debug identifier mismatch was accepted")
	}
	got, err := manager.Get(context.Background(), report.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Modules[0].BinaryArtifact != "" || got.Modules[0].SymbolArtifact != "" {
		t.Fatalf("mismatched artifacts were saved: %#v", got.Modules)
	}
}

func TestPreparerContinuesAfterCandidateDebugIdentifierMismatch(t *testing.T) {
	manager, err := crashreports.New(crashreports.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	const debugID = "EXPECTED"
	report := makeSystemLibraryReport(t, manager, debugID)
	report.ContainerID = "game-container"
	content := []byte("ELF from second candidate")
	preparer := &Preparer{
		Containers: fakeContainerSource{
			containers: []docker.Container{{ID: "game-container", Labels: map[string]string{docker.ManagedLabel: "true", docker.InstanceLabel: "instance-a", docker.RoleLabel: "game"}}},
			archives: map[string][]byte{
				"/lib/i386-linux-gnu/libc.so.6":     makeSystemLibraryArchive(t, "libc.so.6", []byte("ELF from first candidate")),
				"/usr/lib/i386-linux-gnu/libc.so.6": makeSystemLibraryArchive(t, "libc.so.6", content),
			},
		},
		Store: manager,
		Generator: &fakeSymbolGenerator{symbols: []crashsymbols.Symbol{
			{Platform: "linux", Architecture: "x86", DebugIdentifier: "OTHER", DebugFile: "libc.so.6", Content: []byte("wrong symbol")},
			{Platform: "linux", Architecture: "x86", DebugIdentifier: debugID, DebugFile: "libc.so.6", Content: []byte("MODULE Linux x86 EXPECTED libc.so.6\n")},
		}},
		TempRoot: t.TempDir(),
	}
	if err := preparer.Prepare(context.Background(), report); err != nil {
		t.Fatal(err)
	}
	got, err := manager.Get(context.Background(), report.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Modules[0].BinaryArtifact == "" || got.Modules[0].SymbolArtifact == "" {
		t.Fatalf("artifacts=%#v", got.Modules)
	}
	file, _, err := manager.OpenArtifact(context.Background(), crashreports.ArtifactKindBinary, got.Modules[0].BinaryArtifact)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	stored, err := io.ReadAll(file)
	if err != nil || !bytes.Equal(stored, content) {
		t.Fatalf("stored=%q err=%v", stored, err)
	}
}

func TestPreparerRejectsMissingGameContainer(t *testing.T) {
	manager, err := crashreports.New(crashreports.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	report := makeSystemLibraryReport(t, manager, "EXPECTED")
	report.ContainerID = "missing-container"
	preparer := &Preparer{Containers: fakeContainerSource{}, Store: manager, Generator: &fakeSymbolGenerator{}, TempRoot: t.TempDir()}
	if err := preparer.Prepare(context.Background(), report); err == nil {
		t.Fatal("missing game container was silently accepted")
	}
}

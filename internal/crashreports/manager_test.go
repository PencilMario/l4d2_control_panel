package crashreports

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNewUsesNinetyDayDefaultAndCreatesPrivateDirectories(t *testing.T) {
	root := t.TempDir()
	_, err := New(Config{Root: root, Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"incoming", "pending", "reports", "symbols"} {
		info, statErr := os.Stat(filepath.Join(root, name))
		if statErr != nil || !info.IsDir() {
			t.Fatalf("directory %s: info=%v err=%v", name, info, statErr)
		}
	}
	if _, err := New(Config{}); err == nil {
		t.Fatal("empty root was accepted")
	}
}

func TestNewInstallsOnlySourceModAndMetamodBuiltinSymbols(t *testing.T) {
	manager, err := New(Config{Root: t.TempDir(), Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name            string
		debugIdentifier string
		artifactID      string
		debugFile       string
	}{
		{name: "SourceMod", debugIdentifier: "82F610A214789C43E883D413E3D5D89E0", artifactID: "b68a0fedbca140132e5326d00b9525c508f782efa33f80fd08975ae3931f4bd0", debugFile: "sourcemod.2.l4d2.so"},
		{name: "Metamod", debugIdentifier: "F75F0D6FF28DC359537F6E44559F9FDE0", artifactID: "0da46270ded6270082bbc91a8a75c88e8bfcfd6d859f86377e047e648e3ca6b7", debugFile: "metamod.2.l4d2.so"},
	} {
		t.Run(test.name, func(t *testing.T) {
			artifact, found := manager.hasMatchingArtifact(Module{DebugIdentifier: test.debugIdentifier, Platform: "linux", Architecture: "x86"}, ArtifactKindSymbol)
			if !found || !artifact.Builtin || artifact.ID != test.artifactID || artifact.DebugFile != test.debugFile {
				t.Fatalf("artifact=%#v found=%v", artifact, found)
			}
			for _, path := range []string{
				filepath.Join(manager.root, "symbols", "builtin", test.artifactID+".sym"),
				filepath.Join(manager.root, "symbols", test.debugFile, test.debugIdentifier, test.debugFile+".sym"),
			} {
				raw, readErr := os.ReadFile(path)
				if readErr != nil || !bytes.HasPrefix(raw, []byte("MODULE Linux x86 "+test.debugIdentifier+" "+test.debugFile)) {
					t.Fatalf("symbol path=%s bytes=%q err=%v", path, raw[:min(len(raw), 80)], readErr)
				}
			}
		})
	}
	entries, err := os.ReadDir(filepath.Join(manager.root, "symbol-manifests"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("builtin symbol manifests=%v err=%v", entries, err)
	}
}

func TestNewRejectsInvalidRetention(t *testing.T) {
	manager, err := New(Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	for _, days := range []int{0, -1, 3651} {
		if _, err := manager.Cleanup(context.Background(), days); err == nil {
			t.Fatalf("retention days=%d was accepted", days)
		}
	}
}

func TestReceiveStoresContentAddressedReportAndManifest(t *testing.T) {
	manager := newTestManager(t, time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC))
	dump := append([]byte("MDMP"), []byte("crash payload")...)
	metadata := []byte("GameDirectory=left4dead2\nCommandLine=srcds_run")

	report, err := manager.Receive(context.Background(), UploadInput{
		UserID:           "account",
		GameDirectory:    "left4dead2",
		ExtensionVersion: "1.0.0",
		ServerID:         "server-id",
		PresubmitToken:   "pending-token",
		Minidump:         bytes.NewReader(dump),
		Metadata:         bytes.NewReader(metadata),
	})
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(dump)
	wantID := hex.EncodeToString(hash[:])
	if report.ID != wantID || report.SHA256 != wantID {
		t.Fatalf("report=%#v want id=%s", report, wantID)
	}
	if report.MinidumpSize != int64(len(dump)) || report.MetadataSize != int64(len(metadata)) {
		t.Fatalf("sizes=%d/%d", report.MinidumpSize, report.MetadataSize)
	}
	if report.GameDirectory != "left4dead2" || report.ServerID != "server-id" {
		t.Fatalf("metadata=%#v", report)
	}
	base := filepath.Join(manager.root, "reports", report.ID)
	assertFileBytes(t, filepath.Join(base, "minidump.dmp"), dump)
	assertFileBytes(t, filepath.Join(base, "metadata.txt"), metadata)
	assertFileMode(t, filepath.Join(base, "minidump.dmp"), 0o600)
	assertFileMode(t, filepath.Join(base, "metadata.txt"), 0o600)
	assertFileMode(t, filepath.Join(base, "report.json"), 0o600)
	if entries, err := os.ReadDir(filepath.Join(manager.root, "incoming")); err != nil || len(entries) != 0 {
		t.Fatalf("incoming entries=%v err=%v", entries, err)
	}

	got, err := manager.Get(context.Background(), report.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != report.ID || !got.ReceivedAt.Equal(report.ReceivedAt) {
		t.Fatalf("get=%#v want=%#v", got, report)
	}
}

func TestReceivePersistsResolvedGameContainerID(t *testing.T) {
	manager, err := New(Config{
		Root: t.TempDir(),
		Now:  func() time.Time { return testProtocolNow() },
		ResolveContainerID: func(_ context.Context, instanceID string) (string, error) {
			if instanceID != "instance-a" {
				t.Fatalf("instance id=%q", instanceID)
			}
			return "game-container", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := manager.Receive(context.Background(), UploadInput{
		InstanceID: "instance-a", Minidump: bytes.NewReader([]byte("MDMPcontainer")), Metadata: strings.NewReader("metadata"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.ContainerID != "game-container" {
		t.Fatalf("container id=%q", report.ContainerID)
	}
	loaded, err := manager.Get(context.Background(), report.ID)
	if err != nil || loaded.ContainerID != "game-container" {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
}

func TestReceiveUsesGameContainerResolvedAtPreSubmit(t *testing.T) {
	currentContainerID := "old-game-container"
	resolveCalls := 0
	manager, err := New(Config{
		Root: t.TempDir(),
		Now:  func() time.Time { return testProtocolNow() },
		ResolveContainerID: func(_ context.Context, instanceID string) (string, error) {
			if instanceID != "instance-a" {
				t.Fatalf("instance id=%q", instanceID)
			}
			resolveCalls++
			return currentContainerID, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	presubmit, err := manager.PreSubmit(PreSubmitInput{InstanceID: "instance-a"})
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Split(presubmit, "|")[2]
	currentContainerID = "new-game-container"
	report, err := manager.Receive(context.Background(), UploadInput{
		InstanceID: "instance-a", PresubmitToken: token,
		Minidump: bytes.NewReader([]byte("MDMPpre-submit-container")), Metadata: strings.NewReader("metadata"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.ContainerID != "old-game-container" || resolveCalls != 1 {
		t.Fatalf("container id=%q resolve calls=%d", report.ContainerID, resolveCalls)
	}
}

func TestReceiveDeduplicatesDumpAndReplacesMetadata(t *testing.T) {
	manager := newTestManager(t, time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC))
	dump := append([]byte("MDMP"), []byte("same crash")...)
	first, err := manager.Receive(context.Background(), UploadInput{Minidump: bytes.NewReader(dump), Metadata: strings.NewReader("first")})
	if err != nil {
		t.Fatal(err)
	}
	manager.now = func() time.Time { return first.ReceivedAt.Add(time.Hour) }
	second, err := manager.Receive(context.Background(), UploadInput{Minidump: bytes.NewReader(dump), Metadata: strings.NewReader("second")})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("duplicate ids=%s/%s", first.ID, second.ID)
	}
	assertFileBytes(t, filepath.Join(manager.root, "reports", first.ID, "metadata.txt"), []byte("second"))
	reports, err := manager.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || !reports[0].UpdatedAt.After(reports[0].ReceivedAt) {
		t.Fatalf("reports=%#v", reports)
	}
}

func TestReceiveDoesNotTreatPresubmitTokenAsAPath(t *testing.T) {
	manager := newTestManager(t, time.Now().UTC())
	sentinel := filepath.Join(manager.root, "sentinel.json")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.Receive(context.Background(), UploadInput{
		PresubmitToken: "../sentinel",
		Minidump:       bytes.NewReader([]byte("MDMPpath safety")),
		Metadata:       strings.NewReader("metadata"),
	}); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "keep" {
		t.Fatalf("sentinel changed: bytes=%q err=%v", got, err)
	}
}

func TestReceiveRejectsInvalidDumpAndCleansTemporaryFiles(t *testing.T) {
	manager := newTestManager(t, time.Now().UTC())
	if _, err := manager.Receive(context.Background(), UploadInput{Minidump: strings.NewReader("NOPE"), Metadata: strings.NewReader("metadata")}); err == nil {
		t.Fatal("invalid minidump was accepted")
	}
	if entries, err := os.ReadDir(filepath.Join(manager.root, "incoming")); err != nil || len(entries) != 0 {
		t.Fatalf("incoming entries=%v err=%v", entries, err)
	}
	if _, err := manager.Receive(context.Background(), UploadInput{Minidump: bytes.NewReader([]byte("MDMP")), Metadata: nil}); err == nil {
		t.Fatal("missing metadata was accepted")
	}
}

func TestReceiveRollsBackInstalledDumpWhenReportCommitFails(t *testing.T) {
	manager := newTestManager(t, time.Now().UTC())
	dump := []byte("MDMPcommit failure")
	digest := sha256.Sum256(dump)
	reportDir := filepath.Join(manager.root, "reports", hex.EncodeToString(digest[:]))
	metadataDir := filepath.Join(reportDir, "metadata.txt")
	if err := os.MkdirAll(filepath.Join(metadataDir, "keep"), 0o750); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.Receive(context.Background(), UploadInput{
		Minidump: bytes.NewReader(dump),
		Metadata: strings.NewReader("metadata"),
	}); err == nil {
		t.Fatal("receive unexpectedly succeeded")
	}
	if _, err := os.Stat(filepath.Join(reportDir, "minidump.dmp")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("installed minidump remains: %v", err)
	}
	if entries, err := os.ReadDir(filepath.Join(manager.root, "incoming")); err != nil || len(entries) != 0 {
		t.Fatalf("incoming entries=%v err=%v", entries, err)
	}
}

func TestReceiveEnforcesFileLimits(t *testing.T) {
	manager := newTestManager(t, time.Now().UTC())
	tooLargeDump := io.MultiReader(bytes.NewReader([]byte("MDMP")), &repeatingReader{remaining: MaxMinidumpBytes})
	if _, err := manager.Receive(context.Background(), UploadInput{Minidump: tooLargeDump, Metadata: strings.NewReader("metadata")}); !errors.Is(err, ErrMinidumpTooLarge) {
		t.Fatalf("dump error=%v want=%v", err, ErrMinidumpTooLarge)
	}
	tooLargeMetadata := &repeatingReader{remaining: MaxMetadataBytes + 1}
	if _, err := manager.Receive(context.Background(), UploadInput{Minidump: bytes.NewReader([]byte("MDMP")), Metadata: tooLargeMetadata}); !errors.Is(err, ErrMetadataTooLarge) {
		t.Fatalf("metadata error=%v want=%v", err, ErrMetadataTooLarge)
	}
	if entries, err := os.ReadDir(filepath.Join(manager.root, "reports")); err != nil || len(entries) != 0 {
		t.Fatalf("reports entries=%v err=%v", entries, err)
	}
	if entries, err := os.ReadDir(filepath.Join(manager.root, "incoming")); err != nil || len(entries) != 0 {
		t.Fatalf("incoming entries=%v err=%v", entries, err)
	}
}

func TestSaveSymbolUsesGeneratedPath(t *testing.T) {
	manager := newTestManager(t, time.Now().UTC())
	want := []byte("MODULE Linux x86_64 ABC\nFUNC 1 2 symbol\n")
	if err := manager.SaveSymbol(context.Background(), SymbolInput{
		DebugIdentifier: "ABC/../../escape",
		CodeIdentifier:  "code",
		Symbol:          bytes.NewReader(want),
	}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(manager.root, "symbols", "uploaded"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("symbols=%v err=%v", entries, err)
	}
	assertFileBytes(t, filepath.Join(manager.root, "symbols", "uploaded", entries[0].Name()), want)
	if strings.Contains(entries[0].Name(), "escape") || strings.Contains(entries[0].Name(), "..") {
		t.Fatalf("caller data became path: %s", entries[0].Name())
	}
}

func TestCleanupRemovesExpiredReportsAndPendingTokensButKeepsSymbols(t *testing.T) {
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	manager := newTestManager(t, now)
	old, err := manager.Receive(context.Background(), UploadInput{Minidump: bytes.NewReader([]byte("MDMPold")), Metadata: strings.NewReader("old")})
	if err != nil {
		t.Fatal(err)
	}
	manager.now = func() time.Time { return now.Add(90 * 24 * time.Hour) }
	exact, err := manager.Receive(context.Background(), UploadInput{Minidump: bytes.NewReader([]byte("MDMPexact")), Metadata: strings.NewReader("exact")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.PreSubmit(PreSubmitInput{CrashSignature: "2|0|Linux|x86_64|1|SIGSEGV|abc|0|M|server|id"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.SaveSymbol(context.Background(), SymbolInput{DebugIdentifier: "debug", CodeIdentifier: "code", Symbol: strings.NewReader("symbol")}); err != nil {
		t.Fatal(err)
	}
	manager.now = func() time.Time { return now.Add(91 * 24 * time.Hour) }
	result, err := manager.Cleanup(context.Background(), 90)
	if err != nil {
		t.Fatal(err)
	}
	if result.ReportsRemoved != 1 {
		t.Fatalf("cleanup=%#v", result)
	}
	if _, err := manager.Get(context.Background(), old.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old report err=%v", err)
	}
	if _, err := manager.Get(context.Background(), exact.ID); err != nil {
		t.Fatalf("exact-boundary report was removed: %v", err)
	}
	if entries, err := os.ReadDir(filepath.Join(manager.root, "pending")); err != nil || len(entries) != 0 {
		t.Fatalf("pending entries=%v err=%v", entries, err)
	}
	if entries, err := os.ReadDir(filepath.Join(manager.root, "symbols", "uploaded")); err != nil || len(entries) != 1 {
		t.Fatalf("symbols entries=%v err=%v", entries, err)
	}
}

func TestCleanupRetainsReferencedAndBuiltinArtifactsButRemovesExpiredUnreferencedUploads(t *testing.T) {
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	manager := newTestManager(t, now)
	keepContent := []byte("referenced binary")
	keep, err := manager.SaveBinary(context.Background(), BinaryInput{
		DebugIdentifier: "KEEPDEBUG",
		CodeIdentifier:  "KEEPCODE",
		CodeFileName:    "server.dll",
		CodeFile:        bytes.NewReader(keepContent),
	})
	if err != nil {
		t.Fatal(err)
	}
	dropContent := []byte("unreferenced binary")
	drop, err := manager.SaveBinary(context.Background(), BinaryInput{
		DebugIdentifier: "DROPDEBUG",
		CodeIdentifier:  "DROPCODE",
		CodeFileName:    "other.dll",
		CodeFile:        bytes.NewReader(dropContent),
	})
	if err != nil {
		t.Fatal(err)
	}
	builtinContent := []byte("builtin symbol")
	builtinDigest := sha256.Sum256(builtinContent)
	builtinID := hex.EncodeToString(builtinDigest[:])
	builtin := Artifact{
		ID: builtinID, Kind: ArtifactKindSymbol, SHA256: builtinID, Size: int64(len(builtinContent)),
		DebugIdentifier: "BUILTINDEBUG", Builtin: true, ReceivedAt: now.Format(timeFormat),
	}
	if err := os.WriteFile(filepath.Join(manager.root, "symbols", "builtin", builtinID+".sym"), builtinContent, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.writeArtifactManifest(builtin); err != nil {
		t.Fatal(err)
	}

	manager.now = func() time.Time { return now.Add(89 * 24 * time.Hour) }
	signature := "2|0|Windows|x86|1|EXCEPTION|0|0|M|server.dll|KEEPDEBUG"
	pre, err := manager.PreSubmit(PreSubmitInput{CrashSignature: signature})
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(pre, "|")
	if len(parts) != 3 {
		t.Fatalf("presubmit=%q", pre)
	}
	if _, err := manager.Receive(context.Background(), UploadInput{
		PresubmitToken: parts[2],
		Minidump:       bytes.NewReader([]byte("MDMPreferenced")),
		Metadata:       strings.NewReader("metadata"),
	}); err != nil {
		t.Fatal(err)
	}

	manager.now = func() time.Time { return now.Add(91 * 24 * time.Hour) }
	result, err := manager.Cleanup(context.Background(), 90)
	if err != nil {
		t.Fatal(err)
	}
	if result.ArtifactsRemoved != 1 {
		t.Fatalf("cleanup=%#v", result)
	}
	keepFile, _, err := manager.OpenArtifact(context.Background(), ArtifactKindBinary, keep.ID)
	if err != nil {
		t.Fatalf("referenced binary removed: %v", err)
	}
	keepFile.Close()
	builtinFile, _, err := manager.OpenArtifact(context.Background(), ArtifactKindSymbol, builtinID)
	if err != nil {
		t.Fatalf("builtin symbol removed: %v", err)
	}
	builtinFile.Close()
	if _, _, err := manager.OpenArtifact(context.Background(), ArtifactKindBinary, drop.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unreferenced binary error=%v", err)
	}
}

func TestOpenRejectsInvalidReportAndFileKind(t *testing.T) {
	manager := newTestManager(t, time.Now().UTC())
	if _, _, err := manager.Open(context.Background(), "../escape", FileKindMinidump); !errors.Is(err, ErrNotFound) {
		t.Fatalf("invalid id error=%v", err)
	}
	report, err := manager.Receive(context.Background(), UploadInput{Minidump: bytes.NewReader([]byte("MDMPdata")), Metadata: strings.NewReader("meta")})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Open(context.Background(), report.ID, FileKind("unexpected")); !errors.Is(err, ErrInvalidFileKind) {
		t.Fatalf("invalid kind error=%v", err)
	}
	file, _, err := manager.Open(context.Background(), report.ID, FileKindMinidump)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	raw, err := io.ReadAll(file)
	if err != nil || string(raw) != "MDMPdata" {
		t.Fatalf("opened=%q err=%v", raw, err)
	}
}

type repeatingReader struct {
	remaining int64
}

func (r *repeatingReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	for i := range p {
		p[i] = 'x'
	}
	r.remaining -= int64(len(p))
	return len(p), nil
}

func newTestManager(t *testing.T, now time.Time) *Manager {
	t.Helper()
	manager, err := New(Config{
		Root:              t.TempDir(),
		Token:             "secret",
		Now:               func() time.Time { return now },
		AuthorizeInstance: allowTestInstance,
	})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func allowTestInstance(context.Context, string, string) error { return nil }

func assertFileBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s=%q want=%q", path, got, want)
	}
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		// Windows does not expose POSIX owner/group/other permission bits.
		return
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode %s=%o want=%o", path, got, want)
	}
}

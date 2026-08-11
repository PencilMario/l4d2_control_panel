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
	"strings"
	"testing"
)

func TestSaveBinaryIsContentAddressedAndStoresIdentityManifest(t *testing.T) {
	manager := newTestManager(t, testProtocolNow())
	content := []byte("ELF module bytes")
	wantHash := sha256.Sum256(content)
	wantID := hex.EncodeToString(wantHash[:])
	input := BinaryInput{
		InstanceID:      "instance-1",
		DebugIdentifier: "SERVERDEBUG",
		CodeIdentifier:  "SERVERCODE",
		CodeFileName:    `/srv/left4dead2/bin/server.so`,
		CodeFile:        bytes.NewReader(content),
	}
	first, err := manager.SaveBinary(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.SaveBinary(context.Background(), BinaryInput{
		InstanceID:      "instance-1",
		DebugIdentifier: "SERVERDEBUG",
		CodeIdentifier:  "SERVERCODE",
		CodeFileName:    `C:\game\bin\server.so`,
		CodeFile:        bytes.NewReader(content),
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != wantID || second.ID != wantID || first.Basename != "server.so" {
		t.Fatalf("artifacts=%#v %#v want id=%s", first, second, wantID)
	}
	file, artifact, err := manager.OpenArtifact(context.Background(), ArtifactKindBinary, wantID)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	got, err := io.ReadAll(file)
	if err != nil || !bytes.Equal(got, content) || artifact.DebugIdentifier != "SERVERDEBUG" || artifact.InstanceID != "instance-1" {
		t.Fatalf("artifact=%#v bytes=%q err=%v", artifact, got, err)
	}
	entries, err := os.ReadDir(filepath.Join(manager.root, "binaries"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("binaries=%v err=%v", entries, err)
	}
}

func TestSaveBinaryDoesNotFollowExistingSymlink(t *testing.T) {
	manager := newTestManager(t, testProtocolNow())
	content := []byte("ELF module bytes")
	digest := sha256.Sum256(content)
	id := hex.EncodeToString(digest[:])
	target := filepath.Join(manager.root, "binaries", id+".bin")
	sentinel := filepath.Join(manager.root, "sentinel.bin")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sentinel, target); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	_, err := manager.SaveBinary(context.Background(), BinaryInput{DebugIdentifier: "DEBUG", CodeIdentifier: "CODE", CodeFileName: "server.so", CodeFile: bytes.NewReader(content)})
	if err == nil {
		t.Fatal("binary save followed existing symlink")
	}
	if got, readErr := os.ReadFile(sentinel); readErr != nil || string(got) != "keep" {
		t.Fatalf("sentinel=%q err=%v", got, readErr)
	}
}

func TestSaveBinaryEnforcesIndependentLimit(t *testing.T) {
	manager := newTestManager(t, testProtocolNow())
	tooLarge := &repeatingReader{remaining: MaxBinaryBytes + 1}
	_, err := manager.SaveBinary(context.Background(), BinaryInput{DebugIdentifier: "DEBUG", CodeIdentifier: "CODE", CodeFileName: "server.so", CodeFile: tooLarge})
	if !errors.Is(err, ErrBinaryTooLarge) {
		t.Fatalf("error=%v want=%v", err, ErrBinaryTooLarge)
	}
	entries, readErr := os.ReadDir(filepath.Join(manager.root, "binaries"))
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("binaries=%v err=%v", entries, readErr)
	}
}

func TestBinaryIdentityRejectsControlFields(t *testing.T) {
	manager := newTestManager(t, testProtocolNow())
	for _, identifier := range []string{"", "DEBUG|other", "DEBUG\x00other", strings.Repeat("x", 257)} {
		if _, err := manager.SaveBinary(context.Background(), BinaryInput{DebugIdentifier: identifier, CodeIdentifier: "CODE", CodeFileName: "server.so", CodeFile: strings.NewReader("bytes")}); !errors.Is(err, ErrInvalidBinary) {
			t.Fatalf("identifier %q error=%v", identifier, err)
		}
	}
}

func TestSaveSymbolIsContentAddressedAndStoresIdentityManifest(t *testing.T) {
	manager := newTestManager(t, testProtocolNow())
	content := []byte("MODULE Linux x86_64 SERVERDEBUG sourcemod.2.l4d2.so\nFUNC 1 2 symbol\n")
	digest := sha256.Sum256(content)
	wantID := hex.EncodeToString(digest[:])
	if err := manager.SaveSymbol(context.Background(), SymbolInput{
		InstanceID:      "instance-1",
		DebugIdentifier: "SERVERDEBUG",
		CodeIdentifier:  "SERVERCODE",
		Platform:        "linux",
		Architecture:    "x86_64",
		Symbol:          bytes.NewReader(content),
	}); err != nil {
		t.Fatal(err)
	}
	file, artifact, err := manager.OpenArtifact(context.Background(), ArtifactKindSymbol, wantID)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	got, err := io.ReadAll(file)
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("symbol bytes=%q err=%v", got, err)
	}
	if artifact.ID != wantID || artifact.InstanceID != "instance-1" || artifact.Platform != "linux" || artifact.Architecture != "x86_64" {
		t.Fatalf("artifact=%#v", artifact)
	}
	lookupPath := filepath.Join(manager.root, "symbols", "sourcemod.2.l4d2.so", "SERVERDEBUG", "sourcemod.2.l4d2.so.sym")
	assertFileBytes(t, lookupPath, content)
	entries, err := os.ReadDir(filepath.Join(manager.root, "symbols", "uploaded"))
	if err != nil || len(entries) != 1 || entries[0].Name() != wantID+".sym" {
		t.Fatalf("uploaded symbols=%v err=%v", entries, err)
	}
}

func TestLateArtifactsBackfillExistingReportModules(t *testing.T) {
	manager := newTestManager(t, testProtocolNow())
	signature := "2|0|Linux|x86_64|1|SIGSEGV|0|0|M|server.so|LATE-SYMBOL"
	preSubmit, err := manager.PreSubmit(PreSubmitInput{CrashSignature: signature})
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(preSubmit, "|")
	if len(parts) != 3 {
		t.Fatalf("presubmit=%q", preSubmit)
	}
	report, err := manager.Receive(context.Background(), UploadInput{
		PresubmitToken: parts[2],
		Minidump:       bytes.NewReader([]byte("MDMPlate-symbol")),
		Metadata:       strings.NewReader("metadata"),
	})
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("MODULE Linux x86_64 LATE-SYMBOL server.so\nFUNC 1 2 late\n")
	artifactID := symbolHash(content)
	if err := manager.SaveSymbol(context.Background(), SymbolInput{
		DebugIdentifier: "LATE-SYMBOL",
		DebugFile:       "server.so",
		Platform:        "linux",
		Architecture:    "x86_64",
		Symbol:          bytes.NewReader(content),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := manager.Get(context.Background(), report.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Modules) != 1 || got.Modules[0].SymbolArtifact != artifactID {
		t.Fatalf("late symbol was not linked: modules=%#v artifact=%s", got.Modules, artifactID)
	}
}

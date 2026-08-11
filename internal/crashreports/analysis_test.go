package crashreports

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReceiveEnqueuesOnlyNewReportsWithoutBlockingUpload(t *testing.T) {
	enqueued := make(chan Report, 2)
	manager, err := New(Config{
		Root:  t.TempDir(),
		Token: "secret",
		EnqueueAnalysis: func(_ context.Context, report Report) error {
			enqueued <- report
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	dump := []byte("MDMPauto-enqueue")
	first, err := manager.Receive(context.Background(), UploadInput{InstanceID: "instance-a", Minidump: bytes.NewReader(dump), Metadata: strings.NewReader("metadata")})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case received := <-enqueued:
		if received.ID != first.ID || received.InstanceID != "instance-a" {
			t.Fatalf("queued report=%#v want=%#v", received, first)
		}
	case <-time.After(time.Second):
		t.Fatal("new report was not enqueued")
	}
	if _, err := manager.Receive(context.Background(), UploadInput{InstanceID: "instance-a", Minidump: bytes.NewReader(dump), Metadata: strings.NewReader("duplicate")}); err != nil {
		t.Fatal(err)
	}
	select {
	case duplicate := <-enqueued:
		t.Fatalf("duplicate report was enqueued: %#v", duplicate)
	case <-time.After(30 * time.Millisecond):
	}
}

func TestAnalysisUpdatesPersistDerivedFilesAndExposeAuthenticatedKinds(t *testing.T) {
	manager := newTestManager(t, testProtocolNow())
	report, err := manager.Receive(context.Background(), UploadInput{Minidump: bytes.NewReader([]byte("MDMPanalysis")), Metadata: strings.NewReader("metadata")})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetStackwalkStatus(context.Background(), report.ID, AnalysisStatusRunning, ""); err != nil {
		t.Fatal(err)
	}
	if err := manager.SaveStackwalk(context.Background(), report.ID, StackwalkUpdate{Status: AnalysisStatusSucceeded, Text: "#0 accelerator", Tool: "minidump_stackwalk"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.SaveAIAnalysis(context.Background(), report.ID, AIAnalysisUpdate{Status: AnalysisStatusSucceeded, Model: "model", InputSHA256: strings.Repeat("a", 64), Text: "analysis result"}); err != nil {
		t.Fatal(err)
	}
	got, err := manager.Get(context.Background(), report.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.StackwalkStatus != AnalysisStatusSucceeded || got.StackwalkTool != "minidump_stackwalk" || got.AIStatus != AnalysisStatusSucceeded || got.AIAnalysis != "analysis result" {
		t.Fatalf("report=%#v", got)
	}
	for _, kind := range []FileKind{FileKindStackwalk, FileKindAI} {
		file, _, openErr := manager.Open(context.Background(), report.ID, kind)
		if openErr != nil {
			t.Fatalf("open %s: %v", kind, openErr)
		}
		data, readErr := io.ReadAll(file)
		file.Close()
		if readErr != nil || len(data) == 0 {
			t.Fatalf("kind=%s data=%q err=%v", kind, data, readErr)
		}
	}
}

func TestSetAIStatusRemovesStaleAnalysisFile(t *testing.T) {
	manager := newTestManager(t, testProtocolNow())
	report, err := manager.Receive(context.Background(), UploadInput{
		Minidump: bytes.NewReader([]byte("MDMPstale-ai")),
		Metadata: strings.NewReader("metadata"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SaveAIAnalysis(context.Background(), report.ID, AIAnalysisUpdate{
		Status: AnalysisStatusSucceeded,
		Model:  "model",
		Text:   "old analysis",
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetAIStatus(context.Background(), report.ID, AIAnalysisUpdate{
		Status: AnalysisStatusQueued,
		Model:  "model",
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Open(context.Background(), report.ID, FileKindAI); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stale AI file remains: %v", err)
	}
}

func TestPrepareStackwalkCopiesDumpAndCleansTemporaryPath(t *testing.T) {
	manager := newTestManager(t, testProtocolNow())
	report, err := manager.Receive(context.Background(), UploadInput{Minidump: bytes.NewReader([]byte("MDMPcopy")), Metadata: strings.NewReader("metadata")})
	if err != nil {
		t.Fatal(err)
	}
	input, _, cleanup, err := manager.PrepareStackwalk(context.Background(), report.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	data, err := os.ReadFile(input.DumpPath)
	if err != nil || string(data) != "MDMPcopy" || input.SymbolRoot == "" {
		t.Fatalf("input=%+v data=%q err=%v", input, data, err)
	}
	cleanup()
	if _, err := os.Stat(input.DumpPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary dump remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(manager.root, "reports", report.ID, "minidump.dmp")); err != nil {
		t.Fatal(err)
	}
}

func TestDuplicateReceivePreservesAnalysisState(t *testing.T) {
	manager := newTestManager(t, testProtocolNow())
	dump := []byte("MDMPpreserve-analysis")
	first, err := manager.Receive(context.Background(), UploadInput{Minidump: bytes.NewReader(dump), Metadata: strings.NewReader("first")})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SaveStackwalk(context.Background(), first.ID, StackwalkUpdate{Status: AnalysisStatusSucceeded, Text: "stackwalk", Tool: "tool"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.SaveAIAnalysis(context.Background(), first.ID, AIAnalysisUpdate{Status: AnalysisStatusSucceeded, Model: "model", Text: "analysis"}); err != nil {
		t.Fatal(err)
	}
	second, err := manager.Receive(context.Background(), UploadInput{Minidump: bytes.NewReader(dump), Metadata: strings.NewReader("retry")})
	if err != nil {
		t.Fatal(err)
	}
	if second.StackwalkStatus != AnalysisStatusSucceeded || second.AIStatus != AnalysisStatusSucceeded || second.AIAnalysis != "analysis" {
		t.Fatalf("duplicate report lost analysis state: %#v", second)
	}
}

package crashanalysis

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/crashreports"
)

type workerStore struct {
	mu             sync.Mutex
	report         crashreports.Report
	metadata       []byte
	stackwalkCalls int
	aiCalls        int
	stackwalk      crashreports.StackwalkUpdate
	ai             crashreports.AIAnalysisUpdate
	recovered      []crashreports.AnalysisRecovery
	done           chan struct{}
}

func (s *workerStore) Get(context.Context, string) (crashreports.Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.report, nil
}
func (s *workerStore) PrepareStackwalk(context.Context, string) (crashreports.StackwalkInput, crashreports.Report, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return crashreports.StackwalkInput{DumpPath: "dump", SymbolRoot: "symbols"}, s.report, func() {}, nil
}
func (s *workerStore) ReadMetadata(context.Context, string) ([]byte, crashreports.Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.metadata...), s.report, nil
}
func (s *workerStore) ReadStackwalk(context.Context, string) (string, crashreports.Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stackwalk.Text, s.report, nil
}
func (s *workerStore) SetStackwalkStatus(_ context.Context, _ string, status crashreports.AnalysisStatus, message string) error {
	s.mu.Lock()
	s.report.StackwalkStatus = status
	s.stackwalk.Status = status
	s.stackwalk.Error = message
	s.mu.Unlock()
	return nil
}
func (s *workerStore) SaveStackwalk(_ context.Context, _ string, update crashreports.StackwalkUpdate) error {
	s.mu.Lock()
	s.stackwalkCalls++
	s.stackwalk = update
	s.report.StackwalkStatus = update.Status
	s.mu.Unlock()
	if s.done != nil && update.Status == crashreports.AnalysisStatusSucceeded && s.ai.Status != crashreports.AnalysisStatusQueued {
		select {
		case s.done <- struct{}{}:
		default:
		}
	}
	return nil
}
func (s *workerStore) SetAIStatus(_ context.Context, _ string, update crashreports.AIAnalysisUpdate) error {
	s.mu.Lock()
	s.ai = update
	s.report.AIStatus = update.Status
	if s.done != nil && update.Status == crashreports.AnalysisStatusUnconfigured {
		select {
		case s.done <- struct{}{}:
		default:
		}
	}
	s.mu.Unlock()
	return nil
}
func (s *workerStore) SaveAIAnalysis(_ context.Context, _ string, update crashreports.AIAnalysisUpdate) error {
	s.mu.Lock()
	s.aiCalls++
	s.ai = update
	s.report.AIStatus = update.Status
	if s.done != nil && update.Status == crashreports.AnalysisStatusSucceeded {
		select {
		case s.done <- struct{}{}:
		default:
		}
	}
	s.mu.Unlock()
	return nil
}
func (s *workerStore) RecoverAnalysis(context.Context) ([]crashreports.AnalysisRecovery, error) {
	return append([]crashreports.AnalysisRecovery(nil), s.recovered...), nil
}

type workerStackwalk struct{ calls int }

func (s *workerStackwalk) Run(context.Context, string) (StackwalkResult, error) {
	s.calls++
	return StackwalkResult{Text: "#0 accelerator", Tool: "fake"}, nil
}

type workerAI struct{ calls int }

func (a *workerAI) Analyze(_ context.Context, input []byte) (string, error) {
	a.calls++
	if strings.Contains(string(input), "MDMP") {
		return "raw dump leaked", nil
	}
	return "AI result", nil
}

func TestWorkerRunsStackwalkAndAIAndCoalescesDuplicateEnqueue(t *testing.T) {
	store := &workerStore{report: crashreports.Report{ID: strings.Repeat("a", 64), ParsedSignature: &crashreports.CrashSignature{CrashReason: "SIGSEGV"}}, metadata: []byte("metadata"), done: make(chan struct{}, 1)}
	stackwalk := &workerStackwalk{}
	ai := &workerAI{}
	worker, err := NewWorker(WorkerConfig{Store: store, Stackwalker: stackwalk, AI: ai, AIModel: "model"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := worker.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer worker.Stop(context.Background())
	if err := worker.Enqueue(ctx, store.report.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := worker.Enqueue(ctx, store.report.ID, true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-store.done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not finish")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if stackwalk.calls != 1 || ai.calls != 1 || store.stackwalkCalls != 1 || store.aiCalls != 1 || store.ai.Status != crashreports.AnalysisStatusSucceeded {
		t.Fatalf("stackwalk=%d ai=%d store=%+v", stackwalk.calls, ai.calls, store)
	}
}

func TestWorkerMarksAIUnconfiguredWithoutCallingClient(t *testing.T) {
	store := &workerStore{report: crashreports.Report{ID: strings.Repeat("b", 64)}, metadata: []byte("metadata"), done: make(chan struct{}, 1)}
	stackwalk := &workerStackwalk{}
	worker, err := NewWorker(WorkerConfig{Store: store, Stackwalker: stackwalk})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer worker.Stop(context.Background())
	if err := worker.Enqueue(context.Background(), store.report.ID, true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-store.done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not finish")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.ai.Status != crashreports.AnalysisStatusUnconfigured || store.aiCalls != 0 {
		t.Fatalf("ai=%+v calls=%d", store.ai, store.aiCalls)
	}
}

func TestWorkerRunsStackwalkWithoutRequestingAI(t *testing.T) {
	store := &workerStore{report: crashreports.Report{ID: strings.Repeat("f", 64)}, metadata: []byte("metadata"), done: make(chan struct{}, 1)}
	stackwalk := &workerStackwalk{}
	ai := &workerAI{}
	worker, err := NewWorker(WorkerConfig{Store: store, Stackwalker: stackwalk, AI: ai, AIModel: "model"})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer worker.Stop(context.Background())
	if err := worker.Enqueue(context.Background(), store.report.ID, false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-store.done:
	case <-time.After(2 * time.Second):
		t.Fatal("stackwalk-only analysis did not finish")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if stackwalk.calls != 1 || store.stackwalkCalls != 1 || store.stackwalk.Status != crashreports.AnalysisStatusSucceeded || ai.calls != 0 || store.aiCalls != 0 || store.ai.Status != "" {
		t.Fatalf("stackwalk_calls=%d stackwalk=%+v ai_calls=%d ai_client_calls=%d", store.stackwalkCalls, store.stackwalk, store.aiCalls, ai.calls)
	}
}

func TestWorkerRecoversQueuedReportsOnStart(t *testing.T) {
	store := &workerStore{report: crashreports.Report{ID: strings.Repeat("c", 64)}, metadata: []byte("metadata"), recovered: []crashreports.AnalysisRecovery{{ID: strings.Repeat("c", 64)}}, done: make(chan struct{}, 1)}
	worker, err := NewWorker(WorkerConfig{Store: store, Stackwalker: &workerStackwalk{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer worker.Stop(context.Background())
	select {
	case <-store.done:
	case <-time.After(2 * time.Second):
		t.Fatal("recovered report did not finish")
	}
}

func TestWorkerRecoversAIQueuedReportsWithoutDroppingAIRequest(t *testing.T) {
	store := &workerStore{
		report:    crashreports.Report{ID: strings.Repeat("e", 64), StackwalkStatus: crashreports.AnalysisStatusSucceeded, AIStatus: crashreports.AnalysisStatusQueued},
		metadata:  []byte("metadata"),
		stackwalk: crashreports.StackwalkUpdate{Status: crashreports.AnalysisStatusSucceeded, Text: "#0 recovered"},
		recovered: []crashreports.AnalysisRecovery{{ID: strings.Repeat("e", 64), RequestAI: true}},
		done:      make(chan struct{}, 1),
	}
	ai := &workerAI{}
	worker, err := NewWorker(WorkerConfig{Store: store, Stackwalker: &workerStackwalk{}, AI: ai, AIModel: "model"})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer worker.Stop(context.Background())
	select {
	case <-store.done:
	case <-time.After(2 * time.Second):
		t.Fatal("recovered AI report did not finish")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if ai.calls != 1 || store.ai.Status != crashreports.AnalysisStatusSucceeded || store.stackwalkCalls != 0 {
		t.Fatalf("ai_calls=%d ai=%+v stackwalk_calls=%d", ai.calls, store.ai, store.stackwalkCalls)
	}
}

func TestWorkerResolvesAIClientAndModelForEachAnalysis(t *testing.T) {
	store := &workerStore{report: crashreports.Report{ID: strings.Repeat("d", 64)}, metadata: []byte("metadata"), done: make(chan struct{}, 1)}
	stackwalk := &workerStackwalk{}
	ai := &workerAI{}
	providerCalls := 0
	worker, err := NewWorker(WorkerConfig{
		Store: store, Stackwalker: stackwalk,
		AIProvider: func(context.Context) (AIAnalyzer, string, error) {
			providerCalls++
			return ai, "dynamic-model", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer worker.Stop(context.Background())
	if err := worker.Enqueue(context.Background(), store.report.ID, true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-store.done:
	case <-time.After(2 * time.Second):
		t.Fatal("dynamic AI analysis did not finish")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if providerCalls != 1 || ai.calls != 1 || store.ai.Model != "dynamic-model" || store.ai.Status != crashreports.AnalysisStatusSucceeded {
		t.Fatalf("provider_calls=%d ai_calls=%d ai=%+v", providerCalls, ai.calls, store.ai)
	}
}

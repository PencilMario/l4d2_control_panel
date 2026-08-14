package crashanalysis

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/crashreports"
)

type AnalysisStore interface {
	Get(context.Context, string) (crashreports.Report, error)
	PrepareStackwalk(context.Context, string) (crashreports.StackwalkInput, crashreports.Report, func(), error)
	ReadMetadata(context.Context, string) ([]byte, crashreports.Report, error)
	ReadStackwalk(context.Context, string) (string, crashreports.Report, error)
	SetStackwalkStatus(context.Context, string, crashreports.AnalysisStatus, string) error
	SaveStackwalk(context.Context, string, crashreports.StackwalkUpdate) error
	SetAIStatus(context.Context, string, crashreports.AIAnalysisUpdate) error
	SaveAIAnalysis(context.Context, string, crashreports.AIAnalysisUpdate) error
	RecoverAnalysis(context.Context) ([]crashreports.AnalysisRecovery, error)
}

type AIAnalyzer interface {
	Analyze(context.Context, []byte) (string, error)
}

type AIProvider func(context.Context) (AIAnalyzer, string, error)

type StackwalkRunner interface {
	Run(context.Context, string) (StackwalkResult, error)
}

type ArtifactPreparer interface {
	Prepare(context.Context, crashreports.Report) error
}

type WorkerConfig struct {
	Store       AnalysisStore
	Stackwalker StackwalkRunner
	Preparer    ArtifactPreparer
	AI          AIAnalyzer
	AIModel     string
	AIProvider  AIProvider
	QueueSize   int
}

type Worker struct {
	store       AnalysisStore
	stackwalker StackwalkRunner
	preparer    ArtifactPreparer
	ai          AIAnalyzer
	aiModel     string
	aiProvider  AIProvider
	queue       chan workerJob
	mu          sync.Mutex
	pending     map[string]struct{}
	started     bool
	cancel      context.CancelFunc
	done        chan struct{}
}

type workerJob struct {
	reportID  string
	requestAI bool
}

func NewWorker(config WorkerConfig) (*Worker, error) {
	if config.Store == nil || config.Stackwalker == nil {
		return nil, errors.New("analysis store and stackwalker are required")
	}
	if config.QueueSize <= 0 {
		config.QueueSize = 64
	}
	return &Worker{store: config.Store, stackwalker: config.Stackwalker, preparer: config.Preparer, ai: config.AI, aiModel: strings.TrimSpace(config.AIModel), aiProvider: config.AIProvider, queue: make(chan workerJob, config.QueueSize), pending: make(map[string]struct{})}, nil
}

func (w *Worker) Start(parent context.Context) error {
	if parent == nil {
		parent = context.Background()
	}
	recovered, err := w.store.RecoverAnalysis(parent)
	if err != nil {
		return err
	}
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	w.started = true
	w.done = make(chan struct{})
	w.mu.Unlock()
	go w.loop(ctx)
	for _, job := range recovered {
		if err := w.enqueueInternal(job.ID, job.RequestAI); err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) Stop(ctx context.Context) error {
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		return nil
	}
	cancel, done := w.cancel, w.done
	w.started = false
	w.mu.Unlock()
	cancel()
	select {
	case <-done:
		w.mu.Lock()
		w.pending = make(map[string]struct{})
		w.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *Worker) Enqueue(ctx context.Context, reportID string, requestAI bool) error {
	if _, err := w.store.Get(ctx, reportID); err != nil {
		return err
	}
	if err := w.store.SetStackwalkStatus(ctx, reportID, crashreports.AnalysisStatusQueued, ""); err != nil {
		return err
	}
	if requestAI {
		if err := w.store.SetAIStatus(ctx, reportID, crashreports.AIAnalysisUpdate{Status: crashreports.AnalysisStatusQueued, Model: w.aiModel}); err != nil {
			return err
		}
	}
	return w.enqueueInternal(reportID, requestAI)
}

func (w *Worker) enqueueInternal(reportID string, requestAI bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, exists := w.pending[reportID]; exists {
		return nil
	}
	if !w.started {
		return errors.New("analysis worker is not started")
	}
	w.pending[reportID] = struct{}{}
	select {
	case w.queue <- workerJob{reportID: reportID, requestAI: requestAI}:
		return nil
	default:
		delete(w.pending, reportID)
		return errors.New("analysis queue is full")
	}
}

func (w *Worker) loop(ctx context.Context) {
	defer close(w.done)
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-w.queue:
			_ = w.Analyze(ctx, job.reportID, job.requestAI)
			w.mu.Lock()
			delete(w.pending, job.reportID)
			w.mu.Unlock()
		}
	}
}

func (w *Worker) Analyze(ctx context.Context, reportID string, requestAI bool) error {
	report, err := w.store.Get(ctx, reportID)
	if err != nil {
		return err
	}
	stackwalkText := ""
	if requestAI && report.StackwalkStatus == crashreports.AnalysisStatusSucceeded {
		stackwalkText, _, err = w.store.ReadStackwalk(ctx, reportID)
		if err != nil {
			stackwalkText = ""
		}
	}
	if stackwalkText == "" {
		if w.preparer != nil {
			_ = w.preparer.Prepare(ctx, report)
		}
		if err := w.store.SetStackwalkStatus(ctx, reportID, crashreports.AnalysisStatusRunning, ""); err != nil {
			return err
		}
		input, preparedReport, cleanup, prepareErr := w.store.PrepareStackwalk(ctx, reportID)
		if prepareErr != nil {
			_ = w.store.SaveStackwalk(ctx, reportID, crashreports.StackwalkUpdate{Status: crashreports.AnalysisStatusFailed, Error: trimError(prepareErr)})
			return prepareErr
		}
		report = preparedReport
		defer cleanup()
		stackwalk, runErr := w.stackwalker.Run(ctx, input.DumpPath)
		if runErr != nil {
			_ = w.store.SaveStackwalk(ctx, reportID, crashreports.StackwalkUpdate{Status: crashreports.AnalysisStatusFailed, Error: trimError(runErr), Tool: stackwalk.Tool})
			return runErr
		}
		stackwalkText = stackwalk.Text
		if err := w.store.SaveStackwalk(ctx, reportID, crashreports.StackwalkUpdate{Status: crashreports.AnalysisStatusSucceeded, Text: stackwalkText, Tool: stackwalk.Tool}); err != nil {
			return err
		}
	}
	if !requestAI {
		return nil
	}
	ai := w.ai
	model := w.aiModel
	if w.aiProvider != nil {
		var err error
		ai, model, err = w.aiProvider(ctx)
		model = strings.TrimSpace(model)
		if err != nil {
			_ = w.store.SaveAIAnalysis(ctx, reportID, crashreports.AIAnalysisUpdate{Status: crashreports.AnalysisStatusFailed, Model: model, Error: trimError(err), CompletedAt: time.Now().UTC()})
			return err
		}
	}
	if ai == nil || model == "" {
		return w.store.SetAIStatus(ctx, reportID, crashreports.AIAnalysisUpdate{Status: crashreports.AnalysisStatusUnconfigured, Model: model, Error: "AI analysis is not configured"})
	}
	metadata, latest, err := w.store.ReadMetadata(ctx, reportID)
	if err != nil {
		return err
	}
	if latest.ID == "" {
		latest = report
	}
	aiInput, err := BuildAIInput(latest, string(metadata), stackwalkText)
	if err != nil {
		return err
	}
	started := time.Now().UTC()
	if err := w.store.SetAIStatus(ctx, reportID, crashreports.AIAnalysisUpdate{Status: crashreports.AnalysisStatusRunning, Model: model, InputSHA256: aiInput.SHA256, StartedAt: started}); err != nil {
		return err
	}
	text, err := ai.Analyze(ctx, aiInput.Body)
	completed := time.Now().UTC()
	if err != nil {
		_ = w.store.SaveAIAnalysis(ctx, reportID, crashreports.AIAnalysisUpdate{Status: crashreports.AnalysisStatusFailed, Model: model, InputSHA256: aiInput.SHA256, Error: trimError(err), StartedAt: started, CompletedAt: completed})
		return err
	}
	return w.store.SaveAIAnalysis(ctx, reportID, crashreports.AIAnalysisUpdate{Status: crashreports.AnalysisStatusSucceeded, Model: model, InputSHA256: aiInput.SHA256, Text: text, StartedAt: started, CompletedAt: completed})
}

func trimError(err error) string {
	if err == nil {
		return ""
	}
	value := strings.TrimSpace(err.Error())
	if len(value) > 1024 {
		return value[:1024]
	}
	return value
}

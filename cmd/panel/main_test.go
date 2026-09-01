package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/crashreports"
)

type fakeShutdowner struct{ events *[]string }

func TestStartMinimumFreeBytesIsOneGiB(t *testing.T) {
	const want = uint64(1 << 30)
	if startMinimumFreeBytes != want {
		t.Fatalf("start minimum free bytes=%d want=%d", startMinimumFreeBytes, want)
	}
}

func TestPanelWebHandlerDisablesIndexCaching(t *testing.T) {
	webRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(webRoot, "index.html"), []byte("new entrypoint"), 0o600); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	newPanelWebHandler(webRoot).ServeHTTP(response, request)

	if got := response.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control=%q, want no-cache", got)
	}
	if got := response.Body.String(); got != "new entrypoint" {
		t.Fatalf("body=%q, want new entrypoint", got)
	}
}

func (f fakeShutdowner) Shutdown(context.Context) error {
	*f.events = append(*f.events, "http")
	return nil
}

type fakeJobWaiter struct{ events *[]string }

func (f fakeJobWaiter) Wait(context.Context) error {
	*f.events = append(*f.events, "jobs")
	return nil
}

type fakeSamplerStopper struct{ events *[]string }

type blockingEventLogger struct {
	started chan struct{}
	stopped chan struct{}
}

func (l blockingEventLogger) Run(ctx context.Context) {
	close(l.started)
	<-ctx.Done()
	close(l.stopped)
}

func TestStartA2SEventLoggerStopWaitsForExit(t *testing.T) {
	logger := blockingEventLogger{started: make(chan struct{}), stopped: make(chan struct{})}
	stop := startA2SEventLogger(context.Background(), logger)
	<-logger.started
	stop()
	select {
	case <-logger.stopped:
	case <-time.After(time.Second):
		t.Fatal("event logger did not stop")
	}
}

func (f fakeSamplerStopper) Stop(context.Context) error {
	*f.events = append(*f.events, "sampler")
	return nil
}

func TestShutdownPanelStopsHTTPThenSchedulerThenDrainsJobs(t *testing.T) {
	events := []string{}
	err := shutdownPanel(context.Background(), fakeShutdowner{events: &events}, func() { events = append(events, "scheduler") }, fakeSamplerStopper{events: &events}, fakeJobWaiter{events: &events})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(events, ","); got != "http,scheduler,sampler,jobs" {
		t.Fatalf("events=%s", got)
	}
}

type failingJobWaiter struct{ err error }

func (f failingJobWaiter) Wait(context.Context) error { return f.err }

func TestShutdownPanelReturnsDrainFailure(t *testing.T) {
	want := errors.New("drain timed out")
	if err := shutdownPanel(context.Background(), fakeShutdowner{events: &[]string{}}, func() {}, fakeSamplerStopper{events: &[]string{}}, failingJobWaiter{err: want}); !errors.Is(err, want) {
		t.Fatalf("shutdown error=%v", err)
	}
}

func TestShutdownPanelBoundsSchedulerStop(t *testing.T) {
	release := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- shutdownPanel(ctx, fakeShutdowner{events: &[]string{}}, func() { <-release }, fakeSamplerStopper{events: &[]string{}}, fakeJobWaiter{events: &[]string{}})
	}()
	select {
	case err := <-done:
		close(release)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("shutdown error=%v", err)
		}
	case <-time.After(100 * time.Millisecond):
		close(release)
		<-done
		t.Fatal("scheduler stop ignored shutdown deadline")
	}
}

type recordingCrashAnalysisEnqueuer struct {
	reportID  string
	requestAI bool
}

func (q *recordingCrashAnalysisEnqueuer) Enqueue(_ context.Context, reportID string, requestAI bool) error {
	q.reportID = reportID
	q.requestAI = requestAI
	return nil
}

func TestEnqueueCrashReportStackwalkDoesNotRequestAI(t *testing.T) {
	queue := &recordingCrashAnalysisEnqueuer{}
	report := crashreports.Report{ID: "report-id"}

	if err := enqueueCrashReportStackwalk(context.Background(), queue, report); err != nil {
		t.Fatal(err)
	}
	if queue.reportID != report.ID || queue.requestAI {
		t.Fatalf("queue=%+v want report=%q requestAI=false", queue, report.ID)
	}
}

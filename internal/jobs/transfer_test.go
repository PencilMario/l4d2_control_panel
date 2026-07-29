package jobs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
)

type advancingReader struct {
	clock *time.Time
	step  time.Duration
	data  []byte
	chunk int
	err   error
}

func (r *advancingReader) Read(buffer []byte) (int, error) {
	if len(r.data) == 0 {
		if r.err != nil {
			err := r.err
			r.err = nil
			return 0, err
		}
		return 0, io.EOF
	}
	*r.clock = r.clock.Add(r.step)
	count := r.chunk
	if count > len(r.data) {
		count = len(r.data)
	}
	copy(buffer, r.data[:count])
	r.data = r.data[count:]
	return count, nil
}

func transferContext(reporter Reporter) context.Context {
	return context.WithValue(context.Background(), reporterContextKey{}, reporter)
}

func TestTransferLogsKnownSizeAtFiveSecondIntervalsAndCompletion(t *testing.T) {
	now := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	reporter := &collectingReporter{}
	reader := &advancingReader{clock: &now, step: 3 * time.Second, data: []byte("abcdefghijkl"), chunk: 4}
	var destination bytes.Buffer
	written, err := CopyWithProgress(transferContext(reporter), &destination, reader, TransferOptions{
		Source: "github", Filename: "plugins.zip", Total: 12, Interval: 5 * time.Second, Now: func() time.Time { return now },
	})
	if err != nil || written != 12 || destination.String() != "abcdefghijkl" {
		t.Fatalf("written=%d destination=%q err=%v", written, destination.String(), err)
	}
	joined := strings.Join(reporter.messages, "\n")
	for _, want := range []string{"download started", "file=plugins.zip", "8 bytes/12 bytes", "66.7%", "rate=", "eta=", "download completed", "duration=9s"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("logs=%q missing %q", joined, want)
		}
	}
	if strings.Count(joined, "download progress") != 1 {
		t.Fatalf("logs=%q", joined)
	}
}

func TestTransferUnknownSizeOmitsPercentAndETA(t *testing.T) {
	now := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	reporter := &collectingReporter{}
	reader := &advancingReader{clock: &now, step: 5 * time.Second, data: []byte("abcd"), chunk: 4}
	_, err := CopyWithProgress(transferContext(reporter), io.Discard, reader, TransferOptions{
		Source: "github", Filename: "plugins.zip", Total: -1, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(reporter.messages, "\n")
	if strings.Contains(joined, "%") || strings.Contains(joined, "eta=") {
		t.Fatalf("logs=%q", joined)
	}
}

func TestTransferErrorIncludesTransferredBytes(t *testing.T) {
	now := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	want := errors.New("connection reset")
	reporter := &collectingReporter{}
	reader := &advancingReader{clock: &now, step: time.Second, data: []byte("abcd"), chunk: 4, err: want}
	written, err := CopyWithProgress(transferContext(reporter), io.Discard, reader, TransferOptions{
		Source: "github", Filename: "plugins.zip", Total: 12, Now: func() time.Time { return now },
	})
	if written != 4 || !errors.Is(err, want) || !strings.Contains(err.Error(), "4 bytes") {
		t.Fatalf("written=%d err=%v", written, err)
	}
}

func TestTransferOverLimitFailsWithoutCompletionLog(t *testing.T) {
	now := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	reporter := &collectingReporter{}
	reader := &advancingReader{clock: &now, step: time.Second, data: []byte("abcd"), chunk: 4}
	written, err := CopyWithProgress(transferContext(reporter), io.Discard, reader, TransferOptions{
		Source: "github", Filename: "plugins.zip", Total: 4, MaxBytes: 3, Now: func() time.Time { return now },
	})
	joined := strings.Join(reporter.messages, "\n")
	if written != 4 || err == nil || !strings.Contains(err.Error(), "size limit") || strings.Contains(joined, "download completed") {
		t.Fatalf("written=%d err=%v logs=%q", written, err, joined)
	}
}

type collectingReporter struct{ messages []string }

func (*collectingReporter) Progress(string, int, string) {}
func (r *collectingReporter) Log(_ string, _ joblogs.Level, message string) {
	r.messages = append(r.messages, message)
}

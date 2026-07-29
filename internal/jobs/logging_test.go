package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
)

type recordingReporter struct {
	source  string
	level   joblogs.Level
	message string
}

func (r *recordingReporter) Progress(string, int, string) {}
func (r *recordingReporter) Log(source string, level joblogs.Level, message string) {
	r.source, r.level, r.message = source, level, message
}

func TestFormatBytesIncludesHumanAndExactValues(t *testing.T) {
	if got := FormatBytes(1536); got != "1.50 KiB (1536 bytes)" {
		t.Fatalf("FormatBytes(1536)=%q", got)
	}
}

func TestFormatDurationUsesOperationalPrecision(t *testing.T) {
	if got := FormatDuration(1500 * time.Millisecond); got != "1.5s" {
		t.Fatalf("FormatDuration=%q", got)
	}
}

func TestLogfUsesTaskReporterAndNoopsWithoutOne(t *testing.T) {
	reporter := &recordingReporter{}
	ctx := context.WithValue(context.Background(), reporterContextKey{}, Reporter(reporter))
	Logf(ctx, "download", joblogs.Info, "file=%s bytes=%d", "asset.zip", 12)
	if reporter.source != "download" || reporter.level != joblogs.Info || reporter.message != "file=asset.zip bytes=12" {
		t.Fatalf("reporter=%#v", reporter)
	}
	Logf(context.Background(), "download", joblogs.Info, "ignored")
}

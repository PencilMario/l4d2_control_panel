package jobs

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
)

func Logf(ctx context.Context, source string, level joblogs.Level, format string, args ...any) {
	LogContext(ctx, source, level, fmt.Sprintf(format, args...))
}

func FormatBytes(value int64) string {
	const unit = int64(1024)
	if value < unit {
		return fmt.Sprintf("%d bytes", value)
	}
	divisor := unit
	label := "KiB"
	for _, candidate := range []string{"MiB", "GiB", "TiB"} {
		if value < divisor*unit {
			break
		}
		divisor *= unit
		label = candidate
	}
	return fmt.Sprintf("%.2f %s (%d bytes)", float64(value)/float64(divisor), label, value)
}

func FormatDuration(value time.Duration) string {
	if value < 0 {
		value = 0
	}
	return value.Round(time.Millisecond).String()
}

func SafeError(err error, hidden ...string) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	for _, value := range hidden {
		if value == "" {
			continue
		}
		message = strings.ReplaceAll(message, value, "<managed-root>")
		message = strings.ReplaceAll(message, filepath.ToSlash(value), "<managed-root>")
	}
	return message
}

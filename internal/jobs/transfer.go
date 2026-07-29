package jobs

import (
	"context"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
)

type TransferOptions struct {
	Source   string
	Filename string
	Total    int64
	Interval time.Duration
	Now      func() time.Time
}

func CopyWithProgress(ctx context.Context, destination io.Writer, source io.Reader, options TransferOptions) (int64, error) {
	if options.Source == "" {
		options.Source = "download"
	}
	if options.Interval <= 0 {
		options.Interval = 5 * time.Second
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	started := options.Now()
	lastReportedAt := started
	lastReportedBytes := int64(0)
	if options.Total > 0 {
		Logf(ctx, options.Source, joblogs.Info, "download started file=%s total=%s", options.Filename, FormatBytes(options.Total))
	} else {
		Logf(ctx, options.Source, joblogs.Info, "download started file=%s total=unknown", options.Filename)
	}

	buffer := make([]byte, 64*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, transferError(err, written)
		}
		read, readErr := source.Read(buffer)
		if read > 0 {
			count, writeErr := destination.Write(buffer[:read])
			written += int64(count)
			if writeErr != nil {
				return written, transferError(writeErr, written)
			}
			if count != read {
				return written, transferError(io.ErrShortWrite, written)
			}
			now := options.Now()
			if written > lastReportedBytes && now.Sub(lastReportedAt) >= options.Interval {
				logTransferProgress(ctx, options, written, written-lastReportedBytes, now.Sub(lastReportedAt))
				lastReportedAt, lastReportedBytes = now, written
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return written, transferError(readErr, written)
		}
	}
	Logf(ctx, options.Source, joblogs.Info, "download completed file=%s transferred=%s duration=%s", options.Filename, FormatBytes(written), FormatDuration(options.Now().Sub(started)))
	return written, nil
}

func logTransferProgress(ctx context.Context, options TransferOptions, written, sampleBytes int64, sampleDuration time.Duration) {
	rate := int64(0)
	if sampleDuration > 0 {
		rate = int64(math.Round(float64(sampleBytes) / sampleDuration.Seconds()))
	}
	if options.Total <= 0 {
		Logf(ctx, options.Source, joblogs.Info, "download progress file=%s transferred=%s rate=%s/s", options.Filename, FormatBytes(written), FormatBytes(rate))
		return
	}
	percent := math.Min(100, float64(written)*100/float64(options.Total))
	remaining := options.Total - written
	if remaining < 0 {
		remaining = 0
	}
	if rate > 0 {
		eta := time.Duration(float64(remaining) / float64(rate) * float64(time.Second))
		Logf(ctx, options.Source, joblogs.Info, "download progress file=%s transferred=%s/%s percent=%.1f%% rate=%s/s eta=%s", options.Filename, FormatBytes(written), FormatBytes(options.Total), percent, FormatBytes(rate), FormatDuration(eta))
		return
	}
	Logf(ctx, options.Source, joblogs.Info, "download progress file=%s transferred=%s/%s percent=%.1f%% rate=unavailable", options.Filename, FormatBytes(written), FormatBytes(options.Total), percent)
}

func transferError(err error, written int64) error {
	return fmt.Errorf("transfer failed after %s: %w", FormatBytes(written), err)
}

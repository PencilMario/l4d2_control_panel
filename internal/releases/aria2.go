package releases

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
)

type ConnectionSettings interface {
	ReleaseDownloadConnections() (int, error)
}

type CommandRunner interface {
	Run(context.Context, string, []string, string) error
}

type ExecCommandRunner struct{}

func (ExecCommandRunner) Run(ctx context.Context, name string, args []string, stdin string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = strings.NewReader(stdin)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	command.Env = proxyEnvironment(os.Environ())
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return nil
}

func proxyEnvironment(environment []string) []string {
	result := append([]string(nil), environment...)
	values := make(map[string]string, len(environment))
	for _, entry := range environment {
		key, value, found := strings.Cut(entry, "=")
		if found {
			values[key] = value
		}
	}
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"} {
		lower := strings.ToLower(key)
		if _, exists := values[lower]; !exists && values[key] != "" {
			result = append(result, lower+"="+values[key])
		}
	}
	return result
}

type Aria2Downloader struct {
	Runner   CommandRunner
	Settings ConnectionSettings
}

func (d Aria2Downloader) Download(ctx context.Context, request DownloadRequest) (int64, error) {
	if request.URL == "" || request.Destination == "" || request.MaxBytes < 1 {
		return 0, errors.New("valid download URL, destination and size limit are required")
	}
	connections, err := d.Settings.ReleaseDownloadConnections()
	if err != nil {
		return 0, err
	}
	if connections < 1 || connections > 16 {
		return 0, errors.New("download connections must be between 1 and 16")
	}
	runner := d.Runner
	if runner == nil {
		runner = ExecCommandRunner{}
	}
	if err := os.Remove(request.Destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	defer os.Remove(request.Destination + ".aria2")
	directory, output := filepath.Split(request.Destination)
	args := []string{
		"--input-file=-",
		"--dir=" + filepath.Clean(directory),
		"--out=" + output,
		"--split=" + strconv.Itoa(connections),
		"--max-connection-per-server=" + strconv.Itoa(connections),
		"--min-split-size=1M",
		"--continue=true",
		"--allow-overwrite=true",
		"--auto-file-renaming=false",
		"--file-allocation=none",
		"--console-log-level=warn",
		"--summary-interval=0",
		"--download-result=hide",
	}
	jctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runner.Run(jctx, "aria2c", args, request.URL+"\n") }()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	var previous int64
	var previousAt = time.Now()
	for {
		select {
		case runErr := <-done:
			if runErr != nil {
				_ = os.Remove(request.Destination)
				return 0, runErr
			}
			info, statErr := os.Stat(request.Destination)
			if statErr != nil {
				return 0, fmt.Errorf("aria2 output unavailable: %w", statErr)
			}
			if info.Size() > request.MaxBytes {
				_ = os.Remove(request.Destination)
				return 0, errors.New("release asset exceeds size limit")
			}
			return info.Size(), nil
		case now := <-ticker.C:
			info, statErr := os.Stat(request.Destination)
			if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
				cancel()
				<-done
				return 0, statErr
			}
			var size int64
			if statErr == nil {
				size = info.Size()
			}
			if size > request.MaxBytes {
				cancel()
				<-done
				_ = os.Remove(request.Destination)
				return 0, errors.New("release asset exceeds size limit")
			}
			elapsed := now.Sub(previousAt).Seconds()
			rate := float64(size-previous) / elapsed
			jobs.Logf(ctx, "github", joblogs.Info, "download progress source=github file=%s bytes=%d total=%d rate=%s/s connections=%d", request.Filename, size, request.Total, jobs.FormatBytes(int64(rate)), connections)
			previous, previousAt = size, now
		case <-ctx.Done():
			cancel()
			<-done
			_ = os.Remove(request.Destination)
			return 0, ctx.Err()
		}
	}
}

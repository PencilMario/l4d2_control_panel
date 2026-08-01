package releases

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeConnections struct {
	value int
	err   error
}

func (f fakeConnections) ReleaseDownloadConnections() (int, error) { return f.value, f.err }

type fakeCommandRunner struct {
	name  string
	args  []string
	stdin string
	run   func([]string) error
}

func (f *fakeCommandRunner) Run(_ context.Context, name string, args []string, stdin string) error {
	f.name, f.args, f.stdin = name, append([]string(nil), args...), stdin
	if f.run != nil {
		return f.run(args)
	}
	return nil
}

func TestAria2DownloaderUsesCurrentConnectionsAndKeepsURLOutOfArguments(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "release.part")
	runner := &fakeCommandRunner{run: func(args []string) error {
		return os.WriteFile(destination, []byte("payload"), 0600)
	}}
	downloader := Aria2Downloader{Runner: runner, Settings: fakeConnections{value: 8}}
	url := "https://objects.githubusercontent.com/asset?token=signed-secret"
	written, err := downloader.Download(context.Background(), DownloadRequest{URL: url, Destination: destination, Total: 7, MaxBytes: 1024})
	if err != nil || written != 7 {
		t.Fatalf("written=%d err=%v", written, err)
	}
	joined := strings.Join(runner.args, " ")
	for _, want := range []string{"--split=8", "--max-connection-per-server=8", "--out=release.part"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
	if strings.Contains(joined, url) || strings.Contains(joined, "signed-secret") {
		t.Fatalf("URL leaked into args: %q", joined)
	}
	if runner.stdin != url+"\n" {
		t.Fatalf("stdin=%q", runner.stdin)
	}
}

func TestAria2DownloaderRejectsOversizedOutputAndRemovesIt(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "release.part")
	runner := &fakeCommandRunner{run: func([]string) error {
		return os.WriteFile(destination, []byte("too large"), 0600)
	}}
	downloader := Aria2Downloader{Runner: runner, Settings: fakeConnections{value: 4}}
	_, err := downloader.Download(context.Background(), DownloadRequest{URL: "https://github.com/asset", Destination: destination, MaxBytes: 4})
	if err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("err=%v", err)
	}
	if _, statErr := os.Stat(destination); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("output retained: %v", statErr)
	}
}

func TestAria2DownloaderPropagatesSettingsAndCommandFailures(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "release.part")
	settingsErr := errors.New("settings unavailable")
	_, err := (Aria2Downloader{Runner: &fakeCommandRunner{}, Settings: fakeConnections{err: settingsErr}}).Download(context.Background(), DownloadRequest{URL: "https://github.com/asset", Destination: destination, MaxBytes: 10})
	if !errors.Is(err, settingsErr) {
		t.Fatalf("settings err=%v", err)
	}
	commandErr := errors.New("aria2 failed")
	runner := &fakeCommandRunner{run: func([]string) error { return commandErr }}
	_, err = (Aria2Downloader{Runner: runner, Settings: fakeConnections{value: 2}}).Download(context.Background(), DownloadRequest{URL: "https://github.com/asset", Destination: destination, MaxBytes: 10})
	if !errors.Is(err, commandErr) {
		t.Fatalf("command err=%v", err)
	}
}

func TestAria2DownloaderCancelsCommandAndRemovesPartialOutput(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "release.part")
	runner := &fakeCommandRunner{run: func([]string) error {
		return context.Canceled
	}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (Aria2Downloader{Runner: runner, Settings: fakeConnections{value: 8}}).Download(ctx, DownloadRequest{URL: "https://github.com/asset", Destination: destination, MaxBytes: 10})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if _, statErr := os.Stat(destination); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("partial output retained: %v", statErr)
	}
}

func TestProxyEnvironmentAddsLowercaseAliasesForAria2(t *testing.T) {
	environment := proxyEnvironment([]string{"PATH=/bin", "HTTP_PROXY=http://proxy:8080", "HTTPS_PROXY=https://proxy:8443"})
	joined := strings.Join(environment, "\n")
	for _, want := range []string{"http_proxy=http://proxy:8080", "https_proxy=https://proxy:8443"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("environment %q missing %q", joined, want)
		}
	}
}

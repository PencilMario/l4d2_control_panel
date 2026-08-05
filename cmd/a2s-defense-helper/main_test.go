package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/a2sdefense"
)

type retryingEventSource struct {
	calls atomic.Int32
}

func (s *retryingEventSource) Run(ctx context.Context) error {
	if s.calls.Add(1) == 1 {
		return errors.New("listener unavailable")
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestRunEventSourceRetriesWithoutOwningServerLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	source := &retryingEventSource{}
	errorsSeen := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		runEventSource(ctx, source, time.Millisecond, func(err error) { errorsSeen <- err })
		close(done)
	}()
	select {
	case err := <-errorsSeen:
		if err == nil || source.calls.Load() != 1 {
			t.Fatalf("err=%v calls=%d", err, source.calls.Load())
		}
	case <-time.After(time.Second):
		t.Fatal("listener failure not reported")
	}
	for deadline := time.Now().Add(time.Second); source.calls.Load() < 2 && time.Now().Before(deadline); {
		time.Sleep(time.Millisecond)
	}
	if source.calls.Load() < 2 {
		t.Fatal("listener was not retried")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("listener retry loop did not stop")
	}
}

type stubFirewall struct{}

func (stubFirewall) Apply(_ context.Context, config a2sdefense.Config) (a2sdefense.Status, error) {
	return a2sdefense.Status{Compatible: true, Enabled: config.Enabled, Revision: config.Revision}, nil
}
func (stubFirewall) Status(context.Context) (a2sdefense.Status, error) {
	return a2sdefense.Status{Compatible: true}, nil
}

func TestServeCreatesRestrictedSocketAndStopsWithContext(t *testing.T) {
	socket := filepath.Join(os.TempDir(), fmt.Sprintf("a2s-main-%d.sock", os.Getpid()))
	_ = os.Remove(socket)
	t.Cleanup(func() { _ = os.Remove(socket) })
	if err := os.WriteFile(socket, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, socket, stubFirewall{}) }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		connection, err := net.Dial("unix", socket)
		if err == nil {
			connection.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("socket did not become ready: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	info, err := os.Stat(socket)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o660 {
		t.Fatalf("socket mode=%o", info.Mode().Perm())
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("serve did not stop")
	}
}

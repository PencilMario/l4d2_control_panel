package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/a2sdefense"
)

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

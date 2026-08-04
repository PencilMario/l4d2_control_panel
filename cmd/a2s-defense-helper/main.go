package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/a2sdefense"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	socket := os.Getenv("A2S_DEFENSE_HELPER_SOCKET")
	if socket == "" {
		socket = "/run/l4d2-panel/a2s-defense.sock"
	}
	manager := a2sdefense.NewManager(a2sdefense.CommandExecutor{}, time.Now)
	if err := serve(ctx, socket, manager); err != nil {
		log.Fatal(err)
	}
}

func serve(ctx context.Context, socketPath string, firewall a2sdefense.Firewall) error {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o750); err != nil {
		return err
	}
	if err := os.Remove(socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := os.Chmod(socketPath, 0o660); err != nil {
		return err
	}
	server := &http.Server{Handler: a2sdefense.NewServer(firewall), ReadHeaderTimeout: 10 * time.Second}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdownCtx)
		case <-done:
		}
	}()
	err = server.Serve(listener)
	close(done)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

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

	"github.com/not0721here/l4d2-control-panel/internal/overlayfs"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	root := os.Getenv("L4D2_PANEL_DATA_ROOT")
	if root == "" {
		root = "/srv/l4d2-panel"
	}
	socketPath := os.Getenv("OVERLAY_HELPER_SOCKET")
	if socketPath == "" {
		socketPath = "/run/l4d2-panel/overlay.sock"
	}
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
	server := &http.Server{Handler: overlayfs.NewServer(overlayfs.Paths{Root: root}, overlayfs.SystemMounter{}), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	return server.Serve(listener)
}

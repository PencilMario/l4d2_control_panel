package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/socketproxy"
	"github.com/not0721here/l4d2-control-panel/internal/traffic"
)

type captureStatus struct {
	mu  sync.RWMutex
	err error
}

func (s *captureStatus) set(err error) {
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()
}

func (s *captureStatus) unavailable() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.err
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	socket := os.Getenv("DOCKER_SOCKET")
	if socket == "" {
		socket = "/var/run/docker.sock"
	}
	target, _ := url.Parse("http://docker")
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", socket)
	}}

	counter := traffic.NewCounter()
	status := &captureStatus{err: errors.New("traffic capture is starting")}
	go superviseCapture(ctx, counter, status, traffic.StartCapture, time.Second, log.Printf)
	trafficHandler := traffic.NewHandler(counter, status.unavailable)
	handler := newProxyHandler(trafficHandler, proxy)

	socketPath := os.Getenv("SOCKET_PATH")
	if socketPath == "" {
		socketPath = "/run/l4d2-panel/proxy.sock"
	}
	listener, err := listenUnix(socketPath)
	if err != nil {
		return err
	}
	defer func() {
		closeUnixListener(listener)
	}()
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	log.Printf("restricted Docker proxy listening on unix://%s", socketPath)
	return server.Serve(listener)
}

func closeUnixListener(listener net.Listener) {
	_ = listener.Close()
}

func listenUnix(socketPath string) (net.Listener, error) {
	parent := filepath.Dir(socketPath)
	if err := os.MkdirAll(parent, 0750); err != nil {
		return nil, err
	}
	if err := chownPanel(parent); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(socketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("socket path exists and is not a socket: %s", socketPath)
		}
		if err := os.Remove(socketPath); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socketPath, 0660); err != nil {
		listener.Close()
		return nil, err
	}
	if err := chownPanel(socketPath); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}

func chownPanel(path string) error {
	err := os.Chown(path, 10001, 10001)
	if runtime.GOOS == "windows" {
		return nil
	}
	return err
}

func newProxyHandler(trafficHandler, dockerProxy http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/_panel/traffic/") {
			trafficHandler.ServeHTTP(w, r)
			return
		}
		if !socketproxy.Allowed(r.Method, r.URL.Path) {
			http.Error(w, "docker endpoint forbidden", http.StatusForbidden)
			return
		}
		dockerProxy.ServeHTTP(w, r)
	})
}

func superviseCapture(
	ctx context.Context,
	observer traffic.Observer,
	status *captureStatus,
	starter func(context.Context, traffic.Observer) (<-chan error, error),
	retryDelay time.Duration,
	logf func(string, ...any),
) {
	for {
		captureErrors, err := starter(ctx, observer)
		if err == nil {
			status.set(nil)
			select {
			case <-ctx.Done():
				return
			case captureErr, ok := <-captureErrors:
				if !ok || captureErr == nil {
					captureErr = errors.New("traffic capture stopped unexpectedly")
				}
				err = captureErr
			}
		}
		status.set(err)
		logf("traffic capture unavailable: %v", err)
		timer := time.NewTimer(retryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

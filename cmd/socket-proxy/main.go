package main

import (
	"context"
	"encoding/json"
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
	dockerTransport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", socket)
	}}
	proxy.Transport = dockerTransport
	dockerClient := &http.Client{Transport: dockerTransport, Timeout: 5 * time.Second}

	counter := traffic.NewCounter()
	status := &captureStatus{err: errors.New("traffic capture is starting")}
	go superviseCapture(ctx, counter, status, traffic.StartCapture, time.Second, log.Printf)
	trafficHandler := traffic.NewHandler(counter, status.unavailable)
	handler := newProxyHandler(trafficHandler, proxy, func(ctx context.Context, id string) (map[string]string, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/v1.44/containers/"+url.PathEscape(id)+"/json", nil)
		if err != nil {
			return nil, err
		}
		response, err := dockerClient.Do(request)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("inspect container: %s", response.Status)
		}
		var body struct {
			Config struct {
				Labels map[string]string `json:"Labels"`
			} `json:"Config"`
		}
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			return nil, err
		}
		return body.Config.Labels, nil
	})

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
	if err := securePath(parent, 0750, 0, 10001); err != nil {
		return nil, err
	}
	if err := prepareSocketPath(socketPath, dialUnixSocket); err != nil {
		return nil, err
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	if err := securePath(socketPath, 0660, 0, 10001); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}

func prepareSocketPath(socketPath string, dial func(string) (net.Conn, error)) error {
	info, err := os.Lstat(socketPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("socket path exists and is not a socket: %s", socketPath)
	}
	conn, err := dial(socketPath)
	if err == nil {
		_ = conn.Close()
		return fmt.Errorf("socket path already has an active listener: %s", socketPath)
	}
	if !isConnectionRefused(err) {
		return fmt.Errorf("cannot safely determine whether socket is stale: %w", err)
	}
	return os.Remove(socketPath)
}

func isConnectionRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED) || (runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(10061)))
}

func dialUnixSocket(socketPath string) (net.Conn, error) {
	return net.DialTimeout("unix", socketPath, 250*time.Millisecond)
}

func securePath(path string, mode os.FileMode, uid, gid int) error {
	return securePathWith(path, mode, uid, gid, pathOwnership, os.Chown)
}

type ownershipFunc func(string) (uid, gid int, known bool, err error)

func securePathWith(path string, mode os.FileMode, uid, gid int, ownership ownershipFunc, chown func(string, int, int) error) error {
	if err := os.Chmod(path, mode); err != nil {
		return err
	}
	currentUID, currentGID, known, err := ownership(path)
	if err != nil {
		return err
	}
	if known && currentUID == uid && currentGID == gid {
		return nil
	}
	err = chown(path, uid, gid)
	if runtime.GOOS == "windows" {
		return nil
	}
	return err
}

func newProxyHandler(trafficHandler, dockerProxy http.Handler, labels func(context.Context, string) (map[string]string, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/_panel/traffic/") {
			trafficHandler.ServeHTTP(w, r)
			return
		}
		if !socketproxy.Allowed(r.Method, r.URL.Path) {
			http.Error(w, "docker endpoint forbidden", http.StatusForbidden)
			return
		}
		if id, isLogs := socketproxy.LogContainerID(r.URL.Path); isLogs {
			if !socketproxy.AllowedLogQuery(r.URL.Query()) {
				http.Error(w, "docker logs query forbidden", http.StatusForbidden)
				return
			}
			containerLabels, err := labels(r.Context(), id)
			if err != nil || containerLabels["io.l4d2-panel.managed"] != "true" || containerLabels["io.l4d2-panel.role"] != "maintenance" {
				http.Error(w, "docker container logs forbidden", http.StatusForbidden)
				return
			}
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

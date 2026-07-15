package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
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
	go superviseCapture(context.Background(), counter, status, traffic.StartCapture, time.Second, log.Printf)
	trafficHandler := traffic.NewHandler(counter, status.unavailable)
	handler := newProxyHandler(trafficHandler, proxy)

	listen := os.Getenv("LISTEN_ADDR")
	if listen == "" {
		listen = "127.0.0.1:23750"
	}
	server := &http.Server{Addr: listen, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	log.Printf("restricted Docker proxy listening on %s", listen)
	log.Fatal(server.ListenAndServe())
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

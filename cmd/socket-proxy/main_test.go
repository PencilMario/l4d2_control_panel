package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/traffic"
)

func TestSocketListenerRejectsRegularFileAndSetsMode(t *testing.T) {
	path := filepath.Join(os.TempDir(), fmt.Sprintf("l4d2-panel-%d.sock", os.Getpid()))
	t.Cleanup(func() { _ = os.Remove(path) })
	_ = os.Remove(path)
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := listenUnix(path); err == nil {
		t.Fatal("expected regular file rejection")
	}
	_ = os.Remove(path)
	ln, err := listenUnix(path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0660 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	ln, err = listenUnix(path)
	if err != nil {
		t.Fatalf("replace stale socket: %v", err)
	}
}

func TestProxyHandlerRoutesTrafficBeforeDockerPolicy(t *testing.T) {
	counter := traffic.NewCounter()
	trafficHandler := traffic.NewHandler(counter, nil)
	dockerCalls := 0
	dockerProxy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dockerCalls++
		w.WriteHeader(http.StatusTeapot)
	})
	handler := newProxyHandler(trafficHandler, dockerProxy)

	req := httptest.NewRequest(http.MethodPut, "/_panel/traffic/instance-1", http.NoBody)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code == http.StatusForbidden {
		t.Fatal("internal traffic route was incorrectly sent through Docker policy")
	}
	if dockerCalls != 0 {
		t.Fatalf("Docker proxy called %d times for internal route", dockerCalls)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1.44/volumes", nil)
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("forbidden Docker route status = %d, want 403", res.Code)
	}
	if dockerCalls != 0 {
		t.Fatalf("Docker proxy called %d times for forbidden route", dockerCalls)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1.44/info", nil)
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusTeapot || dockerCalls != 1 {
		t.Fatalf("allowed Docker route status/calls = %d/%d", res.Code, dockerCalls)
	}
}

func TestSuperviseCaptureRetriesAndRestoresAvailability(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	status := &captureStatus{err: errors.New("starting")}
	var attempts atomic.Int32
	started := make(chan struct{})
	starter := func(context.Context, traffic.Observer) (<-chan error, error) {
		if attempts.Add(1) == 1 {
			return nil, errors.New("temporary capture failure")
		}
		select {
		case <-started:
		default:
			close(started)
		}
		return make(chan error), nil
	}
	go superviseCapture(ctx, traffic.NewCounter(), status, starter, time.Millisecond, func(string, ...any) {})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("capture supervisor did not retry")
	}
	waitCtx, waitCancel := context.WithTimeout(ctx, time.Second)
	defer waitCancel()
	waitFor(t, waitCtx, func() bool { return status.unavailable() == nil })
}

func waitFor(t *testing.T, ctx context.Context, condition func() bool) {
	t.Helper()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("condition was not met: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}

package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeShutdowner struct{ events *[]string }

func (f fakeShutdowner) Shutdown(context.Context) error {
	*f.events = append(*f.events, "http")
	return nil
}

type fakeJobWaiter struct{ events *[]string }

func (f fakeJobWaiter) Wait(context.Context) error {
	*f.events = append(*f.events, "jobs")
	return nil
}

func TestShutdownPanelStopsHTTPThenSchedulerThenDrainsJobs(t *testing.T) {
	events := []string{}
	err := shutdownPanel(context.Background(), fakeShutdowner{events: &events}, func() { events = append(events, "scheduler") }, fakeJobWaiter{events: &events})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(events, ","); got != "http,scheduler,jobs" {
		t.Fatalf("events=%s", got)
	}
}

type failingJobWaiter struct{ err error }

func (f failingJobWaiter) Wait(context.Context) error { return f.err }

func TestShutdownPanelReturnsDrainFailure(t *testing.T) {
	want := errors.New("drain timed out")
	if err := shutdownPanel(context.Background(), fakeShutdowner{events: &[]string{}}, func() {}, failingJobWaiter{err: want}); !errors.Is(err, want) {
		t.Fatalf("shutdown error=%v", err)
	}
}

func TestShutdownPanelBoundsSchedulerStop(t *testing.T) {
	release := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- shutdownPanel(ctx, fakeShutdowner{events: &[]string{}}, func() { <-release }, fakeJobWaiter{events: &[]string{}})
	}()
	select {
	case err := <-done:
		close(release)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("shutdown error=%v", err)
		}
	case <-time.After(100 * time.Millisecond):
		close(release)
		<-done
		t.Fatal("scheduler stop ignored shutdown deadline")
	}
}

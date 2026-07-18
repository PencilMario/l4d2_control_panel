package maintenance

import (
	"context"
	"testing"
	"time"
)

func TestGateExclusiveWaitsForSharedAndBlocksNewShared(t *testing.T) {
	gate := NewGate()
	releaseShared, err := gate.Shared(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	exclusiveReady := make(chan func(), 1)
	go func() {
		release, acquireErr := gate.Exclusive(context.Background())
		if acquireErr == nil {
			exclusiveReady <- release
		}
	}()
	select {
	case <-exclusiveReady:
		t.Fatal("exclusive acquired while shared lease active")
	case <-time.After(20 * time.Millisecond):
	}
	sharedReady := make(chan func(), 1)
	go func() {
		release, acquireErr := gate.Shared(context.Background())
		if acquireErr == nil {
			sharedReady <- release
		}
	}()
	releaseShared()
	var releaseExclusive func()
	select {
	case releaseExclusive = <-exclusiveReady:
	case <-time.After(time.Second):
		t.Fatal("exclusive did not acquire")
	}
	select {
	case <-sharedReady:
		t.Fatal("new shared lease crossed exclusive lease")
	case <-time.After(20 * time.Millisecond):
	}
	releaseExclusive()
	select {
	case release := <-sharedReady:
		release()
	case <-time.After(time.Second):
		t.Fatal("shared did not resume")
	}
}

func TestGateAcquisitionHonorsCancellation(t *testing.T) {
	gate := NewGate()
	release, err := gate.Exclusive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := gate.Shared(ctx); err == nil {
		t.Fatal("cancelled acquisition succeeded")
	}
}

func TestGateContextLeaseAllowsNestedAcquisition(t *testing.T) {
	gate := NewGate()
	ctx, release, err := gate.ExclusiveContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	_, nestedRelease, err := gate.SharedContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	nestedRelease()
}

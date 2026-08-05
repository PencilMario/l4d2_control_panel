package a2sdefense

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"testing"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

type eventClientStub struct {
	batches []EventBatch
	calls   []struct {
		boot  string
		after uint64
	}
}

func (s *eventClientStub) Events(_ context.Context, boot string, after uint64) (EventBatch, error) {
	s.calls = append(s.calls, struct {
		boot  string
		after uint64
	}{boot, after})
	batch := s.batches[0]
	s.batches = s.batches[1:]
	return batch, nil
}

type instanceStoreStub struct{ instances []domain.Instance }

func (s instanceStoreStub) Instances(context.Context) ([]domain.Instance, error) {
	return slices.Clone(s.instances), nil
}

type eventSinkStub struct {
	events map[string][]Event
	losses map[string][]uint64
	fail   string
}

func (s *eventSinkStub) AppendA2SDefense(_ context.Context, id string, event Event) error {
	if id == s.fail {
		return errors.New("write failed")
	}
	if s.events == nil {
		s.events = map[string][]Event{}
	}
	s.events[id] = append(s.events[id], event)
	return nil
}

func (s *eventSinkStub) AppendA2SDefenseLoss(_ context.Context, id string, lost uint64) error {
	if s.losses == nil {
		s.losses = map[string][]uint64{}
	}
	s.losses[id] = append(s.losses[id], lost)
	return nil
}

func TestEventLoggerFansOutOnlyGameAndSourceTVPorts(t *testing.T) {
	event := Event{Sequence: 7, Timestamp: time.Now(), Source: netip.MustParseAddr("203.0.113.8"), DestinationPort: 27015, Query: QueryPlayer, Action: "DROP"}
	client := &eventClientStub{batches: []EventBatch{{BootID: "boot-1", LatestSequence: 7, Events: []Event{event}}}}
	store := instanceStoreStub{instances: []domain.Instance{
		{ID: "game", GamePort: 27015, ActualState: domain.StateRunning},
		{ID: "tv", GamePort: 27016, SourceTVPort: 27015, ActualState: domain.StateRunning},
		{ID: "plugin", GamePort: 27017, PluginPorts: []int{27015}, ActualState: domain.StateRunning},
		{ID: "stopped", GamePort: 27015, ActualState: domain.StateStopped},
	}}
	sink := &eventSinkStub{}
	logger := NewEventLogger(client, store, sink, time.Millisecond, nil)
	if err := logger.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sink.events["game"]) != 1 || len(sink.events["tv"]) != 1 || len(sink.events) != 2 {
		t.Fatalf("events=%v", sink.events)
	}
}

func TestEventLoggerAdvancesAcrossFailureAndReportsLoss(t *testing.T) {
	event := Event{Sequence: 9, Timestamp: time.Now(), Source: netip.MustParseAddr("192.0.2.1"), DestinationPort: 27015, Query: QueryInfo, Action: "DROP"}
	client := &eventClientStub{batches: []EventBatch{
		{BootID: "boot-2", LatestSequence: 9, Lost: 3, Restarted: true, Events: []Event{event}},
		{BootID: "boot-2", LatestSequence: 9},
	}}
	store := instanceStoreStub{instances: []domain.Instance{{ID: "ok", GamePort: 27015, ActualState: domain.StateRunning}, {ID: "bad", GamePort: 27015, ActualState: domain.StateRunning}}}
	sink := &eventSinkStub{fail: "bad"}
	logger := NewEventLogger(client, store, sink, time.Millisecond, nil)
	if err := logger.Poll(context.Background()); err == nil {
		t.Fatal("expected surfaced write error")
	}
	if err := logger.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sink.events["ok"]) != 1 || len(sink.losses["ok"]) != 1 || sink.losses["ok"][0] != 3 {
		t.Fatalf("events=%v losses=%v", sink.events, sink.losses)
	}
	if got := client.calls[1]; got.boot != "boot-2" || got.after != 9 {
		t.Fatalf("cursor=%+v", got)
	}
}

func TestEventLoggerRunStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	logger := NewEventLogger(&eventClientStub{}, instanceStoreStub{}, &eventSinkStub{}, time.Millisecond, nil)
	logger.Run(ctx)
}

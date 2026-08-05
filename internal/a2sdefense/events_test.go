package a2sdefense

import (
	"net/netip"
	"testing"
	"time"
)

func TestEventRingReturnsCursorBatchesAndImmutableCopies(t *testing.T) {
	ring := NewEventRing("boot-a", 3)
	now := time.Date(2026, 8, 5, 7, 42, 10, 0, time.UTC)
	for index, query := range []QueryType{QueryInfo, QueryPlayer, QueryRules} {
		ring.Add(Event{Timestamp: now.Add(time.Duration(index) * time.Second), Source: netip.MustParseAddr("192.0.2.1"), DestinationPort: 27015 + index, Query: query})
	}
	batch := ring.Batch("boot-a", 1)
	if batch.BootID != "boot-a" || batch.OldestSequence != 1 || batch.LatestSequence != 3 || batch.Lost != 0 || len(batch.Events) != 2 || batch.Events[0].Sequence != 2 {
		t.Fatalf("batch=%+v", batch)
	}
	batch.Events[0].DestinationPort = 1
	again := ring.Batch("boot-a", 1)
	if again.Events[0].DestinationPort != 27016 {
		t.Fatal("caller mutated ring event")
	}
}

func TestEventRingReportsOverwriteAndBootChange(t *testing.T) {
	ring := NewEventRing("boot-b", 2)
	for index := 0; index < 4; index++ {
		ring.Add(Event{Source: netip.MustParseAddr("198.51.100.2"), DestinationPort: 27015, Query: QueryPlayer})
	}
	batch := ring.Batch("boot-b", 1)
	if batch.OldestSequence != 3 || batch.LatestSequence != 4 || batch.Lost != 1 || len(batch.Events) != 2 {
		t.Fatalf("overflow batch=%+v", batch)
	}
	restarted := ring.Batch("old-boot", 99)
	if !restarted.Restarted || restarted.Lost != 0 || len(restarted.Events) != 2 {
		t.Fatalf("restart batch=%+v", restarted)
	}
}

func TestEventRingRejectsInvalidEventsAndUsesFixedAction(t *testing.T) {
	ring := NewEventRing("boot", 2)
	for _, event := range []Event{
		{Source: netip.Addr{}, DestinationPort: 27015, Query: QueryInfo},
		{Source: netip.MustParseAddr("2001:db8::1"), DestinationPort: 27015, Query: QueryInfo},
		{Source: netip.MustParseAddr("192.0.2.1"), DestinationPort: 0, Query: QueryInfo},
		{Source: netip.MustParseAddr("192.0.2.1"), DestinationPort: 27015, Query: QueryType("bad")},
	} {
		if err := ring.Add(event); err == nil {
			t.Fatalf("accepted invalid event: %+v", event)
		}
	}
	if err := ring.Add(Event{Source: netip.MustParseAddr("192.0.2.1"), DestinationPort: 27015, Query: QueryChallenge}); err != nil {
		t.Fatal(err)
	}
	if got := ring.Batch("boot", 0).Events[0].Action; got != "DROP" {
		t.Fatalf("action=%q", got)
	}
}

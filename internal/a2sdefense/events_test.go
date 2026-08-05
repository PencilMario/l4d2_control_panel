package a2sdefense

import (
	"encoding/binary"
	"net/netip"
	"testing"
	"time"
)

func TestParseNFLogPacketExtractsIPv4UDPQuery(t *testing.T) {
	for opcode, query := range map[byte]QueryType{0x54: QueryInfo, 0x55: QueryPlayer, 0x56: QueryRules, 0x57: QueryChallenge, 0x69: QueryOther69} {
		packet := make([]byte, 20+8+5)
		packet[0] = 0x45
		binary.BigEndian.PutUint16(packet[2:4], uint16(len(packet)))
		packet[9] = 17
		copy(packet[12:16], []byte{192, 0, 2, 9})
		copy(packet[16:20], []byte{198, 51, 100, 1})
		binary.BigEndian.PutUint16(packet[20:22], 40000)
		binary.BigEndian.PutUint16(packet[22:24], 27015)
		binary.BigEndian.PutUint16(packet[24:26], 13)
		copy(packet[28:], []byte{0xff, 0xff, 0xff, 0xff, opcode})
		event, err := ParseNFLogPacket(packet, time.Date(2026, 8, 5, 8, 0, 0, 0, time.UTC))
		if err != nil || event.Source.String() != "192.0.2.9" || event.DestinationPort != 27015 || event.Query != query {
			t.Fatalf("opcode=%x event=%+v err=%v", opcode, event, err)
		}
	}
}

func TestParseNFLogPacketRejectsMalformedOrUnsupportedPackets(t *testing.T) {
	valid := make([]byte, 33)
	valid[0] = 0x45
	binary.BigEndian.PutUint16(valid[2:4], uint16(len(valid)))
	valid[9] = 17
	copy(valid[12:16], []byte{192, 0, 2, 1})
	binary.BigEndian.PutUint16(valid[22:24], 27015)
	copy(valid[28:], []byte{0xff, 0xff, 0xff, 0xff, 0x55})
	inputs := [][]byte{
		nil,
		valid[:19],
		append([]byte{0x65}, valid[1:]...),
		func() []byte { value := append([]byte(nil), valid...); value[9] = 6; return value }(),
		func() []byte { value := append([]byte(nil), valid...); value[28] = 0; return value }(),
		func() []byte { value := append([]byte(nil), valid...); value[32] = 0x53; return value }(),
	}
	for _, input := range inputs {
		if _, err := ParseNFLogPacket(input, time.Now()); err == nil {
			t.Fatalf("accepted packet %x", input)
		}
	}
}

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

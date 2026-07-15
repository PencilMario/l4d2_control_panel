package traffic

import (
	"encoding/binary"
	"sync"
	"testing"
)

func TestCounterAttributesDeclaredPortsOncePerDirection(t *testing.T) {
	c := NewCounter()
	if err := c.Register(Session{InstanceID: "instance-1", RunID: "run-1", Ports: []int{27015, 27015, 27016}}); err != nil {
		t.Fatal(err)
	}

	c.Observe(Packet{Length: 100, SrcPort: 27015, DstPort: 27016})
	c.Observe(Packet{Length: 50, SrcPort: 1234, DstPort: 27015})
	c.Observe(Packet{Length: 25, SrcPort: 27016, DstPort: 1234})
	c.Observe(Packet{Length: 999, SrcPort: 1234, DstPort: 4321})

	got, ok := c.Totals("instance-1")
	if !ok {
		t.Fatal("registered instance not found")
	}
	want := Totals{RunID: "run-1", RXBytes: 150, TXBytes: 125}
	if got != want {
		t.Fatalf("Totals() = %+v, want %+v", got, want)
	}
}

func TestCounterRejectsInvalidRegistration(t *testing.T) {
	tests := []Session{
		{},
		{InstanceID: "../escape", RunID: "run-1", Ports: []int{27015}},
		{InstanceID: "instance-1", RunID: "bad/run", Ports: []int{27015}},
		{InstanceID: "instance-1", RunID: "run-1"},
		{InstanceID: "instance-1", RunID: "run-1", Ports: []int{0}},
		{InstanceID: "instance-1", RunID: "run-1", Ports: []int{65536}},
	}
	for _, session := range tests {
		if err := NewCounter().Register(session); err == nil {
			t.Fatalf("Register(%+v) succeeded, want error", session)
		}
	}
}

func TestCounterSameRunPreservesTotalsAndNewRunResets(t *testing.T) {
	c := NewCounter()
	mustRegister(t, c, Session{InstanceID: "instance-1", RunID: "run-1", Ports: []int{27015}})
	c.Observe(Packet{Length: 100, DstPort: 27015})
	mustRegister(t, c, Session{InstanceID: "instance-1", RunID: "run-1", Ports: []int{27016}})
	c.Observe(Packet{Length: 50, DstPort: 27015})
	c.Observe(Packet{Length: 25, DstPort: 27016})
	got, _ := c.Totals("instance-1")
	if got.RXBytes != 125 {
		t.Fatalf("same-run total = %d, want 125", got.RXBytes)
	}

	mustRegister(t, c, Session{InstanceID: "instance-1", RunID: "run-2", Ports: []int{27016}})
	got, _ = c.Totals("instance-1")
	if got != (Totals{RunID: "run-2"}) {
		t.Fatalf("new-run totals = %+v, want reset run-2 totals", got)
	}
}

func TestCounterStopFreezesOnlyMatchingRun(t *testing.T) {
	c := NewCounter()
	mustRegister(t, c, Session{InstanceID: "instance-1", RunID: "run-1", Ports: []int{27015}})
	if err := c.Stop("instance-1", "other-run"); err == nil {
		t.Fatal("Stop accepted a nonmatching run")
	}
	c.Observe(Packet{Length: 20, DstPort: 27015})
	if err := c.Stop("instance-1", "run-1"); err != nil {
		t.Fatal(err)
	}
	c.Observe(Packet{Length: 30, DstPort: 27015})
	got, _ := c.Totals("instance-1")
	if got.RXBytes != 20 {
		t.Fatalf("stopped total = %d, want 20", got.RXBytes)
	}
	mustRegister(t, c, Session{InstanceID: "instance-1", RunID: "run-1", Ports: []int{27016}})
	c.Observe(Packet{Length: 40, DstPort: 27016})
	got, _ = c.Totals("instance-1")
	if got.RXBytes != 20 {
		t.Fatalf("same-run registration unfroze stopped total: %d", got.RXBytes)
	}
}

func TestCounterConcurrentObserveAndRead(t *testing.T) {
	c := NewCounter()
	mustRegister(t, c, Session{InstanceID: "instance-1", RunID: "run-1", Ports: []int{27015}})
	const workers, observations = 8, 1000
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range observations {
				c.Observe(Packet{Length: 1, SrcPort: 27015, DstPort: 27015})
				_, _ = c.Totals("instance-1")
			}
		}()
	}
	wg.Wait()
	got, _ := c.Totals("instance-1")
	if got.RXBytes != workers*observations || got.TXBytes != workers*observations {
		t.Fatalf("concurrent totals = %+v", got)
	}
}

func TestDecodeFrameIPv4TCPAndVLANIPv6UDP(t *testing.T) {
	ipv4 := make([]byte, 14+20+20)
	binary.BigEndian.PutUint16(ipv4[12:14], 0x0800)
	ipv4[14] = 0x45
	ipv4[23] = 6
	binary.BigEndian.PutUint16(ipv4[34:36], 27015)
	binary.BigEndian.PutUint16(ipv4[36:38], 12345)
	packet, ok := decodeFrame(ipv4, 1500)
	if !ok || packet != (Packet{Length: 1500, SrcPort: 27015, DstPort: 12345}) {
		t.Fatalf("IPv4 TCP decode = %+v, %v", packet, ok)
	}

	ipv6 := make([]byte, 14+4+40+8)
	binary.BigEndian.PutUint16(ipv6[12:14], 0x8100)
	binary.BigEndian.PutUint16(ipv6[16:18], 0x86dd)
	ipv6[18] = 0x60
	ipv6[24] = 17
	binary.BigEndian.PutUint16(ipv6[58:60], 12345)
	binary.BigEndian.PutUint16(ipv6[60:62], 27016)
	packet, ok = decodeFrame(ipv6, uint64(len(ipv6)))
	if !ok || packet != (Packet{Length: uint64(len(ipv6)), SrcPort: 12345, DstPort: 27016}) {
		t.Fatalf("VLAN IPv6 UDP decode = %+v, %v", packet, ok)
	}
}

func TestDecodeFrameIgnoresTruncatedUnsupportedAndNonInitialFragments(t *testing.T) {
	frames := [][]byte{
		make([]byte, 13),
		append(make([]byte, 12), 0x08, 0x06),
		func() []byte {
			frame := make([]byte, 14+20+8)
			binary.BigEndian.PutUint16(frame[12:14], 0x0800)
			frame[14] = 0x45
			frame[23] = 17
			binary.BigEndian.PutUint16(frame[20:22], 1)
			return frame
		}(),
	}
	for _, frame := range frames {
		if packet, ok := decodeFrame(frame, uint64(len(frame))); ok {
			t.Fatalf("decodeFrame accepted invalid frame as %+v", packet)
		}
	}
}

func mustRegister(t *testing.T, c *Counter, session Session) {
	t.Helper()
	if err := c.Register(session); err != nil {
		t.Fatal(err)
	}
}

package traffic

import (
	"encoding/binary"
	"errors"
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

func TestCounterStopRejectsInvalidIdentifiers(t *testing.T) {
	c := NewCounter()
	mustRegister(t, c, Session{InstanceID: "instance-1", RunID: "run-1", Ports: []int{27015}})
	tests := []struct {
		instanceID string
		runID      string
	}{
		{instanceID: "", runID: "run-1"},
		{instanceID: "../escape", runID: "run-1"},
		{instanceID: "instance-1", runID: ""},
		{instanceID: "instance-1", runID: "bad/run"},
	}
	for _, tt := range tests {
		err := c.Stop(tt.instanceID, tt.runID)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("Stop(%q, %q) error = %v, want ErrInvalidInput", tt.instanceID, tt.runID, err)
		}
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrRunMismatch) {
			t.Errorf("Stop(%q, %q) conflated invalid input with lookup state: %v", tt.instanceID, tt.runID, err)
		}
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
	binary.BigEndian.PutUint16(ipv4[16:18], 40)
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
	binary.BigEndian.PutUint16(ipv6[22:24], 8)
	ipv6[24] = 17
	binary.BigEndian.PutUint16(ipv6[58:60], 12345)
	binary.BigEndian.PutUint16(ipv6[60:62], 27016)
	packet, ok = decodeFrame(ipv6, uint64(len(ipv6)))
	if !ok || packet != (Packet{Length: uint64(len(ipv6)), SrcPort: 12345, DstPort: 27016}) {
		t.Fatalf("VLAN IPv6 UDP decode = %+v, %v", packet, ok)
	}
}

func TestDecodeFrameIgnoresTruncatedUnsupportedAndNonInitialFragments(t *testing.T) {
	frames := map[string][]byte{
		"truncated Ethernet": make([]byte, 13),
		"unsupported EtherType": func() []byte {
			frame := make([]byte, 14)
			binary.BigEndian.PutUint16(frame[12:14], 0x0806)
			return frame
		}(),
		"truncated VLAN header": func() []byte {
			frame := make([]byte, 17)
			binary.BigEndian.PutUint16(frame[12:14], 0x8100)
			return frame
		}(),
		"truncated stacked VLAN header": func() []byte {
			frame := make([]byte, 21)
			binary.BigEndian.PutUint16(frame[12:14], 0x8100)
			binary.BigEndian.PutUint16(frame[16:18], 0x88a8)
			return frame
		}(),
		"truncated IPv4 base header": ipv4Frame(19, 5, 6),
		"invalid IPv4 IHL":           ipv4Frame(20, 4, 6),
		"truncated IPv4 IHL":         ipv4Frame(23, 6, 6),
		"truncated IPv4 TCP ports":   ipv4Frame(23, 5, 6),
		"truncated IPv4 UDP ports":   ipv4Frame(23, 5, 17),
		"IPv4 non-initial fragment": func() []byte {
			frame := ipv4Frame(28, 5, 17)
			binary.BigEndian.PutUint16(frame[20:22], 1)
			return frame
		}(),
		"truncated IPv6 base header": ipv6Frame(39, 6),
		"truncated IPv6 TCP ports":   ipv6Frame(43, 6),
		"truncated IPv6 UDP ports":   ipv6Frame(43, 17),
		"truncated IPv6 extension header": func() []byte {
			return ipv6Frame(41, 0)
		}(),
		"IPv6 extension length overflow": func() []byte {
			frame := ipv6Frame(48, 0)
			frame[54] = 17
			frame[55] = 2
			return frame
		}(),
		"IPv6 non-initial fragment": func() []byte {
			frame := ipv6Frame(52, 44)
			frame[54] = 17
			binary.BigEndian.PutUint16(frame[56:58], 8)
			return frame
		}(),
		"unsupported IPv6 next header": ipv6Frame(44, 59),
	}
	for name, frame := range frames {
		t.Run(name, func(t *testing.T) {
			if packet, ok := decodeFrame(frame, uint64(len(frame))); ok {
				t.Fatalf("decodeFrame accepted invalid frame as %+v", packet)
			}
		})
	}
}

func TestDecodeFrameHonorsDeclaredIPLengths(t *testing.T) {
	frames := map[string][]byte{
		"IPv4 TCP total length excludes ports": func() []byte {
			frame := ipv4Frame(28, 5, 6)
			binary.BigEndian.PutUint16(frame[16:18], 22)
			binary.BigEndian.PutUint16(frame[34:36], 27015)
			binary.BigEndian.PutUint16(frame[36:38], 12345)
			return frame
		}(),
		"IPv4 UDP total length excludes ports": func() []byte {
			frame := ipv4Frame(28, 5, 17)
			binary.BigEndian.PutUint16(frame[16:18], 23)
			binary.BigEndian.PutUint16(frame[34:36], 27015)
			binary.BigEndian.PutUint16(frame[36:38], 12345)
			return frame
		}(),
		"IPv4 total length exceeds packet": func() []byte {
			frame := ipv4Frame(28, 5, 6)
			binary.BigEndian.PutUint16(frame[16:18], 60)
			binary.BigEndian.PutUint16(frame[34:36], 27015)
			binary.BigEndian.PutUint16(frame[36:38], 12345)
			return frame
		}(),
		"IPv6 payload length excludes ports": func() []byte {
			frame := ipv6Frame(48, 17)
			binary.BigEndian.PutUint16(frame[18:20], 2)
			binary.BigEndian.PutUint16(frame[54:56], 27015)
			binary.BigEndian.PutUint16(frame[56:58], 12345)
			return frame
		}(),
		"IPv6 extension exceeds declared payload": func() []byte {
			frame := ipv6Frame(52, 0)
			binary.BigEndian.PutUint16(frame[18:20], 8)
			frame[54] = 6
			frame[55] = 0
			binary.BigEndian.PutUint16(frame[62:64], 27015)
			binary.BigEndian.PutUint16(frame[64:66], 12345)
			return frame
		}(),
		"IPv6 extension length exceeds declared payload": func() []byte {
			frame := ipv6Frame(60, 0)
			binary.BigEndian.PutUint16(frame[18:20], 12)
			frame[54] = 6
			frame[55] = 1
			binary.BigEndian.PutUint16(frame[70:72], 27015)
			binary.BigEndian.PutUint16(frame[72:74], 12345)
			return frame
		}(),
		"IPv6 payload length exceeds packet": func() []byte {
			frame := ipv6Frame(52, 0)
			binary.BigEndian.PutUint16(frame[18:20], 100)
			frame[54] = 6
			frame[55] = 0
			binary.BigEndian.PutUint16(frame[62:64], 27015)
			binary.BigEndian.PutUint16(frame[64:66], 12345)
			return frame
		}(),
	}
	for name, frame := range frames {
		t.Run(name, func(t *testing.T) {
			if packet, ok := decodeFrame(frame, uint64(len(frame))); ok {
				t.Fatalf("decodeFrame read outside declared IP length: %+v", packet)
			}
		})
	}
}

func ipv4Frame(payloadLength, ihl int, protocol byte) []byte {
	frame := make([]byte, 14+payloadLength)
	binary.BigEndian.PutUint16(frame[12:14], 0x0800)
	if payloadLength > 0 {
		frame[14] = 0x40 | byte(ihl)
	}
	if payloadLength > 3 {
		binary.BigEndian.PutUint16(frame[16:18], uint16(payloadLength))
	}
	if payloadLength > 9 {
		frame[23] = protocol
	}
	return frame
}

func ipv6Frame(payloadLength int, nextHeader byte) []byte {
	frame := make([]byte, 14+payloadLength)
	binary.BigEndian.PutUint16(frame[12:14], 0x86dd)
	if payloadLength > 0 {
		frame[14] = 0x60
	}
	if payloadLength > 6 {
		declaredPayload := payloadLength - 40
		if declaredPayload < 0 {
			declaredPayload = 0
		}
		binary.BigEndian.PutUint16(frame[18:20], uint16(declaredPayload))
	}
	if payloadLength > 6 {
		frame[20] = nextHeader
	}
	return frame
}

func mustRegister(t *testing.T, c *Counter, session Session) {
	t.Helper()
	if err := c.Register(session); err != nil {
		t.Fatal(err)
	}
}

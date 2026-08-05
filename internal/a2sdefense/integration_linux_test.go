//go:build integration && linux

package a2sdefense

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRealIPv4RulesLimitA2SPlayerFloodAndCleanUp(t *testing.T) {
	if os.Getenv("L4D2_A2S_DEFENSE_NETNS") != "1" || os.Geteuid() != 0 {
		t.Fatal("integration test must run as root inside the disposable A2S defense network namespace")
	}
	manager := NewManager(CommandExecutor{}, time.Now)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ring := NewEventRing("integration", DefaultEventCapacity)
	sourceErrors := make(chan error, 1)
	go func() { sourceErrors <- NewNFLogSource(ring, time.Now).Run(ctx) }()
	ports := make([]int, 16)
	for index := range ports {
		ports[index] = 27015 + index
	}
	if _, err := manager.Apply(ctx, Config{Version: APIVersion, Enabled: true, Ports: ports, Revision: 1}); err != nil {
		t.Fatalf("apply rules: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = manager.Apply(cleanupCtx, Config{Version: APIVersion, Enabled: false, Revision: 2})
	})

	listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 27015})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	sender, err := net.DialUDP("udp4", nil, listener.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer sender.Close()

	const sent = 200
	payload := []byte{0xff, 0xff, 0xff, 0xff, 0x55, 0xff, 0xff, 0xff, 0xff}
	for index := 0; index < sent; index++ {
		if _, err := sender.Write(payload); err != nil {
			t.Fatal(err)
		}
	}
	if err := listener.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	received := 0
	buffer := make([]byte, 64)
	for {
		if _, _, err := listener.ReadFromUDP(buffer); err != nil {
			if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
				break
			}
			t.Fatal(err)
		}
		received++
	}
	if received == 0 || received >= sent {
		t.Fatalf("received=%d sent=%d, expected initial burst followed by drops", received, sent)
	}
	status, err := manager.Status(ctx)
	if err != nil {
		t.Fatalf("read live status: %v", err)
	}
	if status.Counters.Player == 0 || status.Counters.Blacklist == 0 || status.BlacklistSize != 1 {
		t.Fatalf("live status did not report drops and attacker: %+v", status)
	}
	deadline := time.Now().Add(2 * time.Second)
	var batch EventBatch
	for time.Now().Before(deadline) {
		batch = ring.Batch("integration", 0)
		if len(batch.Events) > 0 {
			break
		}
		select {
		case sourceErr := <-sourceErrors:
			t.Fatalf("NFLOG source stopped before receiving an event: %v", sourceErr)
		case <-time.After(20 * time.Millisecond):
		}
	}
	if len(batch.Events) == 0 {
		t.Fatal("NFLOG did not deliver a sampled drop event")
	}
	event := batch.Events[0]
	if !event.Source.Is4() || event.SourcePort < 1 || event.DestinationPort != 27015 || event.PacketBytes != 20+8+len(payload) || event.Query != QueryPlayer || event.SampledDrops60s < 1 || event.Action != "DROP" {
		t.Fatalf("unexpected NFLOG event: %+v", event)
	}

	if _, err := manager.Apply(ctx, Config{Version: APIVersion, Enabled: false, Revision: 2}); err != nil {
		t.Fatalf("disable rules: %v", err)
	}
	output, err := CommandExecutor{}.Run(ctx, iptablesSavePath, []string{"-t", "filter"}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, chain := range projectChains() {
		if strings.Contains(output, chain) {
			t.Fatalf("project chain %s remains after disable\n%s", chain, output)
		}
	}
}

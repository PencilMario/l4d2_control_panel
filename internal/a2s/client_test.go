package a2s

import (
	"encoding/binary"
	"math"
	"net"
	"testing"
	"time"
)

func TestQueryInfoAndPlayers(t *testing.T) {
	server, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	go func() {
		buffer := make([]byte, 1400)
		for i := 0; i < 3; i++ {
			n, addr, _ := server.ReadFrom(buffer)
			if n < 5 {
				continue
			}
			switch buffer[4] {
			case 0x54:
				payload := append([]byte{0xff, 0xff, 0xff, 0xff, 0x49, 17}, z("Test Server")...)
				payload = append(payload, z("c2m1_highway")...)
				payload = append(payload, z("left4dead2")...)
				payload = append(payload, z("Left 4 Dead 2")...)
				payload = binary.LittleEndian.AppendUint16(payload, 550)
				payload = append(payload, 4, 8, 0, 'd', 'l', 0, 0, 1, 0)
				_, _ = server.WriteTo(payload, addr)
			case 0x55:
				if binary.LittleEndian.Uint32(buffer[5:9]) == 0xffffffff {
					payload := []byte{0xff, 0xff, 0xff, 0xff, 0x41, 1, 2, 3, 4}
					_, _ = server.WriteTo(payload, addr)
				} else {
					payload := []byte{0xff, 0xff, 0xff, 0xff, 0x44, 1, 0}
					payload = append(payload, z("Coach")...)
					payload = binary.LittleEndian.AppendUint32(payload, 12)
					payload = binary.LittleEndian.AppendUint32(payload, math.Float32bits(90))
					_, _ = server.WriteTo(payload, addr)
				}
			}
		}
	}()
	client := Client{Timeout: time.Second}
	info, err := client.Info(server.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "Test Server" || info.Map != "c2m1_highway" || info.Players != 4 || info.MaxPlayers != 8 {
		t.Fatalf("info=%#v", info)
	}
	players, err := client.Players(server.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 1 || players[0].Name != "Coach" || players[0].Score != 12 || players[0].Duration != 90 {
		t.Fatalf("players=%#v", players)
	}
}
func z(value string) []byte { return append([]byte(value), 0) }

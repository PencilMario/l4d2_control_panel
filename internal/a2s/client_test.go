package a2s

import (
	"bytes"
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
		for i := 0; i < 4; i++ {
			n, addr, _ := server.ReadFrom(buffer)
			if n < 5 {
				continue
			}
			switch buffer[4] {
			case 0x54:
				if !bytes.HasSuffix(buffer[:n], []byte{1, 2, 3, 4}) {
					payload := []byte{0xff, 0xff, 0xff, 0xff, 0x41, 1, 2, 3, 4}
					_, _ = server.WriteTo(payload, addr)
					continue
				}
				_, _ = server.WriteTo(infoResponse(), addr)
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

func TestInfoAcceptsDirectResponse(t *testing.T) {
	server, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	go func() {
		buffer := make([]byte, 1400)
		n, addr, err := server.ReadFrom(buffer)
		if err != nil || n < 5 || buffer[4] != 0x54 {
			return
		}
		_, _ = server.WriteTo(infoResponse(), addr)
	}()

	client := Client{Timeout: time.Second}
	info, err := client.Info(server.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "Test Server" || info.Map != "c2m1_highway" || info.Players != 4 || info.MaxPlayers != 8 {
		t.Fatalf("info=%#v", info)
	}
}

func TestPlayersCollectsSplitResponse(t *testing.T) {
	server, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	go func() {
		buffer := make([]byte, 1400)
		n, addr, readErr := server.ReadFrom(buffer)
		if readErr != nil || n < 9 || buffer[4] != 0x55 {
			return
		}
		_, _ = server.WriteTo([]byte{0xff, 0xff, 0xff, 0xff, 0x41, 1, 2, 3, 4}, addr)
		n, addr, readErr = server.ReadFrom(buffer)
		if readErr != nil || n < 9 || buffer[4] != 0x55 {
			return
		}
		response := playerResponse()
		parts := [][]byte{response[:12], response[12:]}
		for number, part := range parts {
			packet := []byte{0xfe, 0xff, 0xff, 0xff}
			packet = binary.LittleEndian.AppendUint32(packet, 42)
			packet = append(packet, byte(len(parts)), byte(number))
			packet = binary.LittleEndian.AppendUint16(packet, 1248)
			packet = append(packet, part...)
			_, _ = server.WriteTo(packet, addr)
		}
	}()

	players, err := (Client{Timeout: time.Second}).Players(server.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 1 || players[0].Name != "Coach" || players[0].Score != 12 || players[0].Duration != 90 {
		t.Fatalf("players=%#v", players)
	}
}

func infoResponse() []byte {
	payload := append([]byte{0xff, 0xff, 0xff, 0xff, 0x49, 17}, z("Test Server")...)
	payload = append(payload, z("c2m1_highway")...)
	payload = append(payload, z("left4dead2")...)
	payload = append(payload, z("Left 4 Dead 2")...)
	payload = binary.LittleEndian.AppendUint16(payload, 550)
	return append(payload, 4, 8, 0, 'd', 'l', 0, 0, 1, 0)
}

func playerResponse() []byte {
	payload := []byte{0xff, 0xff, 0xff, 0xff, 0x44, 1, 0}
	payload = append(payload, z("Coach")...)
	payload = binary.LittleEndian.AppendUint32(payload, 12)
	payload = binary.LittleEndian.AppendUint32(payload, math.Float32bits(90))
	return payload
}

func z(value string) []byte { return append([]byte(value), 0) }

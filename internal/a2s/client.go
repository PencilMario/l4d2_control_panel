package a2s

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"net"
	"time"
)

type Client struct{ Timeout time.Duration }
type Info struct {
	Name, Map, Folder, Game   string
	AppID                     int
	Players, MaxPlayers, Bots int
}
type Player struct {
	Name     string
	Score    int32
	Duration float32
}

func (c Client) exchange(address string, payload []byte) ([]byte, error) {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	connection, err := net.DialTimeout("udp", address, timeout)
	if err != nil {
		return nil, err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(timeout))
	if _, err = connection.Write(payload); err != nil {
		return nil, err
	}
	buffer := make([]byte, 65535)
	n, err := connection.Read(buffer)
	if err != nil {
		return nil, err
	}
	if n < 5 || !bytes.Equal(buffer[:4], []byte{0xff, 0xff, 0xff, 0xff}) {
		return nil, errors.New("invalid A2S response")
	}
	return append([]byte(nil), buffer[:n]...), nil
}
func (c Client) Info(address string) (Info, error) {
	request := append([]byte{0xff, 0xff, 0xff, 0xff, 0x54}, []byte("Source Engine Query\x00")...)
	raw, err := c.exchange(address, request)
	if err != nil {
		return Info{}, err
	}
	if raw[4] == 0x41 {
		if len(raw) < 9 {
			return Info{}, errors.New("truncated A2S_INFO challenge")
		}
		request = append(request, raw[5:9]...)
		raw, err = c.exchange(address, request)
		if err != nil {
			return Info{}, err
		}
	}
	if raw[4] != 0x49 || len(raw) < 6 {
		return Info{}, errors.New("unexpected A2S_INFO response")
	}
	offset := 6
	name, next, err := cstring(raw, offset)
	if err != nil {
		return Info{}, err
	}
	offset = next
	gameMap, next, err := cstring(raw, offset)
	if err != nil {
		return Info{}, err
	}
	offset = next
	folder, next, err := cstring(raw, offset)
	if err != nil {
		return Info{}, err
	}
	offset = next
	game, next, err := cstring(raw, offset)
	if err != nil {
		return Info{}, err
	}
	offset = next
	if len(raw) < offset+5 {
		return Info{}, errors.New("truncated A2S_INFO response")
	}
	return Info{Name: name, Map: gameMap, Folder: folder, Game: game, AppID: int(binary.LittleEndian.Uint16(raw[offset : offset+2])), Players: int(raw[offset+2]), MaxPlayers: int(raw[offset+3]), Bots: int(raw[offset+4])}, nil
}
func (c Client) Players(address string) ([]Player, error) {
	request := []byte{0xff, 0xff, 0xff, 0xff, 0x55, 0xff, 0xff, 0xff, 0xff}
	challenge, err := c.exchange(address, request)
	if err != nil {
		return nil, err
	}
	if len(challenge) < 9 || challenge[4] != 0x41 {
		return nil, errors.New("invalid A2S challenge")
	}
	copy(request[5:9], challenge[5:9])
	raw, err := c.exchange(address, request)
	if err != nil {
		return nil, err
	}
	if len(raw) < 6 || raw[4] != 0x44 {
		return nil, errors.New("unexpected A2S_PLAYER response")
	}
	count, offset := int(raw[5]), 6
	players := make([]Player, 0, count)
	for i := 0; i < count; i++ {
		if offset >= len(raw) {
			return nil, errors.New("truncated player response")
		}
		offset++
		name, next, err := cstring(raw, offset)
		if err != nil {
			return nil, err
		}
		offset = next
		if len(raw) < offset+8 {
			return nil, errors.New("truncated player fields")
		}
		score := int32(binary.LittleEndian.Uint32(raw[offset : offset+4]))
		duration := math.Float32frombits(binary.LittleEndian.Uint32(raw[offset+4 : offset+8]))
		offset += 8
		players = append(players, Player{Name: name, Score: score, Duration: duration})
	}
	return players, nil
}
func cstring(raw []byte, offset int) (string, int, error) {
	if offset >= len(raw) {
		return "", offset, errors.New("missing string")
	}
	end := bytes.IndexByte(raw[offset:], 0)
	if end < 0 {
		return "", offset, errors.New("unterminated string")
	}
	return string(raw[offset : offset+end]), offset + end + 1, nil
}

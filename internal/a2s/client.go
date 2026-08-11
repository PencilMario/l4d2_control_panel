package a2s

import (
	"bytes"
	"errors"
	"net"
	"time"

	sourcequery "github.com/rumblefrog/go-a2s"
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

func (c Client) newClient(address string) (*sourcequery.Client, error) {
	return sourcequery.NewClient(address, sourcequery.TimeoutOption(c.timeout()))
}

func (c Client) Info(address string) (Info, error) {
	value, err := c.infoAt(address)
	if err == nil {
		return value, nil
	}
	fallback, fallbackErr := c.discoverServerAddress(address)
	if fallbackErr != nil || fallback == address {
		return Info{}, err
	}
	return c.infoAt(fallback)
}

func (c Client) infoAt(address string) (Info, error) {
	client, err := c.newClient(address)
	if err != nil {
		return Info{}, err
	}
	defer client.Close()

	value, err := client.QueryInfo()
	if err != nil {
		return Info{}, err
	}
	return Info{
		Name:       value.Name,
		Map:        value.Map,
		Folder:     value.Folder,
		Game:       value.Game,
		AppID:      int(value.ID),
		Players:    int(value.Players),
		MaxPlayers: int(value.MaxPlayers),
		Bots:       int(value.Bots),
	}, nil
}

func (c Client) Players(address string) ([]Player, error) {
	players, err := c.playersAt(address)
	if err == nil {
		return players, nil
	}
	fallback, fallbackErr := c.discoverServerAddress(address)
	if fallbackErr != nil || fallback == address {
		return nil, err
	}
	return c.playersAt(fallback)
}

func (c Client) playersAt(address string) ([]Player, error) {
	client, err := c.newClient(address)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	value, err := client.QueryPlayer()
	if err != nil {
		return nil, err
	}
	players := make([]Player, 0, len(value.Players))
	for _, player := range value.Players {
		players = append(players, Player{Name: player.Name, Score: player.Score, Duration: player.Duration})
	}
	return players, nil
}

func (c Client) timeout() time.Duration {
	if c.Timeout == 0 {
		return 2 * time.Second
	}
	return c.Timeout
}

func (c Client) discoverServerAddress(address string) (string, error) {
	target, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return "", err
	}
	network := "udp"
	if target.IP.To4() != nil {
		network = "udp4"
	}
	conn, err := net.ListenUDP(network, nil)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(c.timeout())); err != nil {
		return "", err
	}
	request := append([]byte{0xff, 0xff, 0xff, 0xff, 0x54}, []byte("Source Engine Query\x00")...)
	if _, err := conn.WriteToUDP(request, target); err != nil {
		return "", err
	}
	buffer := make([]byte, 1400)
	n, peer, err := conn.ReadFromUDP(buffer)
	if err != nil {
		return "", err
	}
	if n < 5 || !bytes.Equal(buffer[:4], []byte{0xff, 0xff, 0xff, 0xff}) || (buffer[4] != 0x41 && buffer[4] != 0x49) {
		return "", errors.New("invalid A2S discovery response")
	}
	if target.Port != 0 && peer.Port != target.Port {
		return "", errors.New("A2S discovery response port mismatch")
	}
	return peer.String(), nil
}

package a2s

import (
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
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	return sourcequery.NewClient(address, sourcequery.TimeoutOption(timeout))
}

func (c Client) Info(address string) (Info, error) {
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

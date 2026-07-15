package players

import (
	"context"
	"errors"
	"github.com/not0721here/l4d2-control-panel/internal/a2s"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"net"
	"strconv"
	"time"
)

type InstanceRepository interface {
	Instance(context.Context, string) (domain.Instance, error)
}
type Query interface {
	Info(string) (a2s.Info, error)
	Players(string) ([]a2s.Player, error)
}
type Console interface {
	Status(context.Context, string) (string, error)
	PlayerCommand(context.Context, string, string) error
}
type Service struct {
	instances InstanceRepository
	query     Query
	console   Console
	host      string
}
type OnlinePlayer struct {
	UserID    int           `json:"user_id"`
	Name      string        `json:"name"`
	UniqueID  string        `json:"unique_id"`
	Connected string        `json:"connected"`
	Ping      int           `json:"ping"`
	Loss      int           `json:"loss"`
	Score     *int32        `json:"score"`
	Duration  time.Duration `json:"duration"`
}
type Snapshot struct {
	Map        string         `json:"map"`
	Players    []OnlinePlayer `json:"players"`
	MaxPlayers int            `json:"max_players"`
	Match      MatchInfo      `json:"match"`
}
type Summary struct {
	Map        string `json:"map"`
	Players    int    `json:"players"`
	MaxPlayers int    `json:"max_players"`
}

func NewService(instances InstanceRepository, query Query, console Console, host string) *Service {
	return &Service{instances: instances, query: query, console: console, host: host}
}
func (s *Service) Summary(ctx context.Context, id string) (Summary, error) {
	instance, err := s.instances.Instance(ctx, id)
	if err != nil {
		return Summary{}, err
	}
	if instance.ContainerID == "" {
		return Summary{}, errors.New("instance container unavailable")
	}
	address := net.JoinHostPort(s.host, strconv.Itoa(instance.GamePort))
	info, err := s.query.Info(address)
	if err != nil {
		return Summary{}, err
	}
	return Summary{Map: info.Map, Players: info.Players, MaxPlayers: info.MaxPlayers}, nil
}
func (s *Service) Online(ctx context.Context, id string) (Snapshot, error) {
	instance, err := s.instances.Instance(ctx, id)
	if err != nil {
		return Snapshot{}, err
	}
	if instance.ContainerID == "" {
		return Snapshot{}, errors.New("instance container unavailable")
	}
	address := net.JoinHostPort(s.host, strconv.Itoa(instance.GamePort))
	info, err := s.query.Info(address)
	if err != nil {
		return Snapshot{}, err
	}
	queried, err := s.query.Players(address)
	if err != nil {
		return Snapshot{}, err
	}
	statusRaw, err := s.console.Status(ctx, instance.ContainerID)
	if err != nil {
		return Snapshot{}, err
	}
	status := ParseStatusSnapshot(statusRaw)
	if status.Match.Map == "" {
		status.Match.Map = info.Map
	}
	if status.Match.MaxPlayers == 0 {
		status.Match.MaxPlayers = info.MaxPlayers
	}
	queryByName := map[string][]int{}
	for index, player := range queried {
		queryByName[player.Name] = append(queryByName[player.Name], index)
	}
	statusNameCount := map[string]int{}
	for _, player := range status.Players {
		statusNameCount[player.Name]++
	}
	represented := make([]bool, len(queried))
	result := make([]OnlinePlayer, 0, len(status.Players)+len(queried))
	for _, player := range status.Players {
		value := OnlinePlayer{UserID: player.UserID, Name: player.Name, UniqueID: player.SteamID, Connected: player.Connected, Ping: player.Ping, Loss: player.Loss}
		indices := queryByName[player.Name]
		for _, index := range indices {
			represented[index] = true
		}
		if statusNameCount[player.Name] == 1 && len(indices) == 1 {
			queriedPlayer := queried[indices[0]]
			score := queriedPlayer.Score
			value.Score = &score
			value.Duration = time.Duration(float64(time.Second) * float64(queriedPlayer.Duration))
		}
		result = append(result, value)
	}
	for index, player := range queried {
		if represented[index] {
			continue
		}
		score := player.Score
		result = append(result, OnlinePlayer{Name: player.Name, Score: &score, Duration: time.Duration(float64(time.Second) * float64(player.Duration))})
	}
	return Snapshot{Map: status.Match.Map, Players: result, MaxPlayers: status.Match.MaxPlayers, Match: status.Match}, nil
}
func (s *Service) Kick(ctx context.Context, id string, userID int) error {
	instance, err := s.instances.Instance(ctx, id)
	if err != nil {
		return err
	}
	command, err := Kick(userID)
	if err != nil {
		return err
	}
	return s.console.PlayerCommand(ctx, instance.ContainerID, command)
}
func (s *Service) Ban(ctx context.Context, id string, userID, minutes int) error {
	instance, err := s.instances.Instance(ctx, id)
	if err != nil {
		return err
	}
	command, err := Ban(userID, minutes)
	if err != nil {
		return err
	}
	return s.console.PlayerCommand(ctx, instance.ContainerID, command)
}

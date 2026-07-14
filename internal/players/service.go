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
	UserID   int           `json:"user_id"`
	Name     string        `json:"name"`
	Score    int32         `json:"score"`
	Duration time.Duration `json:"duration"`
}
type Snapshot struct {
	Map        string         `json:"map"`
	Players    []OnlinePlayer `json:"players"`
	MaxPlayers int            `json:"max_players"`
}

func NewService(instances InstanceRepository, query Query, console Console, host string) *Service {
	return &Service{instances: instances, query: query, console: console, host: host}
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
	byName := map[string][]int{}
	for _, entry := range ParseStatus(statusRaw) {
		byName[entry.Name] = append(byName[entry.Name], entry.UserID)
	}
	result := make([]OnlinePlayer, 0, len(queried))
	for _, player := range queried {
		userID := 0
		if ids := byName[player.Name]; len(ids) == 1 {
			userID = ids[0]
		}
		result = append(result, OnlinePlayer{UserID: userID, Name: player.Name, Score: player.Score, Duration: time.Duration(float64(time.Second) * float64(player.Duration))})
	}
	return Snapshot{Map: info.Map, Players: result, MaxPlayers: info.MaxPlayers}, nil
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

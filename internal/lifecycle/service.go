package lifecycle

import (
	"context"
	"errors"
	"github.com/not0721here/l4d2-control-panel/internal/docker"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"os"
	"path/filepath"
)

type Repository interface {
	Instance(context.Context, string) (domain.Instance, error)
	UpdateInstance(context.Context, domain.Instance) error
}
type Engine interface {
	Create(context.Context, docker.ContainerSpec) (string, error)
	Start(context.Context, string) error
	RunSupervisor(context.Context, string, string) error
	Stop(context.Context, string, int) error
}
type PortChecker interface{ Available(int) error }
type Service struct {
	repo     Repository
	engine   Engine
	ports    PortChecker
	dataRoot string
}

func New(repo Repository, engine Engine, ports PortChecker, dataRoot string) *Service {
	return &Service{repo: repo, engine: engine, ports: ports, dataRoot: dataRoot}
}
func (s *Service) Start(ctx context.Context, id string) error {
	v, err := s.repo.Instance(ctx, id)
	if err != nil {
		return err
	}
	if v.ContainerID == "" {
		if err := s.ports.Available(v.GamePort); err != nil {
			return err
		}
		base := filepath.Join(s.dataRoot, "instances", v.ID)
		for _, dir := range []string{"game", "private", "backups", "console"} {
			if err := os.MkdirAll(filepath.Join(base, dir), 0750); err != nil {
				return err
			}
		}
		v.ActualState = domain.StateStarting
		if err := s.repo.UpdateInstance(ctx, v); err != nil {
			return err
		}
		containerID, err := s.engine.Create(ctx, docker.BuildContainerSpec(s.dataRoot, v))
		if err != nil {
			return s.fault(ctx, v, err)
		}
		v.ContainerID = containerID
		if err := s.repo.UpdateInstance(ctx, v); err != nil {
			return err
		}
	}
	if err := s.engine.Start(ctx, v.ContainerID); err != nil {
		return s.fault(ctx, v, err)
	}
	v.DesiredState, v.ActualState = domain.StateRunning, domain.StateRunning
	return s.repo.UpdateInstance(ctx, v)
}
func (s *Service) Stop(ctx context.Context, id string) error {
	v, err := s.repo.Instance(ctx, id)
	if err != nil {
		return err
	}
	if v.ContainerID == "" {
		return errors.New("instance has no container")
	}
	_ = s.engine.RunSupervisor(ctx, v.ContainerID, "stop")
	if err := s.engine.Stop(ctx, v.ContainerID, 15); err != nil {
		return s.fault(ctx, v, err)
	}
	v.DesiredState, v.ActualState = domain.StateStopped, domain.StateStopped
	return s.repo.UpdateInstance(ctx, v)
}
func (s *Service) Restart(ctx context.Context, id string) error {
	if err := s.Stop(ctx, id); err != nil {
		return err
	}
	return s.Start(ctx, id)
}
func (s *Service) fault(ctx context.Context, v domain.Instance, cause error) error {
	v.ActualState = domain.StateFaulted
	_ = s.repo.UpdateInstance(ctx, v)
	return cause
}

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
	Instances(context.Context) ([]domain.Instance, error)
	UpdateInstance(context.Context, domain.Instance) error
	DeleteInstance(context.Context, string) error
}
type Engine interface {
	Create(context.Context, docker.ContainerSpec) (string, error)
	Start(context.Context, string) error
	RunSupervisor(context.Context, string, string) error
	Stop(context.Context, string, int) error
	ListManaged(context.Context) ([]docker.Container, error)
	Remove(context.Context, string) error
}

func (s *Service) Reconcile(ctx context.Context) ([]docker.Container, error) {
	containers, err := s.engine.ListManaged(ctx)
	if err != nil {
		return nil, err
	}
	instances, err := s.repo.Instances(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]docker.Container, len(containers))
	for _, container := range containers {
		byID[container.InstanceID()] = container
	}
	known := make(map[string]bool, len(instances))
	for _, instance := range instances {
		known[instance.ID] = true
		if container, ok := byID[instance.ID]; ok {
			instance.ContainerID = container.ID
			if container.State == "running" {
				instance.ActualState = domain.StateRunning
			} else {
				instance.ActualState = domain.StateStopped
			}
			if err := s.repo.UpdateInstance(ctx, instance); err != nil {
				return nil, err
			}
		} else if instance.ActualState == domain.StateRunning || instance.DesiredState == domain.StateRunning {
			instance.ActualState = domain.StateOrphaned
			if err := s.repo.UpdateInstance(ctx, instance); err != nil {
				return nil, err
			}
		}
	}
	unknown := make([]docker.Container, 0)
	for _, container := range containers {
		if !known[container.InstanceID()] {
			unknown = append(unknown, container)
		}
	}
	return unknown, nil
}

type PortChecker interface{ Available(int) error }
type HealthChecker interface {
	Wait(context.Context, domain.Instance) error
}
type Service struct {
	repo     Repository
	engine   Engine
	ports    PortChecker
	dataRoot string
	health   HealthChecker
}
type Option func(*Service)

func WithHealth(checker HealthChecker) Option { return func(s *Service) { s.health = checker } }

func New(repo Repository, engine Engine, ports PortChecker, dataRoot string, options ...Option) *Service {
	service := &Service{repo: repo, engine: engine, ports: ports, dataRoot: dataRoot}
	for _, option := range options {
		option(service)
	}
	return service
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
		if v.ActualState == domain.StateUninstalled {
			v.ActualState = domain.StateInstalling
		} else {
			v.ActualState = domain.StateStarting
		}
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
	if s.health != nil {
		if err := s.health.Wait(ctx, v); err != nil {
			return s.fault(ctx, v, err)
		}
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
func (s *Service) Rebuild(ctx context.Context, id string) error {
	instance, err := s.repo.Instance(ctx, id)
	if err != nil {
		return err
	}
	wasRunning := instance.ActualState == domain.StateRunning
	if wasRunning {
		if err := s.Stop(ctx, id); err != nil {
			return err
		}
		instance, err = s.repo.Instance(ctx, id)
		if err != nil {
			return err
		}
	}
	if instance.ContainerID != "" {
		if err := s.engine.Remove(ctx, instance.ContainerID); err != nil {
			return err
		}
	}
	instance.ContainerID = ""
	instance.ActualState = domain.StateStopped
	if wasRunning {
		instance.DesiredState = domain.StateRunning
	}
	if err := s.repo.UpdateInstance(ctx, instance); err != nil {
		return err
	}
	if wasRunning {
		return s.Start(ctx, id)
	}
	return nil
}
func (s *Service) Delete(ctx context.Context, id string, deleteData bool) error {
	instance, err := s.repo.Instance(ctx, id)
	if err != nil {
		return err
	}
	if instance.ActualState == domain.StateRunning {
		if err := s.Stop(ctx, id); err != nil {
			return err
		}
		instance, err = s.repo.Instance(ctx, id)
		if err != nil {
			return err
		}
	}
	if instance.ContainerID != "" {
		if err := s.engine.Remove(ctx, instance.ContainerID); err != nil {
			return err
		}
	}
	if err := s.repo.DeleteInstance(ctx, id); err != nil {
		return err
	}
	if deleteData {
		if filepath.Base(id) != id {
			return errors.New("invalid instance id")
		}
		return os.RemoveAll(filepath.Join(s.dataRoot, "instances", id))
	}
	return nil
}
func (s *Service) fault(ctx context.Context, v domain.Instance, cause error) error {
	v.ActualState = domain.StateFaulted
	_ = s.repo.UpdateInstance(ctx, v)
	return cause
}

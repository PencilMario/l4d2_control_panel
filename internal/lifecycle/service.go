package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"github.com/not0721here/l4d2-control-panel/internal/docker"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/maintenance"
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

var ErrMaintenanceActive = errors.New("instance maintenance writer is active")

func (s *Service) Reconcile(ctx context.Context) ([]docker.Container, error) {
	containers, err := s.engine.ListManaged(ctx)
	if err != nil {
		return nil, err
	}
	instances, err := s.repo.Instances(ctx)
	if err != nil {
		return nil, err
	}
	gameByID := make(map[string]docker.Container, len(containers))
	maintenanceByID := make(map[string]bool)
	for _, container := range containers {
		switch container.Role() {
		case "maintenance":
			maintenanceByID[container.InstanceID()] = true
		case "game":
			gameByID[container.InstanceID()] = container
		}
	}
	known := make(map[string]bool, len(instances))
	for _, instance := range instances {
		known[instance.ID] = true
		container, hasGame := gameByID[instance.ID]
		if hasGame {
			instance.ContainerID = container.ID
		}
		if maintenanceByID[instance.ID] {
			instance.ActualState = domain.StateUpdating
			if err := s.repo.UpdateInstance(ctx, instance); err != nil {
				return nil, err
			}
			continue
		}
		if hasGame {
			if container.State == "running" {
				if s.health == nil {
					instance.ActualState = domain.StateRunning
				} else {
					instance.ActualState = domain.StateStarting
				}
			} else {
				instance.ActualState = domain.StateStopped
			}
			if err := s.repo.UpdateInstance(ctx, instance); err != nil {
				return nil, err
			}
			if container.State == "running" && s.health != nil {
				candidate := instance
				go func() {
					if err := s.health.Wait(context.Background(), candidate); err != nil {
						_ = s.fault(context.Background(), candidate, err)
						return
					}
					candidate.ActualState = domain.StateRunning
					_ = s.repo.UpdateInstance(context.Background(), candidate)
				}()
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
		role := container.Role()
		if !known[container.InstanceID()] || (role != "game" && role != "maintenance") {
			unknown = append(unknown, container)
		}
	}
	return unknown, nil
}

type PortChecker interface {
	Available(context.Context, string, []int) error
}
type HealthChecker interface {
	Wait(context.Context, domain.Instance) error
}
type SpaceChecker interface{ Available(string) (uint64, error) }
type Provisioner interface {
	Prepare(context.Context, domain.Instance) error
}
type Service struct {
	repo                Repository
	engine              Engine
	ports               PortChecker
	dataRoot            string
	health              HealthChecker
	space               SpaceChecker
	provisioner         Provisioner
	minimumInstallBytes uint64
	maintenanceGate     *maintenance.Gate
}
type Option func(*Service)

func WithHealth(checker HealthChecker) Option { return func(s *Service) { s.health = checker } }
func WithSpace(checker SpaceChecker, minimum uint64) Option {
	return func(s *Service) { s.space = checker; s.minimumInstallBytes = minimum }
}
func WithProvisioner(provisioner Provisioner) Option {
	return func(s *Service) { s.provisioner = provisioner }
}
func WithMaintenanceGate(gate *maintenance.Gate) Option {
	return func(s *Service) { s.maintenanceGate = gate }
}

func New(repo Repository, engine Engine, ports PortChecker, dataRoot string, options ...Option) *Service {
	service := &Service{repo: repo, engine: engine, ports: ports, dataRoot: dataRoot}
	for _, option := range options {
		option(service)
	}
	return service
}
func (s *Service) Start(ctx context.Context, id string) error {
	ctx, release, err := s.lease(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := s.ensureNoMaintenance(ctx, id); err != nil {
		return err
	}
	v, err := s.repo.Instance(ctx, id)
	if err != nil {
		return err
	}
	if v.ActualState == domain.StateUninstalled && s.space != nil {
		available, err := s.space.Available(s.dataRoot)
		if err != nil {
			return err
		}
		if available < s.minimumInstallBytes {
			return fmt.Errorf("insufficient disk space: have %d bytes, need %d", available, s.minimumInstallBytes)
		}
	}
	declaredPorts := []int{v.GamePort}
	if v.SourceTVPort != 0 {
		declaredPorts = append(declaredPorts, v.SourceTVPort)
	}
	declaredPorts = append(declaredPorts, v.PluginPorts...)
	if s.ports != nil {
		if err := s.ports.Available(ctx, v.ID, declaredPorts); err != nil {
			return err
		}
	}
	if v.ContainerID == "" {
		base := filepath.Join(s.dataRoot, "instances", v.ID)
		for _, dir := range []string{"game", "private", "backups", "console"} {
			if err := os.MkdirAll(filepath.Join(base, dir), 0750); err != nil {
				return err
			}
		}
		needsProvision := s.provisioner != nil && (v.ActualState == domain.StateUninstalled || v.PackageVersion != v.SelectedPackageID)
		if s.provisioner != nil && v.SelectedPackageID == "" {
			return s.fault(ctx, v, errors.New("instance package is required"))
		}
		if needsProvision {
			v.ActualState = domain.StateInstalling
		} else {
			v.ActualState = domain.StateStarting
		}
		if err := s.repo.UpdateInstance(ctx, v); err != nil {
			return err
		}
		if needsProvision {
			if err := s.provisioner.Prepare(ctx, v); err != nil {
				return s.fault(ctx, v, err)
			}
			v, err = s.repo.Instance(ctx, id)
			if err != nil {
				return err
			}
		}
		spec, err := docker.BuildContainerSpec(s.dataRoot, v)
		if err != nil {
			return s.fault(ctx, v, err)
		}
		containerID, err := s.engine.Create(ctx, spec)
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
	ctx, release, err := s.lease(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := s.ensureNoMaintenance(ctx, id); err != nil {
		return err
	}
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
	ctx, release, err := s.lease(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := s.Stop(ctx, id); err != nil {
		return err
	}
	return s.Start(ctx, id)
}
func (s *Service) Rebuild(ctx context.Context, id string) error {
	ctx, release, err := s.lease(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := s.ensureNoMaintenance(ctx, id); err != nil {
		return err
	}
	instance, err := s.repo.Instance(ctx, id)
	if err != nil {
		return err
	}
	wasRunning := instance.DesiredState == domain.StateRunning || instance.ActualState == domain.StateRunning || instance.ActualState == domain.StateStarting || instance.ActualState == domain.StateInstalling
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
	ctx, release, err := s.lease(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := s.ensureNoMaintenance(ctx, id); err != nil {
		return err
	}
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

func (s *Service) lease(ctx context.Context) (context.Context, func(), error) {
	if s.maintenanceGate == nil {
		return ctx, func() {}, nil
	}
	return s.maintenanceGate.SharedContext(ctx)
}
func (s *Service) fault(ctx context.Context, v domain.Instance, cause error) error {
	v.ActualState = domain.StateFaulted
	_ = s.repo.UpdateInstance(ctx, v)
	return cause
}

func (s *Service) ensureNoMaintenance(ctx context.Context, instanceID string) error {
	containers, err := s.engine.ListManaged(ctx)
	if err != nil {
		return err
	}
	for _, container := range containers {
		if container.InstanceID() == instanceID && container.Role() == "maintenance" {
			return fmt.Errorf("%w: %s", ErrMaintenanceActive, container.ID)
		}
	}
	return nil
}

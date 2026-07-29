package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"github.com/not0721here/l4d2-control-panel/internal/docker"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
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
	legacy := make([]string, 0)
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
			needsUpgrade := container.Labels[docker.GameLogMountsLabel] != docker.GameLogMountsVersion
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
			if needsUpgrade {
				legacy = append(legacy, instance.ID)
			}
			if container.State == "running" && s.health != nil && !needsUpgrade {
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
	for _, instanceID := range legacy {
		if err := s.Rebuild(ctx, instanceID); err != nil {
			return nil, fmt.Errorf("upgrade legacy game container %s: %w", instanceID, err)
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
type LogPreparer interface {
	Prepare(context.Context, string) error
}
type Service struct {
	repo                Repository
	engine              Engine
	ports               PortChecker
	dataRoot            string
	health              HealthChecker
	space               SpaceChecker
	provisioner         Provisioner
	logPreparer         LogPreparer
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
func WithLogPreparer(preparer LogPreparer) Option {
	return func(s *Service) { s.logPreparer = preparer }
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
	jobs.Logf(ctx, "lifecycle", joblogs.Info, "lifecycle start requested instance=%s", id)
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
		jobs.Logf(ctx, "lifecycle", joblogs.Info, "disk preflight instance=%s available=%s required=%s", id, jobs.FormatBytes(int64(available)), jobs.FormatBytes(int64(s.minimumInstallBytes)))
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
		jobs.Logf(ctx, "lifecycle", joblogs.Info, "port preflight completed instance=%s ports=%v", id, declaredPorts)
	}
	if v.ContainerID == "" {
		base := filepath.Join(s.dataRoot, "instances", v.ID)
		for _, dir := range []string{"game", "private", "backups", "console"} {
			if err := os.MkdirAll(filepath.Join(base, dir), 0750); err != nil {
				return err
			}
		}
		needsProvision := s.provisioner != nil && (v.ActualState == domain.StateUninstalled || v.PackageSourceID != "" || v.PackageSourceRepository != "" || v.PackageVersion != v.SelectedPackageID)
		if s.provisioner != nil && v.SelectedPackageID == "" && v.PackageSourceID == "" && v.PackageSourceRepository == "" {
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
			jobs.Logf(ctx, "lifecycle", joblogs.Info, "provisioning package instance=%s selected_package=%s source_id=%s source_repository=%s", id, v.SelectedPackageID, v.PackageSourceID, v.PackageSourceRepository)
			if err := s.provisioner.Prepare(ctx, v); err != nil {
				return s.fault(ctx, v, err)
			}
			v, err = s.repo.Instance(ctx, id)
			if err != nil {
				return err
			}
		}
		if s.logPreparer != nil {
			if err := s.logPreparer.Prepare(ctx, v.ID); err != nil {
				return s.fault(ctx, v, err)
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
		jobs.Logf(ctx, "lifecycle", joblogs.Info, "container created instance=%s container=%s image=%s", id, containerID, v.RuntimeImage)
		if err := s.repo.UpdateInstance(ctx, v); err != nil {
			return err
		}
	}
	if err := s.engine.Start(ctx, v.ContainerID); err != nil {
		return s.fault(ctx, v, err)
	}
	jobs.Logf(ctx, "lifecycle", joblogs.Info, "container started instance=%s container=%s", id, v.ContainerID)
	if s.health != nil {
		if err := s.health.Wait(ctx, v); err != nil {
			return s.fault(ctx, v, err)
		}
	}
	v.DesiredState, v.ActualState = domain.StateRunning, domain.StateRunning
	if err := s.repo.UpdateInstance(ctx, v); err != nil {
		return err
	}
	jobs.Logf(ctx, "lifecycle", joblogs.Info, "lifecycle start completed instance=%s state=running", id)
	return nil
}
func (s *Service) Stop(ctx context.Context, id string) error {
	jobs.Logf(ctx, "lifecycle", joblogs.Info, "lifecycle stop requested instance=%s", id)
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
	jobs.Logf(ctx, "lifecycle", joblogs.Info, "supervisor stop requested instance=%s container=%s", id, v.ContainerID)
	if err := s.engine.Stop(ctx, v.ContainerID, 15); err != nil {
		return s.fault(ctx, v, err)
	}
	v.DesiredState, v.ActualState = domain.StateStopped, domain.StateStopped
	if err := s.repo.UpdateInstance(ctx, v); err != nil {
		return err
	}
	jobs.Logf(ctx, "lifecycle", joblogs.Info, "lifecycle stop completed instance=%s state=stopped", id)
	return nil
}
func (s *Service) Restart(ctx context.Context, id string) error {
	jobs.Logf(ctx, "lifecycle", joblogs.Info, "lifecycle restart requested instance=%s", id)
	ctx, release, err := s.lease(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := s.Stop(ctx, id); err != nil {
		return err
	}
	if err := s.Start(ctx, id); err != nil {
		return err
	}
	jobs.Logf(ctx, "lifecycle", joblogs.Info, "lifecycle restart completed instance=%s", id)
	return nil
}
func (s *Service) Rebuild(ctx context.Context, id string) error {
	jobs.Logf(ctx, "lifecycle", joblogs.Info, "container rebuild requested instance=%s", id)
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
	if s.logPreparer != nil {
		if err := s.logPreparer.Prepare(ctx, instance.ID); err != nil {
			return fmt.Errorf("prepare game logs for rebuild: %w", err)
		}
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
		jobs.Logf(ctx, "lifecycle", joblogs.Info, "removing container for rebuild instance=%s container=%s", id, instance.ContainerID)
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
	if s.logPreparer != nil {
		if err := s.logPreparer.Prepare(ctx, instance.ID); err != nil {
			return s.fault(ctx, instance, err)
		}
	}
	spec, err := docker.BuildContainerSpec(s.dataRoot, instance)
	if err != nil {
		return s.fault(ctx, instance, err)
	}
	containerID, err := s.engine.Create(ctx, spec)
	if err != nil {
		return s.fault(ctx, instance, err)
	}
	instance.ContainerID = containerID
	jobs.Logf(ctx, "lifecycle", joblogs.Info, "replacement container created instance=%s container=%s", id, containerID)
	instance.DesiredState, instance.ActualState = domain.StateStopped, domain.StateStopped
	return s.repo.UpdateInstance(ctx, instance)
}
func (s *Service) Delete(ctx context.Context, id string, deleteData bool) error {
	jobs.Logf(ctx, "lifecycle", joblogs.Info, "instance deletion requested instance=%s delete_data=%t", id, deleteData)
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
		jobs.Logf(ctx, "lifecycle", joblogs.Info, "removing instance container instance=%s container=%s", id, instance.ContainerID)
		if err := s.engine.Remove(ctx, instance.ContainerID); err != nil {
			return err
		}
	}
	if err := s.repo.DeleteInstance(ctx, id); err != nil {
		return err
	}
	jobs.Logf(ctx, "lifecycle", joblogs.Info, "instance record deleted instance=%s", id)
	if deleteData {
		if filepath.Base(id) != id {
			return errors.New("invalid instance id")
		}
		if err := os.RemoveAll(filepath.Join(s.dataRoot, "instances", id)); err != nil {
			return err
		}
		jobs.Logf(ctx, "lifecycle", joblogs.Info, "instance data deleted instance=%s relative_path=instances/%s", id, id)
	}
	jobs.Logf(ctx, "lifecycle", joblogs.Info, "instance deletion completed instance=%s delete_data=%t", id, deleteData)
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

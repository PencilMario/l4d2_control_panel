package updates

import (
	"context"
	"errors"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
)

type InstanceRepository interface {
	Instance(context.Context, string) (domain.Instance, error)
	UpdateInstance(context.Context, domain.Instance) error
}
type GameUpdater interface {
	HasMaintenance(context.Context, string) (bool, error)
	UpdateGame(context.Context, string, domain.Instance) error
}
type PrivateApplier interface {
	Apply(context.Context, string) error
}
type PackageSource interface {
	Get(string) (content.PackageVersion, error)
}
type SourceSynchronizer interface {
	SyncLatest(context.Context, string) (content.PackageVersion, error)
}
type ReinstallOptions struct {
	Game    bool
	Package bool
}
type GameCoordinator struct {
	Root      string
	Instances InstanceRepository
	Lifecycle Lifecycle
	Updater   GameUpdater
	Private   PrivateApplier
	Packages  PackageSource
	Sources   SourceSynchronizer
	Deployer  Deployer
}

func (c GameCoordinator) Update(ctx context.Context, id string) error {
	return c.Reinstall(ctx, id, ReinstallOptions{Game: true})
}

func (c GameCoordinator) Reinstall(ctx context.Context, id string, options ReinstallOptions) error {
	jobs.Logf(ctx, "update", joblogs.Info, "instance reinstall started instance=%s game=%t package=%t", id, options.Game, options.Package)
	if !options.Game && !options.Package {
		return errors.New("at least one reinstall target is required")
	}
	instance, err := c.Instances.Instance(ctx, id)
	if err != nil {
		return err
	}
	var item content.PackageVersion
	if options.Package {
		switch {
		case instance.PackageSourceID != "":
			if c.Sources == nil {
				return c.fault(ctx, id, errors.New("source package sync unavailable"))
			}
			item, err = c.Sources.SyncLatest(ctx, instance.PackageSourceID)
		case instance.SelectedPackageID != "":
			if c.Packages == nil {
				return c.fault(ctx, id, errors.New("package reinstall unavailable"))
			}
			item, err = c.Packages.Get(instance.SelectedPackageID)
		default:
			return c.fault(ctx, id, errors.New("instance has no selected package"))
		}
		if err != nil {
			return c.fault(ctx, id, err)
		}
		jobs.Logf(ctx, "update", joblogs.Info, "selected package instance=%s package_id=%s version=%s file=%s size=%s", id, item.ID, item.Version, item.Filename, jobs.FormatBytes(item.Size))
	}
	maintenance := false
	if options.Game {
		maintenance, err = c.Updater.HasMaintenance(ctx, id)
		if err != nil {
			return err
		}
	}
	resume := instance.DesiredState == domain.StateRunning
	needsStop := instance.ActualState == domain.StateRunning || instance.ActualState == domain.StateStarting || instance.ActualState == domain.StateInstalling
	if !maintenance && needsStop {
		jobs.Logf(ctx, "update", joblogs.Info, "stopping instance for reinstall instance=%s", id)
		if err := c.Lifecycle.Stop(ctx, id); err != nil {
			return err
		}
	}
	instance, err = c.Instances.Instance(ctx, id)
	if err != nil {
		return err
	}
	if resume {
		instance.DesiredState = domain.StateRunning
	}
	instance.ActualState = domain.StateUpdating
	if err := c.Instances.UpdateInstance(ctx, instance); err != nil {
		return err
	}
	if options.Game {
		jobs.Logf(ctx, "update", joblogs.Info, "updating game files instance=%s", id)
		if err := c.Updater.UpdateGame(ctx, c.Root, instance); err != nil {
			return c.fault(ctx, id, err)
		}
	}
	var transaction Deployment
	if options.Package {
		if c.Deployer == nil {
			return c.fault(ctx, id, errors.New("package reinstall unavailable"))
		}
		transaction, err = c.Deployer.Begin(ctx, id, item.ArchivePath, item.Version, Full)
		if err != nil {
			return c.fault(ctx, id, err)
		}
	} else if err := c.Private.Apply(ctx, id); err != nil {
		return c.fault(ctx, id, err)
	}
	latest, err := c.Instances.Instance(ctx, id)
	if err != nil {
		return err
	}
	if latest.DesiredState == domain.StateRunning {
		jobs.Logf(ctx, "update", joblogs.Info, "restarting instance after reinstall instance=%s", id)
		if err := c.Lifecycle.Start(ctx, id); err != nil {
			if transaction != nil {
				_ = transaction.Rollback()
			}
			return c.fault(ctx, id, err)
		}
	} else {
		latest.ActualState = domain.StateStopped
		if err := c.Instances.UpdateInstance(ctx, latest); err != nil {
			if transaction != nil {
				_ = transaction.Rollback()
			}
			return err
		}
	}
	if transaction != nil {
		if err := transaction.Commit(); err != nil {
			if latest.DesiredState == domain.StateRunning {
				stopErr := c.Lifecycle.Stop(ctx, id)
				if stopErr != nil {
					return c.fault(ctx, id, errors.Join(err, stopErr))
				}
				rollbackErr := transaction.Rollback()
				startErr := c.Lifecycle.Start(ctx, id)
				return c.fault(ctx, id, errors.Join(err, rollbackErr, startErr))
			}
			return c.fault(ctx, id, errors.Join(err, transaction.Rollback()))
		}
		latest, err = c.Instances.Instance(ctx, id)
		if err != nil {
			return err
		}
		latest.PackageVersion = item.ID
		if err := c.Instances.UpdateInstance(ctx, latest); err != nil {
			return err
		}
		jobs.Logf(ctx, "update", joblogs.Info, "instance reinstall completed instance=%s game=%t package=%t package_id=%s", id, options.Game, options.Package, item.ID)
		return nil
	}
	jobs.Logf(ctx, "update", joblogs.Info, "instance reinstall completed instance=%s game=%t package=%t", id, options.Game, options.Package)
	return nil
}

func (c GameCoordinator) fault(ctx context.Context, id string, cause error) error {
	jobs.Logf(ctx, "update", joblogs.Error, "instance reinstall failed instance=%s error=%q", id, cause.Error())
	instance, err := c.Instances.Instance(ctx, id)
	if err != nil {
		return cause
	}
	instance.ActualState = domain.StateFaulted
	_ = c.Instances.UpdateInstance(ctx, instance)
	return cause
}

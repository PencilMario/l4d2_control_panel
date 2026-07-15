package updates

import (
	"context"
	"errors"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
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
	Deployer  Deployer
}

func (c GameCoordinator) Update(ctx context.Context, id string) error {
	return c.Reinstall(ctx, id, ReinstallOptions{Game: true})
}

func (c GameCoordinator) Reinstall(ctx context.Context, id string, options ReinstallOptions) error {
	if !options.Game && !options.Package {
		return errors.New("at least one reinstall target is required")
	}
	instance, err := c.Instances.Instance(ctx, id)
	if err != nil {
		return err
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
		if err := c.Updater.UpdateGame(ctx, c.Root, instance); err != nil {
			return c.fault(ctx, id, err)
		}
	}
	var transaction Deployment
	if options.Package {
		if instance.SelectedPackageID == "" {
			return c.fault(ctx, id, errors.New("instance has no selected package"))
		}
		if c.Packages == nil || c.Deployer == nil {
			return c.fault(ctx, id, errors.New("package reinstall unavailable"))
		}
		item, err := c.Packages.Get(instance.SelectedPackageID)
		if err != nil {
			return c.fault(ctx, id, err)
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
		latest.PackageVersion = latest.SelectedPackageID
		return c.Instances.UpdateInstance(ctx, latest)
	}
	return nil
}

func (c GameCoordinator) fault(ctx context.Context, id string, cause error) error {
	instance, err := c.Instances.Instance(ctx, id)
	if err != nil {
		return cause
	}
	instance.ActualState = domain.StateFaulted
	_ = c.Instances.UpdateInstance(ctx, instance)
	return cause
}

package updates

import (
	"context"
	"errors"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

type Lifecycle interface {
	Start(context.Context, string) error
	Stop(context.Context, string) error
}
type Deployer interface {
	Begin(context.Context, string, string, string, Mode) (Deployment, error)
}
type PackageInstanceRepository interface {
	Instance(context.Context, string) (domain.Instance, error)
	UpdateInstance(context.Context, domain.Instance) error
}
type Coordinator struct {
	Lifecycle Lifecycle
	Deployer  Deployer
	Instances PackageInstanceRepository
}

func (c Coordinator) ApplyPackage(ctx context.Context, instanceID string, item content.PackageVersion, mode Mode) error {
	instance, err := c.Instances.Instance(ctx, instanceID)
	if err != nil {
		return err
	}
	if mode == Hot {
		transaction, err := c.Deployer.Begin(ctx, instanceID, item.ArchivePath, item.Version, mode)
		if err != nil {
			return err
		}
		if err := transaction.Commit(); err != nil {
			return errors.Join(err, transaction.Rollback())
		}
		return c.markApplied(ctx, instanceID, item.ID)
	}
	if mode != Full {
		return errors.New("unsupported update mode")
	}
	resume := instance.DesiredState == domain.StateRunning || instance.ActualState == domain.StateRunning || instance.ActualState == domain.StateStarting || instance.ActualState == domain.StateInstalling
	wasActive := instance.ActualState == domain.StateRunning || instance.ActualState == domain.StateStarting || instance.ActualState == domain.StateInstalling
	if wasActive {
		if err := c.Lifecycle.Stop(ctx, instanceID); err != nil {
			return err
		}
	}
	transaction, deployErr := c.Deployer.Begin(ctx, instanceID, item.ArchivePath, item.Version, mode)
	if deployErr != nil {
		if wasActive {
			return errors.Join(deployErr, c.Lifecycle.Start(ctx, instanceID))
		}
		return deployErr
	}
	if resume {
		if startErr := c.Lifecycle.Start(ctx, instanceID); startErr != nil {
			return c.rollbackStarted(ctx, instanceID, transaction, startErr)
		}
	}
	if commitErr := transaction.Commit(); commitErr != nil {
		if resume {
			return c.rollbackStarted(ctx, instanceID, transaction, commitErr)
		}
		return errors.Join(commitErr, transaction.Rollback())
	}
	return c.markApplied(ctx, instanceID, item.ID)
}

func (c Coordinator) rollbackStarted(ctx context.Context, instanceID string, transaction Deployment, cause error) error {
	stopErr := c.Lifecycle.Stop(ctx, instanceID)
	if stopErr != nil {
		return errors.Join(cause, stopErr)
	}
	rollbackErr := transaction.Rollback()
	if rollbackErr != nil {
		return errors.Join(cause, rollbackErr)
	}
	return errors.Join(cause, c.Lifecycle.Start(ctx, instanceID))
}

func (c Coordinator) markApplied(ctx context.Context, instanceID, packageID string) error {
	instance, err := c.Instances.Instance(ctx, instanceID)
	if err != nil {
		return err
	}
	instance.SelectedPackageID = packageID
	instance.PackageVersion = packageID
	return c.Instances.UpdateInstance(ctx, instance)
}

package updates

import (
	"context"
	"errors"
	"github.com/not0721here/l4d2-control-panel/internal/content"
)

type Lifecycle interface {
	Start(context.Context, string) error
	Stop(context.Context, string) error
}
type Deployer interface {
	Begin(context.Context, string, string, string, Mode) (Deployment, error)
}
type Coordinator struct {
	Lifecycle Lifecycle
	Deployer  Deployer
}

func (c Coordinator) ApplyPackage(ctx context.Context, instanceID string, item content.PackageVersion, mode Mode) error {
	if mode == Hot {
		transaction, err := c.Deployer.Begin(ctx, instanceID, item.ArchivePath, item.Version, mode)
		if err != nil {
			return err
		}
		if err := transaction.Commit(); err != nil {
			return errors.Join(err, transaction.Rollback())
		}
		return nil
	}
	if mode != Full {
		return errors.New("unsupported update mode")
	}
	if err := c.Lifecycle.Stop(ctx, instanceID); err != nil {
		return err
	}
	transaction, deployErr := c.Deployer.Begin(ctx, instanceID, item.ArchivePath, item.Version, mode)
	if deployErr != nil {
		return errors.Join(deployErr, c.Lifecycle.Start(ctx, instanceID))
	}
	startErr := c.Lifecycle.Start(ctx, instanceID)
	if startErr == nil {
		if commitErr := transaction.Commit(); commitErr == nil {
			return nil
		} else {
			return c.rollbackStarted(ctx, instanceID, transaction, commitErr)
		}
	}
	return c.rollbackStarted(ctx, instanceID, transaction, startErr)
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

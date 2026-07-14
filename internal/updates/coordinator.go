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
	Apply(context.Context, string, string, string, Mode) error
}
type Coordinator struct {
	Lifecycle Lifecycle
	Deployer  Deployer
}

func (c Coordinator) ApplyPackage(ctx context.Context, instanceID string, item content.PackageVersion, mode Mode) error {
	if mode == Hot {
		return c.Deployer.Apply(ctx, instanceID, item.ArchivePath, item.Version, mode)
	}
	if mode != Full {
		return errors.New("unsupported update mode")
	}
	if err := c.Lifecycle.Stop(ctx, instanceID); err != nil {
		return err
	}
	deployErr := c.Deployer.Apply(ctx, instanceID, item.ArchivePath, item.Version, mode)
	startErr := c.Lifecycle.Start(ctx, instanceID)
	if deployErr != nil && startErr != nil {
		return errors.Join(deployErr, startErr)
	}
	if deployErr != nil {
		return deployErr
	}
	return startErr
}

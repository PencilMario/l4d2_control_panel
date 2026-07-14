package updates

import (
	"context"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

type InstanceRepository interface {
	Instance(context.Context, string) (domain.Instance, error)
	UpdateInstance(context.Context, domain.Instance) error
}
type GameUpdater interface {
	UpdateGame(context.Context, string, domain.Instance) error
}
type PrivateApplier interface {
	Apply(context.Context, string) error
}
type GameCoordinator struct {
	Root      string
	Instances InstanceRepository
	Lifecycle Lifecycle
	Updater   GameUpdater
	Private   PrivateApplier
}

func (c GameCoordinator) Update(ctx context.Context, id string) error {
	instance, err := c.Instances.Instance(ctx, id)
	if err != nil {
		return err
	}
	if err := c.Lifecycle.Stop(ctx, id); err != nil {
		return err
	}
	if err := c.Updater.UpdateGame(ctx, c.Root, instance); err != nil {
		return c.fault(ctx, instance, err)
	}
	if err := c.Private.Apply(ctx, id); err != nil {
		return c.fault(ctx, instance, err)
	}
	if err := c.Lifecycle.Start(ctx, id); err != nil {
		return c.fault(ctx, instance, err)
	}
	return nil
}
func (c GameCoordinator) fault(ctx context.Context, instance domain.Instance, cause error) error {
	instance.ActualState = domain.StateFaulted
	_ = c.Instances.UpdateInstance(ctx, instance)
	return cause
}

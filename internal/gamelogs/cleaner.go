package gamelogs

import (
	"context"
	"errors"
	"fmt"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

type CleanerStore interface {
	Instances(context.Context) ([]domain.Instance, error)
	GameLogMaxFileSizeMB() (int, error)
}

type logMaintainer interface {
	Maintain(context.Context, string, int, int64) (CleanupResult, error)
}

// Cleaner runs the game-log policy synchronously as part of the global clear task.
type Cleaner struct {
	store   CleanerStore
	manager logMaintainer
}

func NewCleaner(store CleanerStore, manager logMaintainer) *Cleaner {
	return &Cleaner{store: store, manager: manager}
}

func (c *Cleaner) Cleanup(ctx context.Context, retentionDays int) error {
	instances, err := c.store.Instances(ctx)
	if err != nil {
		return fmt.Errorf("list game log instances: %w", err)
	}
	maxFileSizeMB, err := c.store.GameLogMaxFileSizeMB()
	if err != nil {
		return fmt.Errorf("read game log max file size: %w", err)
	}

	maxFileSizeBytes := int64(maxFileSizeMB) << 20
	var cleanupErrs []error
	for _, instance := range instances {
		_, err := c.manager.Maintain(ctx, instance.ID, retentionDays, maxFileSizeBytes)
		if err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("instance %s: %w", instance.ID, err))
		}
	}
	return errors.Join(cleanupErrs...)
}

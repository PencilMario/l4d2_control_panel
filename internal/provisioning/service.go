package provisioning

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/updates"
)

type PackageSource interface {
	Get(string) (content.PackageVersion, error)
}

type Deployer interface {
	Apply(context.Context, string, string, string, updates.Mode) error
}

type InstanceRepository interface {
	Instance(context.Context, string) (domain.Instance, error)
	UpdateInstance(context.Context, domain.Instance) error
}

type SharedStateSource interface {
	SharedGameState(context.Context) (domain.SharedGameState, error)
}

type SharedOverlay interface {
	Ensure(context.Context, string, string) error
}

type Service struct {
	Root        string
	Packages    PackageSource
	Deployer    Deployer
	Instances   InstanceRepository
	SharedState SharedStateSource
	Overlay     SharedOverlay
}

func (s Service) Prepare(ctx context.Context, instance domain.Instance) error {
	if instance.SelectedPackageID == "" {
		return errors.New("instance package is required")
	}
	item, err := s.Packages.Get(instance.SelectedPackageID)
	if err != nil {
		return err
	}
	if s.SharedState == nil || s.Overlay == nil {
		return errors.New("shared game services are unavailable")
	}
	state, stateErr := s.SharedState.SharedGameState(ctx)
	if stateErr != nil {
		return fmt.Errorf("shared game state is unavailable: %w", stateErr)
	}
	if state.MigrationState != "ready" || state.ActiveReleaseID == "" {
		return errors.New("shared game is not ready")
	}
	if err := ensureSharedGameLink(s.Root, instance.ID); err != nil {
		return err
	}
	if err := s.Overlay.Ensure(ctx, instance.ID, state.ActiveReleaseID); err != nil {
		return err
	}
	if err := s.Deployer.Apply(ctx, instance.ID, item.ArchivePath, item.Version, updates.Full); err != nil {
		return err
	}
	latest, err := s.Instances.Instance(ctx, instance.ID)
	if err != nil {
		return err
	}
	latest.PackageVersion = item.ID
	return s.Instances.UpdateInstance(ctx, latest)
}

func ensureSharedGameLink(root, instanceID string) error {
	base := filepath.Join(root, "instances", instanceID)
	game := filepath.Join(base, "game")
	if info, err := os.Lstat(game); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		entries, readErr := os.ReadDir(game)
		if readErr != nil {
			return readErr
		}
		if len(entries) != 0 {
			return errors.New("instance game directory is not empty; migration is required")
		}
		if err := os.Remove(game); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Join(base, "overlay", "upper"), 0770); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(base, "overlay", "work"), 0770); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(base, "overlay", "merged"), 0770); err != nil {
		return err
	}
	return os.Symlink(filepath.Join("overlay", "merged"), game)
}

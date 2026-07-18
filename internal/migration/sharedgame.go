package migration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/maintenance"
)

type InstanceRepository interface {
	Instances(context.Context) ([]domain.Instance, error)
	SaveSharedGameState(context.Context, domain.SharedGameState) error
}

type Installer interface {
	InstallSharedGame(context.Context, string, string, domain.Instance, bool) error
}

type Publisher interface {
	StagePath(string, string) string
	Publish(context.Context, string) error
	Activate(context.Context, string) error
}

type Layout interface {
	Prepare(context.Context, string, string) error
	Rollback(context.Context, string, string) error
}

type Reconciler interface {
	Switch(context.Context, domain.Instance, string, string) error
}

type SharedGameService struct {
	Root       string
	Instances  InstanceRepository
	Installer  Installer
	Publisher  Publisher
	Layout     Layout
	Reconciler Reconciler
	Gate       *maintenance.Gate
}

func (s SharedGameService) Migrate(ctx context.Context) error {
	if s.Gate != nil {
		var release func()
		var err error
		ctx, release, err = s.Gate.ExclusiveContext(ctx)
		if err != nil {
			return err
		}
		defer release()
	}
	instances, err := s.Instances.Instances(ctx)
	if err != nil {
		return err
	}
	if len(instances) == 0 {
		return errors.New("shared game migration requires at least one instance")
	}
	for _, instance := range instances {
		if instance.ActualState != domain.StateStopped && instance.ActualState != domain.StateUninstalled && instance.ActualState != domain.StateFaulted && instance.ActualState != domain.StateOrphaned {
			return fmt.Errorf("instance %s must be stopped before migration", instance.ID)
		}
	}
	migrationID := uuid.NewString()
	releaseID := uuid.NewString()
	state := domain.SharedGameState{MigrationState: "installing", OperationID: migrationID, OperationStage: "installing"}
	if err := s.Instances.SaveSharedGameState(ctx, state); err != nil {
		return err
	}
	stage := s.Publisher.StagePath(s.Root, releaseID)
	if err := s.Installer.InstallSharedGame(ctx, s.Root, stage, instances[0], false); err != nil {
		return s.fail(ctx, state, err)
	}
	if err := s.Publisher.Publish(ctx, releaseID); err != nil {
		return s.fail(ctx, state, err)
	}
	state.OperationStage = "instances"
	if err := s.Instances.SaveSharedGameState(ctx, state); err != nil {
		return err
	}
	prepared := make([]domain.Instance, 0, len(instances))
	for _, instance := range instances {
		if err := s.Layout.Prepare(ctx, instance.ID, migrationID); err != nil {
			return s.rollback(ctx, state, prepared, migrationID, err)
		}
		prepared = append(prepared, instance)
		if err := s.Reconciler.Switch(ctx, instance, "", releaseID); err != nil {
			return s.rollback(ctx, state, prepared, migrationID, err)
		}
	}
	if err := s.Publisher.Activate(ctx, releaseID); err != nil {
		return s.rollback(ctx, state, prepared, migrationID, err)
	}
	state.ActiveReleaseID = releaseID
	state.MigrationState = "ready"
	state.OperationID = ""
	state.OperationStage = ""
	return s.Instances.SaveSharedGameState(ctx, state)
}

func (s SharedGameService) rollback(ctx context.Context, state domain.SharedGameState, prepared []domain.Instance, migrationID string, cause error) error {
	var rollbackErr error
	for index := len(prepared) - 1; index >= 0; index-- {
		rollbackErr = errors.Join(rollbackErr, s.Layout.Rollback(ctx, prepared[index].ID, migrationID))
	}
	return s.fail(ctx, state, errors.Join(cause, rollbackErr))
}

func (s SharedGameService) fail(ctx context.Context, state domain.SharedGameState, cause error) error {
	state.MigrationState = "failed"
	state.OperationStage = "failed"
	_ = s.Instances.SaveSharedGameState(ctx, state)
	return cause
}

type FilesystemLayout struct {
	Root string
}

func (l FilesystemLayout) Prepare(_ context.Context, instanceID, migrationID string) error {
	base := filepath.Join(l.Root, "instances", instanceID)
	game := filepath.Join(base, "game")
	legacy := filepath.Join(base, "legacy-game."+migrationID)
	if info, err := os.Lstat(game); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("instance game path is already a symbolic link")
		}
		if err := os.Rename(game, legacy); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Join(base, "overlay"), 0o770); err != nil {
		return err
	}
	return os.Symlink(filepath.Join("overlay", "merged"), game)
}

func (l FilesystemLayout) Rollback(_ context.Context, instanceID, migrationID string) error {
	base := filepath.Join(l.Root, "instances", instanceID)
	game := filepath.Join(base, "game")
	legacy := filepath.Join(base, "legacy-game."+migrationID)
	if info, err := os.Lstat(game); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if err := os.Remove(game); err != nil {
			return err
		}
	}
	if _, err := os.Stat(legacy); err == nil {
		return os.Rename(legacy, game)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

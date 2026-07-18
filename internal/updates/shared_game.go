package updates

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/maintenance"
)

type SharedGameRepository interface {
	Instances(context.Context) ([]domain.Instance, error)
	SharedGameState(context.Context) (domain.SharedGameState, error)
	SaveSharedGameState(context.Context, domain.SharedGameState) error
}

type SharedGamePlayers interface {
	PlayerCount(context.Context, string) (int, error)
}

type SharedGameInstaller interface {
	InstallSharedGame(context.Context, string, string, domain.Instance, bool) error
}

type SharedGameReconciler interface {
	Switch(context.Context, domain.Instance, string, string) error
}

type SharedGameUnmounted interface {
	Unmount(context.Context, domain.Instance, string) error
}

type SharedGameCoordinator struct {
	Root       string
	Instances  SharedGameRepository
	Players    SharedGamePlayers
	Installer  SharedGameInstaller
	Reconciler SharedGameReconciler
	Lifecycle  Lifecycle
	Gate       *maintenance.Gate
	WaitDelay  time.Duration
}

func (c SharedGameCoordinator) Update(ctx context.Context, onlinePolicy string) error {
	if onlinePolicy != "force" && onlinePolicy != "skip" && onlinePolicy != "wait" {
		return errors.New("online policy must be skip, wait or force")
	}
	instances, err := c.Instances.Instances(ctx)
	if err != nil {
		return err
	}
	if len(instances) == 0 {
		return errors.New("shared game update requires at least one instance runtime image")
	}
	if err := c.waitForPlayers(ctx, instances, onlinePolicy); err != nil {
		return err
	}
	gate := c.Gate
	if gate == nil {
		gate = maintenance.NewGate()
	}
	ctx, release, err := gate.ExclusiveContext(ctx)
	if err != nil {
		return err
	}
	defer release()
	instances, err = c.Instances.Instances(ctx)
	if err != nil {
		return err
	}
	if err := c.waitForPlayers(ctx, instances, onlinePolicy); err != nil {
		return err
	}
	state, err := c.Instances.SharedGameState(ctx)
	if err != nil {
		return err
	}
	activeRelease := state.ActiveReleaseID
	state.OperationID = uuid.NewString()
	state.OperationStage = "stopping"
	if err := c.Instances.SaveSharedGameState(ctx, state); err != nil {
		return err
	}
	if activeRelease == "" || state.MigrationState != "ready" {
		return c.fail(ctx, state, errors.New("shared game release is not ready"))
	}
	active := make([]domain.Instance, 0, len(instances))
	for _, instance := range instances {
		if instance.ActualState != domain.StateRunning && instance.ActualState != domain.StateStarting && instance.ActualState != domain.StateInstalling {
			continue
		}
		if err := c.Lifecycle.Stop(ctx, instance.ID); err != nil {
			return err
		}
		active = append(active, instance)
	}
	state.OperationStage = "unmounting"
	if err := c.Instances.SaveSharedGameState(ctx, state); err != nil {
		return err
	}
	if unmount, ok := c.Reconciler.(SharedGameUnmounted); ok {
		for _, instance := range instances {
			if err := unmount.Unmount(ctx, instance, activeRelease); err != nil {
				return c.fail(ctx, state, err)
			}
		}
	}
	state.OperationStage = "validating"
	if err := c.Instances.SaveSharedGameState(ctx, state); err != nil {
		return err
	}
	activePath := filepath.Join(c.Root, "game", "releases", activeRelease)
	if err := c.Installer.InstallSharedGame(ctx, c.Root, activePath, instances[0], true); err != nil {
		return c.fail(ctx, state, err)
	}
	state.OperationStage = "rebuilding"
	if err := c.Instances.SaveSharedGameState(ctx, state); err != nil {
		return err
	}
	for _, instance := range instances {
		if err := c.Reconciler.Switch(ctx, instance, activeRelease, activeRelease); err != nil {
			return c.fail(ctx, state, err)
		}
	}
	if err := pruneInactiveReleases(c.Root, activeRelease); err != nil {
		return c.fail(ctx, state, err)
	}
	state.PreviousReleaseID = ""
	state.ActiveReleaseID = activeRelease
	state.OperationStage = "restarting"
	if err := c.Instances.SaveSharedGameState(ctx, state); err != nil {
		return err
	}
	for _, instance := range active {
		latest := instance
		for _, candidate := range instances {
			if candidate.ID == instance.ID {
				latest = candidate
				break
			}
		}
		if latest.DesiredState == domain.StateRunning {
			if err := c.Lifecycle.Start(ctx, latest.ID); err != nil {
				return err
			}
		}
	}
	state.OperationID = ""
	state.OperationStage = ""
	return c.Instances.SaveSharedGameState(ctx, state)
}

func (c SharedGameCoordinator) fail(ctx context.Context, state domain.SharedGameState, cause error) error {
	state.MigrationState = "failed"
	state.OperationStage = "failed"
	_ = c.Instances.SaveSharedGameState(ctx, state)
	return cause
}

func pruneInactiveReleases(root, activeID string) error {
	directory := filepath.Join(root, "game", "releases")
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == activeID || !entry.IsDir() {
			continue
		}
		if err := os.RemoveAll(filepath.Join(directory, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (c SharedGameCoordinator) waitForPlayers(ctx context.Context, instances []domain.Instance, policy string) error {
	if policy == "force" || c.Players == nil {
		return nil
	}
	delay := c.WaitDelay
	if delay <= 0 {
		delay = time.Minute
	}
	for {
		var blocked []string
		for _, instance := range instances {
			if instance.ActualState != domain.StateRunning && instance.ActualState != domain.StateStarting {
				continue
			}
			count, err := c.Players.PlayerCount(ctx, instance.ID)
			if err != nil {
				blocked = append(blocked, instance.Name+": query failed")
			} else if count > 0 {
				blocked = append(blocked, fmt.Sprintf("%s: %d players", instance.Name, count))
			}
		}
		if len(blocked) == 0 {
			return nil
		}
		if policy == "skip" {
			return fmt.Errorf("shared game update skipped: %v", blocked)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

type FilesystemGamePublisher struct {
	Root string
}

func (p FilesystemGamePublisher) StagePath(_ string, releaseID string) string {
	return filepath.Join(p.Root, "game", "staging", releaseID)
}

func (p FilesystemGamePublisher) Publish(_ context.Context, releaseID string) error {
	stage := p.StagePath(p.Root, releaseID)
	if err := os.Chown(stage, os.Getuid(), os.Getgid()); err != nil {
		return err
	}
	release := filepath.Join(p.Root, "game", "releases", releaseID)
	if err := os.MkdirAll(filepath.Dir(release), 0o750); err != nil {
		return err
	}
	return os.Rename(stage, release)
}

func (p FilesystemGamePublisher) Activate(_ context.Context, releaseID string) error {
	gameRoot := filepath.Join(p.Root, "game")
	temporary := filepath.Join(gameRoot, ".current-"+releaseID)
	if err := os.Symlink(filepath.Join("releases", releaseID), temporary); err != nil {
		return err
	}
	return os.Rename(temporary, filepath.Join(gameRoot, "current"))
}

package provisioning

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

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
	Instances(context.Context) ([]domain.Instance, error)
	UpdateInstance(context.Context, domain.Instance) error
}

type SharedStateSource interface {
	SharedGameState(context.Context) (domain.SharedGameState, error)
}

type SharedOverlay interface {
	Ensure(context.Context, string, string) error
}

type AcceleratorEnsurer interface {
	Ensure(context.Context, domain.Instance) error
}

type Service struct {
	Root     string
	Packages PackageSource
	Sources  interface {
		GitHubSource(context.Context, string) (domain.GitHubSource, error)
	}
	Deployer    Deployer
	Instances   InstanceRepository
	SharedState SharedStateSource
	Overlay     SharedOverlay
	Accelerator AcceleratorEnsurer
}

func (s Service) RecoverOverlays(ctx context.Context) error {
	if s.SharedState == nil || s.Overlay == nil || s.Instances == nil {
		return errors.New("shared game services are unavailable")
	}
	state, err := s.SharedState.SharedGameState(ctx)
	releaseID := ""
	if err == nil && state.MigrationState == "ready" && state.ActiveReleaseID != "" {
		releaseID = state.ActiveReleaseID
	} else {
		releaseID, err = currentReleaseID(s.Root)
		if err != nil {
			return errors.New("shared game is not ready")
		}
	}
	instances, err := s.Instances.Instances(ctx)
	if err != nil {
		return fmt.Errorf("list instances for overlay recovery: %w", err)
	}
	for _, instance := range instances {
		if err := s.Overlay.Ensure(ctx, instance.ID, releaseID); err != nil {
			return fmt.Errorf("recover instance overlay %s: %w", instance.ID, err)
		}
	}
	return nil
}

var releaseIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func currentReleaseID(root string) (string, error) {
	current := filepath.Join(root, "game", "current")
	target, err := os.Readlink(current)
	if err != nil {
		return "", err
	}
	releaseID := filepath.Base(target)
	if !releaseIDPattern.MatchString(releaseID) || filepath.Clean(target) != filepath.Join("releases", releaseID) {
		return "", errors.New("invalid current release link")
	}
	info, err := os.Stat(filepath.Join(root, "game", "releases", releaseID))
	if err != nil || !info.IsDir() {
		return "", errors.New("current release is unavailable")
	}
	return releaseID, nil
}

func (s Service) Prepare(ctx context.Context, instance domain.Instance) error {
	if instance.SelectedPackageID == "" && instance.PackageSourceID == "" && instance.PackageSourceRepository == "" {
		return errors.New("instance package is required")
	}
	var item content.PackageVersion
	var err error
	if instance.PackageSourceID != "" {
		if s.Sources == nil {
			return errors.New("source package lookup unavailable")
		}
		source, sourceErr := s.Sources.GitHubSource(ctx, instance.PackageSourceID)
		if sourceErr != nil {
			return sourceErr
		}
		resolver, ok := s.Packages.(interface {
			LatestSourceVersion(string) (content.PackageVersion, error)
		})
		if !ok {
			return errors.New("source package lookup unavailable")
		}
		item, err = resolver.LatestSourceVersion(source.Repository)
	} else if instance.PackageSourceRepository != "" {
		resolver, ok := s.Packages.(interface {
			LatestSourceVersion(string) (content.PackageVersion, error)
		})
		if !ok {
			return errors.New("source package lookup unavailable")
		}
		item, err = resolver.LatestSourceVersion(instance.PackageSourceRepository)
	} else {
		item, err = s.Packages.Get(instance.SelectedPackageID)
	}
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
	if s.Accelerator != nil {
		if err := s.Accelerator.Ensure(ctx, instance); err != nil {
			return fmt.Errorf("ensure Accelerator after package deployment: %w", err)
		}
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

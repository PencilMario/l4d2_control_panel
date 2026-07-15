package provisioning

import (
	"context"
	"errors"

	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/updates"
)

type Installer interface {
	InstallGame(context.Context, string, domain.Instance) error
}

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

type Service struct {
	Root      string
	Installer Installer
	Packages  PackageSource
	Deployer  Deployer
	Instances InstanceRepository
}

func (s Service) Prepare(ctx context.Context, instance domain.Instance) error {
	if instance.SelectedPackageID == "" {
		return errors.New("instance package is required")
	}
	item, err := s.Packages.Get(instance.SelectedPackageID)
	if err != nil {
		return err
	}
	if err := s.Installer.InstallGame(ctx, s.Root, instance); err != nil {
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

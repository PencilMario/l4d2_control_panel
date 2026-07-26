package updates

import (
	"context"
	"errors"

	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

type UpperResetter interface {
	ResetUpper(context.Context, string, string) error
}

type FullPackageApplier interface {
	Apply(context.Context, string, string, string, Mode) error
}

type SharedGameRebuilder struct {
	Overlay  UpperResetter
	Packages PackageSource
	Deployer FullPackageApplier
	Private  PrivateApplier
}

func (r SharedGameRebuilder) Unmount(ctx context.Context, instance domain.Instance, releaseID string) error {
	if releaseID == "" {
		return errors.New("shared game release is required")
	}
	unmount, ok := r.Overlay.(interface {
		Unmount(context.Context, string, string) error
	})
	if !ok {
		return errors.New("shared overlay does not support unmount")
	}
	return unmount.Unmount(ctx, instance.ID, releaseID)
}

func (r SharedGameRebuilder) Switch(ctx context.Context, instance domain.Instance, _, releaseID string) error {
	if releaseID == "" {
		return errors.New("shared game release is required")
	}
	if instance.SelectedPackageID == "" && instance.PackageSourceRepository == "" {
		return errors.New("instance package is required")
	}
	var item content.PackageVersion
	var err error
	if instance.PackageSourceRepository != "" {
		resolver, ok := r.Packages.(interface {
			LatestSourceVersion(string) (content.PackageVersion, error)
		})
		if !ok {
			return errors.New("source package lookup unavailable")
		}
		item, err = resolver.LatestSourceVersion(instance.PackageSourceRepository)
	} else {
		item, err = r.Packages.Get(instance.SelectedPackageID)
	}
	if err != nil {
		return err
	}
	if err := r.Overlay.ResetUpper(ctx, instance.ID, releaseID); err != nil {
		return err
	}
	if err := r.Deployer.Apply(ctx, instance.ID, item.ArchivePath, item.Version, Full); err != nil {
		return err
	}
	return r.Private.Apply(ctx, instance.ID)
}

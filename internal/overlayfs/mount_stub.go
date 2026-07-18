//go:build !linux

package overlayfs

import (
	"context"
	"errors"
)

type SystemMounter struct{}

func (SystemMounter) Preflight(context.Context) error { return errors.New("overlayfs requires linux") }
func (SystemMounter) Ensure(context.Context, Mount) error {
	return errors.New("overlayfs requires linux")
}
func (SystemMounter) Inspect(context.Context, Mount) (MountStatus, error) {
	return MountStatus{}, errors.New("overlayfs requires linux")
}
func (SystemMounter) ResetManagedPaths(context.Context, Mount, []string) error {
	return errors.New("overlayfs requires linux")
}
func (SystemMounter) ResetUpper(context.Context, Mount) error {
	return errors.New("overlayfs requires linux")
}
func (SystemMounter) Unmount(context.Context, Mount) error {
	return errors.New("overlayfs requires linux")
}

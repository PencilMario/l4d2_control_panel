//go:build linux

package gamelogs

import (
	"os"
	"syscall"
)

func openAppendNoFollow(path string, mode os.FileMode) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_APPEND|syscall.O_CREAT|syscall.O_WRONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, uint32(mode.Perm()))
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

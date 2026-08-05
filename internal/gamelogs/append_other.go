//go:build !linux

package gamelogs

import "os"

func openAppendNoFollow(path string, mode os.FileMode) (*os.File, error) {
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return nil, os.ErrInvalid
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, mode)
}

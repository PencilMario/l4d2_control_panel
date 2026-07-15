//go:build !windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

func pathOwnership(path string) (int, int, bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, 0, false, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false, fmt.Errorf("unsupported ownership metadata for %s", path)
	}
	return int(stat.Uid), int(stat.Gid), true, nil
}

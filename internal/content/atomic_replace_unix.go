//go:build !windows

package content

import "os"

func atomicReplaceFile(source, target string) error {
	return os.Rename(source, target)
}

func readAtomicFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

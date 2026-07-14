package safepath

import (
	"errors"
	"path/filepath"
	"strings"
)

func Join(root, name string) (string, error) {
	if name == "" || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") || filepath.IsAbs(name) || filepath.VolumeName(name) != "" {
		return "", errors.New("absolute paths are forbidden")
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes data root")
	}
	target := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes data root")
	}
	return target, nil
}

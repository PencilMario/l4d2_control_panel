package safepath

import (
	"errors"
	"path"
	"path/filepath"
	"strings"
)

func Join(root, name string) (string, error) {
	portable := strings.ReplaceAll(name, `\`, "/")
	if portable == "" || strings.HasPrefix(portable, "/") || hasDrivePrefix(portable) {
		return "", errors.New("absolute paths are forbidden")
	}
	clean := path.Clean(portable)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("path escapes data root")
	}
	target := filepath.Join(root, filepath.FromSlash(clean))
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes data root")
	}
	return target, nil
}

func hasDrivePrefix(name string) bool {
	if len(name) < 2 || name[1] != ':' {
		return false
	}
	return name[0] >= 'A' && name[0] <= 'Z' || name[0] >= 'a' && name[0] <= 'z'
}

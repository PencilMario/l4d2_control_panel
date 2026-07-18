package httpapi

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func readSharedGameVersion(currentPath string) (string, error) {
	file, err := os.Open(filepath.Join(currentPath, "left4dead2", "steam.inf"))
	if err != nil {
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, found := strings.Cut(scanner.Text(), "=")
		if found && strings.EqualFold(strings.TrimSpace(key), "PatchVersion") {
			version := strings.TrimSpace(value)
			if version == "" {
				return "", errors.New("PatchVersion is empty")
			}
			return version, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("PatchVersion not found")
}

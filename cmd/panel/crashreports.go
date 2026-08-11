package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/not0721here/l4d2-control-panel/internal/crashreports"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

type crashReportInstanceRepository interface {
	Instances(context.Context) ([]domain.Instance, error)
}

func newCrashReportInstanceAuthorizer(dataRoot string, repository crashReportInstanceRepository) crashreports.InstanceAuthorizer {
	resolve := newCrashReportInstanceResolver(dataRoot, repository)
	return func(ctx context.Context, serverID, gameDirectory string) error {
		_, err := resolve(ctx, serverID, gameDirectory)
		return err
	}
}

func newCrashReportInstanceResolver(dataRoot string, repository crashReportInstanceRepository) crashreports.InstanceResolver {
	return func(ctx context.Context, serverID, gameDirectory string) (string, error) {
		if gameDirectory != "" && gameDirectory != "left4dead2" {
			return "", crashreports.ErrInstanceNotAllowed
		}
		serverID = strings.TrimSpace(serverID)
		if serverID == "" {
			return "", crashreports.ErrInstanceNotAllowed
		}
		instances, err := repository.Instances(ctx)
		if err != nil {
			return "", err
		}
		for _, instance := range instances {
			if instance.ID == "" || filepath.Base(instance.ID) != instance.ID || instance.ID == "." || instance.ID == ".." {
				continue
			}
			if filepath.Clean(instance.ID) != instance.ID || filepath.IsAbs(instance.ID) {
				continue
			}
			if managedID, ok := readManagedServerID(dataRoot, instance.ID); ok && managedID == serverID {
				return instance.ID, nil
			}
		}
		return "", crashreports.ErrInstanceNotAllowed
	}
}

func readManagedServerID(dataRoot, instanceID string) (string, bool) {
	instanceRoot := filepath.Join(dataRoot, "instances", instanceID)
	instanceInfo, err := os.Lstat(instanceRoot)
	if err != nil || instanceInfo.Mode()&os.ModeSymlink != 0 || !instanceInfo.IsDir() {
		return "", false
	}
	gameRoot, ok := managedGameRoot(instanceRoot)
	if !ok {
		return "", false
	}
	path := gameRoot
	for _, component := range []string{"left4dead2", "addons", "sourcemod", "data", "dumps"} {
		path = filepath.Join(path, component)
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", false
		}
	}
	path = filepath.Join(path, "server-id.txt")
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(raw)), true
}

func managedGameRoot(instanceRoot string) (string, bool) {
	gamePath := filepath.Join(instanceRoot, "game")
	info, err := os.Lstat(gamePath)
	if err != nil {
		return "", false
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return gamePath, info.IsDir()
	}

	overlayRoot := filepath.Join(instanceRoot, "overlay")
	mergedRoot := filepath.Join(overlayRoot, "merged")
	for _, path := range []string{overlayRoot, mergedRoot} {
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", false
		}
	}
	resolved, err := filepath.EvalSymlinks(gamePath)
	if err != nil {
		return "", false
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", false
	}
	expected, err := filepath.EvalSymlinks(mergedRoot)
	if err != nil {
		return "", false
	}
	expected, err = filepath.Abs(expected)
	if err != nil || filepath.Clean(resolved) != filepath.Clean(expected) {
		return "", false
	}
	return resolved, true
}

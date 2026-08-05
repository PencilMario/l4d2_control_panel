package databaseconfig

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
)

type Repository interface {
	DatabaseConfig(context.Context) (Config, error)
	Instances(context.Context) ([]domain.Instance, error)
}

type Result struct {
	Synced, Deferred int
	Failed           []string
}

type Reporter interface {
	Progress(string, int, string)
	Log(string, joblogs.Level, string)
}

type Synchronizer struct {
	Root       string
	Repository Repository
}

func (s Synchronizer) SyncAll(ctx context.Context, reporter Reporter) (Result, error) {
	config, err := s.Repository.DatabaseConfig(ctx)
	if err != nil {
		return Result{}, err
	}
	raw, err := Render(config)
	if err != nil {
		return Result{}, err
	}
	instances, err := s.Repository.Instances(ctx)
	if err != nil {
		return Result{}, err
	}
	result := Result{}
	for index, instance := range instances {
		if reporter != nil {
			reporter.Progress("sync", 20+(index+1)*75/max(1, len(instances)), fmt.Sprintf("正在同步 %s", instance.Name))
		}
		deferred, syncErr := s.write(instance.ID, raw)
		if deferred {
			result.Deferred++
			continue
		}
		if syncErr != nil {
			result.Failed = append(result.Failed, instance.Name)
			if reporter != nil {
				reporter.Log("database", joblogs.Error, fmt.Sprintf("实例 %s 同步失败", instance.Name))
			}
			continue
		}
		result.Synced++
		if reporter != nil {
			reporter.Log("database", joblogs.Info, fmt.Sprintf("实例 %s 已同步", instance.Name))
		}
	}
	if len(result.Failed) > 0 {
		return result, fmt.Errorf("database sync failed for %s", strings.Join(result.Failed, ", "))
	}
	return result, nil
}

func (s Synchronizer) SyncInstance(ctx context.Context, instanceID string) error {
	config, err := s.Repository.DatabaseConfig(ctx)
	if err != nil {
		return err
	}
	raw, err := Render(config)
	if err != nil {
		return err
	}
	_, err = s.write(instanceID, raw)
	return err
}

func (s Synchronizer) write(instanceID string, raw []byte) (bool, error) {
	if instanceID == "" || filepath.Base(instanceID) != instanceID || strings.ContainsAny(instanceID, `/\`) {
		return false, errors.New("invalid instance id")
	}
	game := filepath.Join(s.Root, "instances", instanceID, "game", "left4dead2")
	if info, err := os.Stat(game); err != nil || !info.IsDir() {
		if os.IsNotExist(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return true, nil
	}
	dir := filepath.Join(game, "addons", "sourcemod", "configs")
	if err := os.MkdirAll(dir, 0750); err != nil {
		return false, err
	}
	temp, err := os.CreateTemp(dir, ".databases-*.tmp")
	if err != nil {
		return false, err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err = temp.Write(raw); err == nil {
		err = temp.Chmod(0640)
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return false, err
	}
	target := filepath.Join(dir, "databases.cfg")
	_ = os.Remove(target)
	return false, os.Rename(tempName, target)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

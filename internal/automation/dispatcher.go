package automation

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
	"github.com/not0721here/l4d2-control-panel/internal/maintenance"
	"github.com/not0721here/l4d2-control-panel/internal/players"
	"github.com/not0721here/l4d2-control-panel/internal/releases"
	"github.com/not0721here/l4d2-control-panel/internal/updates"
	"time"
)

type Dispatcher struct {
	Jobs           *jobs.Manager
	Players        *players.Service
	Packages       *content.PackageManager
	PackagesUpdate *updates.Coordinator
	GameUpdate     *updates.GameCoordinator
	Releases       releases.Client
	Maintenance    *maintenance.Manager
	Secrets        interface {
		Get(context.Context, string) (string, bool, error)
	}
}

func (d Dispatcher) Dispatch(ctx context.Context, task domain.ScheduledTask) error {
	if d.Jobs == nil {
		return errors.New("job manager unavailable")
	}
	_, err := d.Jobs.Start(context.WithoutCancel(ctx), task.InstanceID, "scheduled_"+task.Type, func(run context.Context, reporter jobs.Reporter) error {
		if task.OnlinePolicy != "force" && task.InstanceID != "" && d.Players != nil {
			for {
				snapshot, err := d.Players.Online(run, task.InstanceID)
				if err == nil && len(snapshot.Players) == 0 {
					break
				}
				if task.OnlinePolicy == "skip" {
					return errors.New("scheduled task skipped because players are online")
				}
				select {
				case <-run.Done():
					return run.Err()
				case <-time.After(time.Minute):
				}
			}
		}
		var input struct {
			PackageID     string `json:"package_id"`
			Repository    string `json:"repository"`
			AssetPattern  string `json:"asset_pattern"`
			RetentionDays int    `json:"retention_days"`
		}
		if task.Payload != "" {
			if err := json.Unmarshal([]byte(task.Payload), &input); err != nil {
				return err
			}
		}
		switch task.Type {
		case "game_update":
			return d.GameUpdate.Update(run, task.InstanceID)
		case "package_hot", "package_full":
			item, err := d.Packages.Get(input.PackageID)
			if err != nil {
				return err
			}
			mode := updates.Hot
			if task.Type == "package_full" {
				mode = updates.Full
			}
			return d.PackagesUpdate.ApplyPackage(run, task.InstanceID, item, mode)
		case "release_check":
			token := ""
			if d.Secrets != nil {
				token, _, _ = d.Secrets.Get(run, "github_token")
			}
			_, err := d.Releases.FetchLatest(run, input.Repository, input.AssetPattern, token, d.Packages)
			return err
		case "backup":
			_, err := d.Maintenance.Backup(run, task.InstanceID)
			return err
		case "cleanup":
			days := input.RetentionDays
			if days < 1 {
				days = 30
			}
			_, err := d.Maintenance.Cleanup(run, time.Duration(days)*24*time.Hour)
			return err
		default:
			return errors.New("unsupported scheduled task type")
		}
	})
	return err
}

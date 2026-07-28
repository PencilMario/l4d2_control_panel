package automation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
	"github.com/not0721here/l4d2-control-panel/internal/maintenance"
	"github.com/not0721here/l4d2-control-panel/internal/players"
	"github.com/not0721here/l4d2-control-panel/internal/releases"
	"github.com/not0721here/l4d2-control-panel/internal/updates"
)

type Dispatcher struct {
	Jobs           *jobs.Manager
	Players        *players.Service
	Packages       *content.PackageManager
	PackagesUpdate interface {
		ApplyPackage(context.Context, string, content.PackageVersion, updates.Mode) error
	}
	GameUpdate       *updates.GameCoordinator
	SharedGameUpdate interface {
		Update(context.Context, string) error
	}
	Releases       releases.Client
	ReleaseFetcher interface {
		FetchLatest(context.Context, string, string, string, *content.PackageManager) (releases.FetchResult, error)
	}
	Sources interface {
		GitHubSource(context.Context, string) (domain.GitHubSource, error)
	}
	Instances interface {
		Instance(context.Context, string) (domain.Instance, error)
	}
	Maintenance *maintenance.Manager
	Gate        *maintenance.Gate
	Secrets     interface {
		Get(context.Context, string) (string, bool, error)
	}
}

func (d Dispatcher) Dispatch(ctx context.Context, task domain.ScheduledTask) error {
	if d.Jobs == nil {
		return errors.New("job manager unavailable")
	}
	_, err := d.Jobs.Start(context.WithoutCancel(ctx), task.InstanceID, "scheduled_"+task.Type, func(run context.Context, reporter jobs.Reporter) error {
		return d.run(run, task)
	})
	return err
}

func (d Dispatcher) run(ctx context.Context, task domain.ScheduledTask) error {
	if task.Type == "game_update" {
		if d.SharedGameUpdate == nil {
			return errors.New("shared game update unavailable")
		}
		return d.SharedGameUpdate.Update(ctx, task.OnlinePolicy)
	}
	if d.Gate != nil {
		var release func()
		var err error
		ctx, release, err = d.Gate.SharedContext(ctx)
		if err != nil {
			return err
		}
		defer release()
	}
	var input struct {
		PackageID     string `json:"package_id"`
		Repository    string `json:"repository"`
		AssetPattern  string `json:"asset_pattern"`
		RetentionDays int    `json:"retention_days"`
		SourceID      string `json:"source_id"`
	}
	if task.Payload != "" {
		if err := json.Unmarshal([]byte(task.Payload), &input); err != nil {
			return err
		}
	}
	if task.Type == "release_check" && input.SourceID != "" {
		if d.Sources == nil {
			return errors.New("GitHub source not found")
		}
		source, err := d.Sources.GitHubSource(ctx, input.SourceID)
		if err != nil {
			return errors.New("GitHub source not found")
		}
		input.Repository, input.AssetPattern = source.Repository, source.AssetPattern
	}
	if err := d.waitForPlayers(ctx, task); err != nil {
		return err
	}
	switch task.Type {
	case "package_full", "release_full":
		if d.GameUpdate == nil {
			return errors.New("instance package update unavailable")
		}
		return d.GameUpdate.Reinstall(ctx, task.InstanceID, updates.ReinstallOptions{Package: true})
	case "release_check":
		token := ""
		if d.Secrets != nil {
			token, _, _ = d.Secrets.Get(ctx, "github_token")
		}
		_, err := d.Releases.FetchLatest(ctx, input.Repository, input.AssetPattern, token, d.Packages)
		return err
	case "backup":
		_, err := d.Maintenance.Backup(ctx, task.InstanceID)
		return err
	case "cleanup":
		days := input.RetentionDays
		if days < 1 {
			days = 30
		}
		_, err := d.Maintenance.Cleanup(ctx, time.Duration(days)*24*time.Hour)
		return err
	default:
		return errors.New("unsupported scheduled task type")
	}
}

func (d Dispatcher) waitForPlayers(ctx context.Context, task domain.ScheduledTask) error {
	if task.OnlinePolicy == "force" || task.InstanceID == "" || d.Players == nil {
		return nil
	}
	for {
		snapshot, err := d.Players.Online(ctx, task.InstanceID)
		if err == nil && len(snapshot.Players) == 0 {
			return nil
		}
		if task.OnlinePolicy == "skip" {
			return errors.New("scheduled task skipped because players are online")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Minute):
		}
	}
}

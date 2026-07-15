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
	PackagesUpdate interface {
		ApplyPackage(context.Context, string, content.PackageVersion, updates.Mode) error
	}
	GameUpdate     *updates.GameCoordinator
	Releases       releases.Client
	ReleaseFetcher interface {
		FetchLatest(context.Context, string, string, string, *content.PackageManager) (releases.FetchResult, error)
	}
	Sources interface {
		GitHubSource(context.Context, string) (domain.GitHubSource, error)
	}
	Maintenance *maintenance.Manager
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
	if input.SourceID != "" {
		if d.Sources == nil {
			return errors.New("GitHub source not found")
		}
		source, err := d.Sources.GitHubSource(ctx, input.SourceID)
		if err != nil {
			return errors.New("GitHub source not found")
		}
		input.Repository, input.AssetPattern = source.Repository, source.AssetPattern
	}
	if task.Type == "release_hot" || task.Type == "release_full" {
		fetcher := d.ReleaseFetcher
		if fetcher == nil {
			fetcher = d.Releases
		}
		token := ""
		if d.Secrets != nil {
			token, _, _ = d.Secrets.Get(ctx, "github_token")
		}
		result, err := fetcher.FetchLatest(ctx, input.Repository, input.AssetPattern, token, d.Packages)
		if err != nil || !result.Updated {
			return err
		}
		if err := d.waitForPlayers(ctx, task); err != nil {
			return err
		}
		mode := updates.Hot
		if task.Type == "release_full" {
			mode = updates.Full
		}
		return d.PackagesUpdate.ApplyPackage(ctx, task.InstanceID, result.Package, mode)
	}
	if err := d.waitForPlayers(ctx, task); err != nil {
		return err
	}
	switch task.Type {
	case "game_update":
		return d.GameUpdate.Update(ctx, task.InstanceID)
	case "package_hot", "package_full":
		item, err := d.Packages.Get(input.PackageID)
		if err != nil {
			return err
		}
		mode := updates.Hot
		if task.Type == "package_full" {
			mode = updates.Full
		}
		return d.PackagesUpdate.ApplyPackage(ctx, task.InstanceID, item, mode)
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

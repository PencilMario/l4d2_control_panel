package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/crashreports"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
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
	Maintenance interface {
		Backup(context.Context, string) (string, error)
		Cleanup(context.Context, time.Duration) (int, error)
	}
	GameLogs interface {
		Cleanup(context.Context, int) error
	}
	CrashReports interface {
		Cleanup(context.Context, int) (crashreports.CleanupResult, error)
	}
	Gate    *maintenance.Gate
	Secrets interface {
		Get(context.Context, string) (string, bool, error)
	}
}

func (d Dispatcher) Dispatch(ctx context.Context, task domain.ScheduledTask) error {
	if d.Jobs == nil {
		return errors.New("job manager unavailable")
	}
	_, err := d.Jobs.StartWithOptions(context.WithoutCancel(ctx), task.InstanceID, "scheduled_"+task.Type, jobs.StartOptions{TimeoutMinutes: task.TimeoutMinutes}, func(run context.Context, reporter jobs.Reporter) error {
		return d.run(run, task)
	})
	return err
}

func (d Dispatcher) run(ctx context.Context, task domain.ScheduledTask) error {
	jobs.Logf(ctx, "schedule", joblogs.Info, "scheduled task started schedule=%s type=%s target=%s online_policy=%s", task.ID, task.Type, task.InstanceID, task.OnlinePolicy)
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
	jobs.Logf(ctx, "schedule", joblogs.Info, "scheduled parameters package_id=%s source_id=%s repository=%s asset_pattern=%q retention_days=%d", input.PackageID, input.SourceID, input.Repository, input.AssetPattern, input.RetentionDays)
	if task.Type == "release_check" && input.SourceID != "" {
		if d.Sources == nil {
			return errors.New("GitHub source not found")
		}
		source, err := d.Sources.GitHubSource(ctx, input.SourceID)
		if err != nil {
			return errors.New("GitHub source not found")
		}
		jobs.Logf(ctx, "schedule", joblogs.Info, "resolved GitHub source source_id=%s source_name=%q repository=%s asset_pattern=%q", source.ID, source.Name, source.Repository, source.AssetPattern)
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
		if d.Maintenance == nil {
			return errors.New("maintenance unavailable")
		}
		_, err := d.Maintenance.Backup(ctx, task.InstanceID)
		return err
	case "cleanup":
		days := input.RetentionDays
		if days < 1 {
			days = 30
		}
		var cleanupErrs []error
		maintenanceResult := -1
		if d.Maintenance == nil {
			cleanupErrs = append(cleanupErrs, errors.New("maintenance unavailable"))
		} else {
			var err error
			maintenanceResult, err = d.Maintenance.Cleanup(ctx, time.Duration(days)*24*time.Hour)
			if err != nil {
				cleanupErrs = append(cleanupErrs, fmt.Errorf("maintenance cleanup: %w", err))
			}
		}
		if d.GameLogs == nil {
			cleanupErrs = append(cleanupErrs, errors.New("game log cleanup unavailable"))
		} else if err := d.GameLogs.Cleanup(ctx, days); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("game log cleanup: %w", err))
		}
		crashResult := crashreports.CleanupResult{}
		if d.CrashReports == nil {
			cleanupErrs = append(cleanupErrs, errors.New("crash report cleanup unavailable"))
		} else {
			var err error
			crashResult, err = d.CrashReports.Cleanup(ctx, days)
			if err != nil {
				cleanupErrs = append(cleanupErrs, fmt.Errorf("crash report cleanup: %w", err))
			}
		}
		jobs.Logf(ctx, "schedule", joblogs.Info, "cleanup summary retention_days=%d maintenance_removed=%d crash_reports_removed=%d crash_pending_removed=%d crash_artifacts_removed=%d crash_bytes_released=%d", days, maintenanceResult, crashResult.ReportsRemoved, crashResult.PendingRemoved, crashResult.ArtifactsRemoved, crashResult.BytesReleased)
		return errors.Join(cleanupErrs...)
	default:
		return errors.New("unsupported scheduled task type")
	}
}

func (d Dispatcher) waitForPlayers(ctx context.Context, task domain.ScheduledTask) error {
	if task.OnlinePolicy == "force" || task.InstanceID == "" || d.Players == nil {
		jobs.Logf(ctx, "schedule", joblogs.Info, "player policy allows execution target=%s policy=%s", task.InstanceID, task.OnlinePolicy)
		return nil
	}
	if d.Instances != nil {
		instance, err := d.Instances.Instance(ctx, task.InstanceID)
		if err == nil && instance.ActualState == domain.StateStopped {
			jobs.Logf(ctx, "schedule", joblogs.Info, "player check bypassed target=%s state=%s", task.InstanceID, instance.ActualState)
			return nil
		}
	}
	for {
		snapshot, err := d.Players.Online(ctx, task.InstanceID)
		if err != nil && task.OnlinePolicy == "wait" {
			jobs.Logf(ctx, "schedule", joblogs.Warn, "player query failed; forcing scheduled execution target=%s policy=%s error=%q", task.InstanceID, task.OnlinePolicy, err.Error())
			return nil
		}
		if err == nil && len(snapshot.Players) == 0 {
			jobs.Logf(ctx, "schedule", joblogs.Info, "player check passed target=%s players=0", task.InstanceID)
			return nil
		}
		if task.OnlinePolicy == "skip" {
			jobs.Logf(ctx, "schedule", joblogs.Warn, "scheduled task skipped target=%s players=%d query_error=%t", task.InstanceID, len(snapshot.Players), err != nil)
			return errors.New("scheduled task skipped because players are online")
		}
		jobs.Logf(ctx, "schedule", joblogs.Info, "waiting for players target=%s players=%d query_error=%t retry_in=1m", task.InstanceID, len(snapshot.Players), err != nil)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Minute):
		}
	}
}

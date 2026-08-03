package vpkrestart

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
	"github.com/not0721here/l4d2-control-panel/internal/players"
)

type Repository interface {
	Instances(context.Context) ([]domain.Instance, error)
	Instance(context.Context, string) (domain.Instance, error)
	UpsertVPKRestart(context.Context, domain.VPKRestart) error
	PendingVPKRestarts(context.Context) ([]domain.VPKRestart, error)
	ClaimVPKRestart(context.Context, string) (bool, error)
	UpdateVPKRestart(context.Context, string, string, int) error
}

type PlayerProvider interface {
	Online(context.Context, string) (players.Snapshot, error)
}
type Lifecycle interface {
	Restart(context.Context, string) error
}
type JobStarter interface {
	StartWithOptions(context.Context, string, string, jobs.StartOptions, func(context.Context, jobs.Reporter) error) (jobs.Job, error)
}

type Coordinator struct {
	repo        Repository
	players     PlayerProvider
	lifecycle   Lifecycle
	jobs        JobStarter
	interval    time.Duration
	waitTimeout time.Duration
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

func New(repo Repository, playerProvider PlayerProvider, lifecycle Lifecycle, jobManager JobStarter) *Coordinator {
	return &Coordinator{repo: repo, players: playerProvider, lifecycle: lifecycle, jobs: jobManager, interval: 30 * time.Second, waitTimeout: 24 * time.Hour}
}

func active(v domain.Instance) bool {
	return v.ContainerID != "" && (v.DesiredState == domain.StateRunning || v.ActualState == domain.StateRunning || v.ActualState == domain.StateStarting || v.ActualState == domain.StateInstalling)
}

func (c *Coordinator) Register(ctx context.Context, publicationID string) (int, error) {
	jobs.Logf(ctx, "vpk-restart", joblogs.Info, "registering deferred restart publication=%s", publicationID)
	instances, err := c.repo.Instances(ctx)
	if err != nil {
		return 0, err
	}
	pending, err := c.repo.PendingVPKRestarts(ctx)
	if err != nil {
		return 0, err
	}
	existing := map[string]domain.VPKRestart{}
	for _, item := range pending {
		existing[item.InstanceID] = item
	}
	count := 0
	for _, instance := range instances {
		if !active(instance) {
			continue
		}
		item := domain.VPKRestart{InstanceID: instance.ID, ContainerID: instance.ContainerID, PublicationID: publicationID, Status: "waiting"}
		if current, ok := existing[instance.ID]; ok {
			item = current
			item.PublicationID = publicationID
		}
		if err := c.repo.UpsertVPKRestart(ctx, item); err != nil {
			return count, err
		}
		if item.JobID == "" {
			job, startErr := c.startJob(context.WithoutCancel(ctx), item)
			if startErr != nil {
				return count, startErr
			}
			item.JobID = job.ID
			if err := c.repo.UpsertVPKRestart(ctx, item); err != nil {
				return count, err
			}
		}
		count++
		jobs.Logf(ctx, "vpk-restart", joblogs.Info, "deferred restart registered instance=%s container=%s publication=%s", instance.ID, instance.ContainerID, publicationID)
	}
	return count, nil
}

func (c *Coordinator) Check(ctx context.Context) error {
	items, err := c.repo.PendingVPKRestarts(ctx)
	if err != nil {
		return err
	}
	var result error
	for _, item := range items {
		if item.JobID != "" {
			continue
		}
		if item.Status == "queued" {
			if err := c.repo.UpdateVPKRestart(ctx, item.InstanceID, "retry", item.Failures); err != nil {
				result = errors.Join(result, err)
				continue
			}
			item.Status = "retry"
		}
		if err := c.checkOne(ctx, item); err != nil {
			result = errors.Join(result, err)
		}
	}
	return result
}

func (c *Coordinator) startJob(ctx context.Context, item domain.VPKRestart) (jobs.Job, error) {
	return c.jobs.StartWithOptions(ctx, item.InstanceID, "shared_vpk_restart", jobs.StartOptions{
		TimeoutMinutes: domain.DefaultJobTimeoutMinutes,
		Preflight: func(run context.Context, reporter jobs.Reporter) error {
			return c.waitForPlayers(run, reporter, item)
		},
	}, func(run context.Context, reporter jobs.Reporter) error {
		current, err := c.repo.Instance(run, item.InstanceID)
		if err != nil || !active(current) {
			_ = c.repo.UpdateVPKRestart(run, item.InstanceID, "cancelled", item.Failures)
			return err
		}
		if current.ContainerID != item.ContainerID {
			return c.repo.UpdateVPKRestart(run, item.InstanceID, "completed", item.Failures)
		}
		if reporter != nil {
			reporter.Progress("restart", 80, "Restarting instance to load shared VPK")
		}
		if err := c.lifecycle.Restart(run, item.InstanceID); err != nil {
			_ = c.repo.UpdateVPKRestart(run, item.InstanceID, "failed", item.Failures)
			return err
		}
		return c.repo.UpdateVPKRestart(run, item.InstanceID, "completed", item.Failures)
	})
}

func (c *Coordinator) waitForPlayers(ctx context.Context, reporter jobs.Reporter, item domain.VPKRestart) error {
	timeout := c.waitTimeout
	if timeout <= 0 {
		timeout = 24 * time.Hour
	}
	deadline := item.CreatedAt.Add(timeout)
	if item.CreatedAt.IsZero() {
		deadline = time.Now().Add(timeout)
	}
	for {
		instance, err := c.repo.Instance(ctx, item.InstanceID)
		if err != nil || !active(instance) || instance.ContainerID != item.ContainerID {
			return nil
		}
		snapshot, queryErr := c.players.Online(ctx, item.InstanceID)
		if queryErr == nil && len(snapshot.Players) == 0 {
			return nil
		}
		message := "Waiting for players to leave"
		if queryErr != nil {
			message = "Player query failed; waiting until timeout"
		}
		if reporter != nil {
			reporter.Progress("waiting_players", 5, message)
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil
		}
		wait := c.interval
		if remaining < wait {
			wait = remaining
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *Coordinator) checkOne(ctx context.Context, item domain.VPKRestart) error {
	instance, err := c.repo.Instance(ctx, item.InstanceID)
	if err != nil {
		return c.repo.UpdateVPKRestart(ctx, item.InstanceID, "cancelled", item.Failures)
	}
	if !active(instance) {
		return c.repo.UpdateVPKRestart(ctx, item.InstanceID, "cancelled", item.Failures)
	}
	if instance.ContainerID != item.ContainerID {
		return c.repo.UpdateVPKRestart(ctx, item.InstanceID, "completed", item.Failures)
	}
	snapshot, queryErr := c.players.Online(ctx, item.InstanceID)
	if queryErr == nil && len(snapshot.Players) > 0 {
		return c.repo.UpdateVPKRestart(ctx, item.InstanceID, "waiting", 0)
	}
	failures := item.Failures
	readinessReason := "no-online-players"
	if queryErr != nil {
		failures++
		readinessReason = "player-query-failure-threshold"
		if failures < 3 {
			return c.repo.UpdateVPKRestart(ctx, item.InstanceID, "waiting", failures)
		}
	}
	claimed, err := c.repo.ClaimVPKRestart(ctx, item.InstanceID)
	if err != nil || !claimed {
		return err
	}
	_, err = c.jobs.StartWithOptions(context.WithoutCancel(ctx), item.InstanceID, "shared_vpk_restart", jobs.StartOptions{TimeoutMinutes: domain.DefaultJobTimeoutMinutes}, func(run context.Context, reporter jobs.Reporter) error {
		jobs.Logf(run, "vpk-restart", joblogs.Info, "restart readiness confirmed instance=%s reason=%s query_failures=%d publication=%s", item.InstanceID, readinessReason, failures, item.PublicationID)
		jobs.Logf(run, "vpk-restart", joblogs.Info, "deferred restart task started instance=%s publication=%s expected_container=%s", item.InstanceID, item.PublicationID, item.ContainerID)
		current, loadErr := c.repo.Instance(run, item.InstanceID)
		if loadErr != nil || !active(current) {
			_ = c.repo.UpdateVPKRestart(run, item.InstanceID, "cancelled", failures)
			return loadErr
		}
		if current.ContainerID != item.ContainerID {
			return c.repo.UpdateVPKRestart(run, item.InstanceID, "completed", failures)
		}
		if reporter != nil {
			reporter.Progress("restart", 80, "Restarting instance to load shared VPK")
		}
		if restartErr := c.lifecycle.Restart(run, item.InstanceID); restartErr != nil {
			_ = c.repo.UpdateVPKRestart(run, item.InstanceID, "retry", failures)
			return restartErr
		}
		if err := c.repo.UpdateVPKRestart(run, item.InstanceID, "completed", failures); err != nil {
			return err
		}
		jobs.Logf(run, "vpk-restart", joblogs.Info, "deferred restart completed instance=%s publication=%s", item.InstanceID, item.PublicationID)
		return nil
	})
	if err != nil {
		_ = c.repo.UpdateVPKRestart(ctx, item.InstanceID, "retry", failures)
	}
	return err
}

func (c *Coordinator) Start(parent context.Context) {
	if c.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	c.cancel = cancel
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		_ = c.Check(ctx)
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = c.Check(ctx)
			}
		}
	}()
}

func (c *Coordinator) Stop() {
	if c.cancel != nil {
		c.cancel()
		c.wg.Wait()
		c.cancel = nil
	}
}

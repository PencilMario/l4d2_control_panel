package vpkrestart

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
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
	Start(context.Context, string, string, func(context.Context, jobs.Reporter) error) (jobs.Job, error)
}

type Coordinator struct {
	repo      Repository
	players   PlayerProvider
	lifecycle Lifecycle
	jobs      JobStarter
	interval  time.Duration
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

func New(repo Repository, playerProvider PlayerProvider, lifecycle Lifecycle, jobManager JobStarter) *Coordinator {
	return &Coordinator{repo: repo, players: playerProvider, lifecycle: lifecycle, jobs: jobManager, interval: 30 * time.Second}
}

func active(v domain.Instance) bool {
	return v.ContainerID != "" && (v.DesiredState == domain.StateRunning || v.ActualState == domain.StateRunning || v.ActualState == domain.StateStarting || v.ActualState == domain.StateInstalling)
}

func (c *Coordinator) Register(ctx context.Context, publicationID string) (int, error) {
	instances, err := c.repo.Instances(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, instance := range instances {
		if !active(instance) {
			continue
		}
		if err := c.repo.UpsertVPKRestart(ctx, domain.VPKRestart{InstanceID: instance.ID, ContainerID: instance.ContainerID, PublicationID: publicationID, Status: "waiting"}); err != nil {
			return count, err
		}
		count++
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
	if queryErr != nil {
		failures++
		if failures < 3 {
			return c.repo.UpdateVPKRestart(ctx, item.InstanceID, "waiting", failures)
		}
	}
	claimed, err := c.repo.ClaimVPKRestart(ctx, item.InstanceID)
	if err != nil || !claimed {
		return err
	}
	_, err = c.jobs.Start(context.WithoutCancel(ctx), item.InstanceID, "shared_vpk_restart", func(run context.Context, reporter jobs.Reporter) error {
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
		return c.repo.UpdateVPKRestart(run, item.InstanceID, "completed", failures)
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

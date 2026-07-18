package gamelogs

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
	"github.com/robfig/cron/v3"
)

const CleanupJobType = "cleanup_game_logs"

type SchedulerStore interface {
	Instances(context.Context) ([]domain.Instance, error)
	GameLogRetentionDays() (int, error)
	HasActiveJob(context.Context, string, string) (bool, error)
}

type EnqueueResult struct {
	Queued, Deduplicated, Failed int
	Errors                       []string
	JobIDs                       []string
}

type Scheduler struct {
	mu      sync.Mutex
	store   SchedulerStore
	jobs    *jobs.Manager
	cleaner *Manager
	cron    *cron.Cron
	started bool
}

func NewScheduler(store SchedulerStore, jobManager *jobs.Manager, cleaner *Manager) *Scheduler {
	return &Scheduler{store: store, jobs: jobManager, cleaner: cleaner, cron: cron.New()}
}

func (s *Scheduler) EnqueueAll(ctx context.Context) EnqueueResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := EnqueueResult{Errors: []string{}, JobIDs: []string{}}
	instances, err := s.store.Instances(ctx)
	if err != nil {
		result.Failed++
		result.Errors = append(result.Errors, err.Error())
		return result
	}
	for _, instance := range instances {
		active, err := s.store.HasActiveJob(ctx, instance.ID, CleanupJobType)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, instance.ID+": "+err.Error())
			continue
		}
		if active {
			result.Deduplicated++
			continue
		}
		id := instance.ID
		job, err := s.jobs.Start(ctx, id, CleanupJobType, func(runCtx context.Context, reporter jobs.Reporter) error {
			days, err := s.store.GameLogRetentionDays()
			if err != nil {
				return fmt.Errorf("read game log retention: %w", err)
			}
			cutoff := s.cleaner.now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
			reporter.Progress("cleanup", 10, fmt.Sprintf("retention=%d cutoff=%s", days, cutoff.Format(time.RFC3339)))
			cleanup, cleanupErr := s.cleaner.Cleanup(runCtx, id, days)
			summary := fmt.Sprintf("retention=%d cutoff=%s Scanned=%d Expired=%d Deleted=%d Skipped=%d ReleasedBytes=%d Failures=%d", days, cutoff.Format(time.RFC3339), cleanup.Scanned, cleanup.Expired, cleanup.Deleted, cleanup.Skipped, cleanup.ReleasedBytes, len(cleanup.Failures))
			reporter.Log("cleanup", joblogs.Info, summary)
			if len(cleanup.Failures) > 0 {
				reporter.Log("cleanup", joblogs.Error, "failure summary: "+strings.Join(cleanup.Failures, "; "))
			}
			return cleanupErr
		})
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, id+": "+err.Error())
			continue
		}
		result.Queued++
		result.JobIDs = append(result.JobIDs, job.ID)
	}
	return result
}

func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return nil
	}
	if _, err := s.cron.AddFunc("0 3 * * *", func() {
		result := s.EnqueueAll(context.Background())
		if result.Failed > 0 {
			log.Printf("enqueue daily game log cleanup: %s", strings.Join(result.Errors, "; "))
		}
	}); err != nil {
		return err
	}
	s.started = true
	s.cron.Start()
	return nil
}

func (s *Scheduler) Stop() { <-s.cron.Stop().Done() }

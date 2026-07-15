package scheduler

import (
	"context"
	"errors"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/robfig/cron/v3"
	"sync"
	"time"
)

type Repository interface {
	SaveScheduledTask(context.Context, domain.ScheduledTask) error
	ScheduledTasks(context.Context) ([]domain.ScheduledTask, error)
	DeleteScheduledTask(context.Context, string) error
}
type Dispatcher interface {
	Dispatch(context.Context, domain.ScheduledTask) error
}
type Service struct {
	repo       Repository
	dispatcher Dispatcher
	cron       *cron.Cron
	mu         sync.Mutex
	entries    map[string]cron.EntryID
}

var taskTypes = map[string]bool{"game_update": true, "package_hot": true, "package_full": true, "release_check": true, "release_hot": true, "release_full": true, "backup": true, "cleanup": true}

func NewService(repo Repository, dispatcher Dispatcher) *Service {
	s := &Service{repo: repo, dispatcher: dispatcher, cron: cron.New(), entries: map[string]cron.EntryID{}}
	s.cron.Start()
	tasks, _ := repo.ScheduledTasks(context.Background())
	for _, task := range tasks {
		if task.Enabled {
			_ = s.schedule(task)
		}
	}
	return s
}
func (s *Service) Save(ctx context.Context, task domain.ScheduledTask) error {
	if !taskTypes[task.Type] {
		return errors.New("unsupported scheduled task type")
	}
	if task.OnlinePolicy != "skip" && task.OnlinePolicy != "wait" && task.OnlinePolicy != "force" {
		return errors.New("online policy must be skip, wait or force")
	}
	parsed, err := Parse(task.Cron, task.Timezone)
	if err != nil {
		return err
	}
	task.NextRun = parsed.Next(time.Now())
	if err := s.repo.SaveScheduledTask(ctx, task); err != nil {
		return err
	}
	s.mu.Lock()
	if entry, ok := s.entries[task.ID]; ok {
		s.cron.Remove(entry)
		delete(s.entries, task.ID)
	}
	s.mu.Unlock()
	if task.Enabled {
		return s.schedule(task)
	}
	return nil
}
func (s *Service) schedule(task domain.ScheduledTask) error {
	parsed, err := Parse(task.Cron, task.Timezone)
	if err != nil {
		return err
	}
	entry := s.cron.Schedule(parsed, cron.FuncJob(func() { _ = s.execute(context.Background(), task.ID) }))
	s.mu.Lock()
	s.entries[task.ID] = entry
	s.mu.Unlock()
	return nil
}
func (s *Service) RunNow(ctx context.Context, id string) error { return s.execute(ctx, id) }
func (s *Service) execute(ctx context.Context, id string) error {
	tasks, err := s.repo.ScheduledTasks(ctx)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if task.ID != id {
			continue
		}
		if err := s.dispatcher.Dispatch(ctx, task); err != nil {
			return err
		}
		task.LastRun = time.Now().UTC()
		parsed, _ := Parse(task.Cron, task.Timezone)
		task.NextRun = parsed.Next(task.LastRun)
		return s.repo.SaveScheduledTask(ctx, task)
	}
	return errors.New("scheduled task not found")
}
func (s *Service) List(ctx context.Context) ([]domain.ScheduledTask, error) {
	return s.repo.ScheduledTasks(ctx)
}
func (s *Service) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	if entry, ok := s.entries[id]; ok {
		s.cron.Remove(entry)
		delete(s.entries, id)
	}
	s.mu.Unlock()
	return s.repo.DeleteScheduledTask(ctx, id)
}
func (s *Service) Stop() { ctx := s.cron.Stop(); <-ctx.Done() }

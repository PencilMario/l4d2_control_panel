package jobs

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/joblogs"
)

type Status string

const (
	Pending     Status = "pending"
	Running     Status = "running"
	Succeeded   Status = "succeeded"
	Failed      Status = "failed"
	Interrupted Status = "interrupted"
)

type Job struct {
	ID, InstanceID, Type, Stage, Message string
	Status                               Status
	Percent, TimeoutMinutes              int
	Error                                string
	CreatedAt, UpdatedAt                 time.Time
	StartedAt, FinishedAt                *time.Time
}
type StartOptions struct {
	TimeoutMinutes int
	Preflight      func(context.Context, Reporter) error
}
type Reporter interface {
	Progress(stage string, percent int, message string)
	Log(source string, level joblogs.Level, message string)
}
type reporter struct {
	m         *Manager
	id        string
	kind      string
	target    string
	startedAt time.Time
}

type reporterContextKey struct{}

func LogContext(ctx context.Context, source string, level joblogs.Level, message string) {
	if value, ok := ctx.Value(reporterContextKey{}).(Reporter); ok && value != nil {
		value.Log(source, level, message)
	}
}

func (r reporter) Progress(stage string, percent int, message string) {
	r.m.mu.Lock()
	j := r.m.jobs[r.id]
	now := time.Now().UTC()
	j.Stage, j.Percent, j.Message, j.UpdatedAt = stage, percent, message, now
	event := domain.JobEvent{JobID: j.ID, Kind: "progress", Stage: stage, Percent: percent, Message: message, CreatedAt: now}
	var persistErr error
	if r.m.repo != nil {
		persistErr = r.m.repo.SaveJobWithEvent(toRecord(j), event)
	}
	if persistErr == nil {
		r.m.jobs[r.id] = j
		r.m.events[j.ID] = append(r.m.events[j.ID], event)
	}
	r.m.mu.Unlock()
	if persistErr != nil {
		log.Printf("persist job progress %s: %v", j.ID, persistErr)
		return
	}
	r.m.appendLog(j.ID, stage, joblogs.Info, message)
}
func (r reporter) Log(source string, level joblogs.Level, message string) {
	r.m.appendLog(r.id, source, level, message)
}

func (r reporter) logStart() {
	r.m.appendLog(r.id, "task", joblogs.Info, fmt.Sprintf("task started type=%s target=%s", r.kind, r.target))
}

func (r reporter) logFinish(err error) {
	r.m.mu.RLock()
	phase := r.m.jobs[r.id].Stage
	r.m.mu.RUnlock()
	duration := FormatDuration(time.Since(r.startedAt))
	if err != nil {
		r.m.appendLog(r.id, "task", joblogs.Error, fmt.Sprintf("task failed type=%s target=%s phase=%s duration=%s error=%q", r.kind, r.target, phase, duration, err.Error()))
		return
	}
	r.m.appendLog(r.id, "task", joblogs.Info, fmt.Sprintf("task completed type=%s target=%s duration=%s", r.kind, r.target, duration))
}

type Manager struct {
	mu     sync.RWMutex
	jobs   map[string]Job
	events map[string][]domain.JobEvent
	locks  map[string]*sync.Mutex
	repo   Repository
	logs   LogSink
	wg     sync.WaitGroup
}

type LogSink interface {
	Append(context.Context, string, string, joblogs.Level, string) (joblogs.Record, error)
	Finalize(context.Context, string) error
}

type Option func(*Manager)

func WithLogSink(sink LogSink) Option {
	return func(manager *Manager) { manager.logs = sink }
}

type Repository interface {
	SaveJobWithEvent(domain.JobRecord, domain.JobEvent) error
	LoadJob(string) (domain.JobRecord, bool, error)
	JobEvents(string) ([]domain.JobEvent, error)
}

func NewManager(options ...Option) *Manager {
	m := &Manager{jobs: map[string]Job{}, events: map[string][]domain.JobEvent{}, locks: map[string]*sync.Mutex{}}
	for _, option := range options {
		option(m)
	}
	return m
}
func NewPersistentManager(repo Repository, options ...Option) *Manager {
	m := NewManager(options...)
	m.repo = repo
	if recovery, ok := repo.(interface{ RecoverJobsWithIDs() ([]string, error) }); ok {
		ids, err := recovery.RecoverJobsWithIDs()
		if err != nil {
			log.Printf("recover jobs: %v", err)
		} else {
			for _, id := range ids {
				record, found, loadErr := repo.LoadJob(id)
				if loadErr != nil {
					log.Printf("load recovered job %s: %v", id, loadErr)
					continue
				}
				if found {
					m.appendLog(id, "task", joblogs.Error, record.Error)
					m.finalizeLog(id)
				}
			}
		}
	} else if recovery, ok := repo.(interface{ RecoverJobs() error }); ok {
		if err := recovery.RecoverJobs(); err != nil {
			log.Printf("recover jobs: %v", err)
		}
	}
	if pruner, ok := repo.(interface{ PruneCompletedJobs() error }); ok {
		if err := pruner.PruneCompletedJobs(); err != nil {
			log.Printf("prune completed jobs after recovery: %v", err)
		}
	}
	return m
}
func (m *Manager) Start(ctx context.Context, instanceID, kind string, fn func(context.Context, Reporter) error) (Job, error) {
	return m.StartWithOptions(ctx, instanceID, kind, StartOptions{}, fn)
}

func (m *Manager) StartWithOptions(ctx context.Context, instanceID, kind string, options StartOptions, fn func(context.Context, Reporter) error) (Job, error) {
	now := time.Now().UTC()
	timeoutMinutes := domain.NormalizeJobTimeoutMinutes(options.TimeoutMinutes)
	if timeoutMinutes < domain.MinJobTimeoutMinutes || timeoutMinutes > domain.MaxJobTimeoutMinutes {
		return Job{}, fmt.Errorf("job timeout must be between %d and %d minutes", domain.MinJobTimeoutMinutes, domain.MaxJobTimeoutMinutes)
	}
	j := Job{ID: uuid.NewString(), InstanceID: instanceID, Type: kind, Status: Pending, TimeoutMinutes: timeoutMinutes, CreatedAt: now, UpdatedAt: now}
	event := domain.JobEvent{JobID: j.ID, Kind: "queued", Message: "Task queued", CreatedAt: now}
	if m.repo != nil {
		if err := m.repo.SaveJobWithEvent(toRecord(j), event); err != nil {
			return Job{}, err
		}
	}
	m.mu.Lock()
	m.jobs[j.ID] = j
	m.events[j.ID] = append(m.events[j.ID], event)
	lock := m.locks[instanceID]
	if lock == nil {
		lock = &sync.Mutex{}
		m.locks[instanceID] = lock
	}
	m.wg.Add(1)
	m.mu.Unlock()
	m.appendLog(j.ID, "task", joblogs.Info, event.Message)
	go func() {
		defer m.wg.Done()
		target := instanceID
		if target == "" {
			target = "global"
		}
		var activeReporter reporter
		startReporter := func() error {
			if err := m.setStatus(j.ID, Running, 0, ""); err != nil {
				return fmt.Errorf("task could not start because its running state could not be persisted: %w", err)
			}
			activeReporter = reporter{m: m, id: j.ID, kind: kind, target: target, startedAt: time.Now()}
			activeReporter.logStart()
			return nil
		}
		if options.Preflight != nil {
			if err := startReporter(); err != nil {
				_ = m.setStatus(j.ID, Failed, -1, err.Error())
				return
			}
			if err := options.Preflight(context.WithValue(ctx, reporterContextKey{}, Reporter(activeReporter)), activeReporter); err != nil {
				activeReporter.logFinish(err)
				_ = m.setStatus(j.ID, Failed, -1, err.Error())
				return
			}
		}
		lock.Lock()
		defer lock.Unlock()
		if options.Preflight == nil {
			if err := startReporter(); err != nil {
				_ = m.setStatus(j.ID, Failed, -1, err.Error())
				return
			}
		}
		operationCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMinutes)*time.Minute)
		err := fn(context.WithValue(operationCtx, reporterContextKey{}, Reporter(activeReporter)), activeReporter)
		cancel()
		activeReporter.logFinish(err)
		if err != nil {
			_ = m.setStatus(j.ID, Failed, -1, err.Error())
		} else {
			_ = m.setStatus(j.ID, Succeeded, 100, "")
		}
	}()
	return j, nil
}

func (m *Manager) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (m *Manager) setStatus(id string, status Status, percent int, message string) error {
	m.mu.Lock()
	j := m.jobs[id]
	now := time.Now().UTC()
	j.Status, j.UpdatedAt = status, now
	if percent >= 0 {
		j.Percent = percent
	}
	j.Error = message
	event := domain.JobEvent{JobID: j.ID, Kind: string(status), Stage: j.Stage, Percent: j.Percent, CreatedAt: now}
	switch status {
	case Running:
		j.StartedAt = &now
		event.Kind = "started"
		event.Message = "Task started"
		if j.Stage == "" {
			j.Stage = "running"
			event.Stage = j.Stage
		}
		j.Message = event.Message
	case Succeeded:
		j.FinishedAt = &now
		event.Message = "Task completed"
		j.Stage = "complete"
		j.Message = event.Message
	case Failed, Interrupted:
		j.FinishedAt = &now
		event.Message = message
		if j.Stage == "" {
			j.Stage = string(status)
			event.Stage = j.Stage
		}
		j.Message = message
	}
	var persistErr error
	var pruneErr error
	if m.repo != nil {
		persistErr = m.repo.SaveJobWithEvent(toRecord(j), event)
		if persistErr == nil && (status == Succeeded || status == Failed || status == Interrupted) {
			if pruner, ok := m.repo.(interface{ PruneCompletedJobs() error }); ok {
				pruneErr = pruner.PruneCompletedJobs()
			}
		}
	}
	if persistErr == nil {
		m.jobs[id] = j
		m.events[id] = append(m.events[id], event)
		if m.repo != nil && (status == Succeeded || status == Failed || status == Interrupted) {
			delete(m.jobs, id)
			delete(m.events, id)
		}
	}
	m.mu.Unlock()
	if persistErr != nil {
		log.Printf("persist job status %s: %v", id, persistErr)
		return persistErr
	}
	if pruneErr != nil {
		log.Printf("prune completed jobs: %v", pruneErr)
	}
	switch status {
	case Running:
		m.appendLog(id, "task", joblogs.Info, event.Message)
	case Succeeded:
		m.appendLog(id, "task", joblogs.Info, event.Message)
		m.finalizeLog(id)
	case Failed, Interrupted:
		m.appendLog(id, "task", joblogs.Error, event.Message)
		m.finalizeLog(id)
	}
	return nil
}

func (m *Manager) appendLog(jobID, source string, level joblogs.Level, message string) {
	if m.logs == nil || message == "" {
		return
	}
	if _, err := m.logs.Append(context.Background(), jobID, source, level, message); err != nil {
		log.Printf("append job log %s: %v", jobID, err)
	}
}

func (m *Manager) finalizeLog(jobID string) {
	if m.logs == nil {
		return
	}
	if err := m.logs.Finalize(context.Background(), jobID); err != nil {
		log.Printf("finalize job log %s: %v", jobID, err)
	}
}
func (m *Manager) Get(id string) (Job, bool) {
	m.mu.RLock()
	j, ok := m.jobs[id]
	m.mu.RUnlock()
	if !ok && m.repo != nil {
		record, found, err := m.repo.LoadJob(id)
		if err == nil && found {
			return fromRecord(record), true
		}
	}
	return j, ok
}

func (m *Manager) Details(id string) (Job, []domain.JobEvent, bool, error) {
	m.mu.RLock()
	job, ok := m.jobs[id]
	if ok {
		events := append([]domain.JobEvent(nil), m.events[id]...)
		m.mu.RUnlock()
		return job, events, true, nil
	}
	m.mu.RUnlock()
	if m.repo == nil {
		return Job{}, nil, false, nil
	}
	record, found, err := m.repo.LoadJob(id)
	if err != nil || !found {
		return Job{}, nil, found, err
	}
	events, err := m.repo.JobEvents(id)
	if err != nil {
		return Job{}, nil, false, err
	}
	return fromRecord(record), events, true, nil
}

func toRecord(j Job) domain.JobRecord {
	return domain.JobRecord{ID: j.ID, InstanceID: j.InstanceID, Type: j.Type, Stage: j.Stage, Message: j.Message, Status: string(j.Status), Error: j.Error, Percent: j.Percent, TimeoutMinutes: j.TimeoutMinutes, CreatedAt: j.CreatedAt, UpdatedAt: j.UpdatedAt, StartedAt: j.StartedAt, FinishedAt: j.FinishedAt}
}
func fromRecord(v domain.JobRecord) Job {
	return Job{ID: v.ID, InstanceID: v.InstanceID, Type: v.Type, Stage: v.Stage, Message: v.Message, Status: Status(v.Status), Error: v.Error, Percent: v.Percent, TimeoutMinutes: domain.NormalizeJobTimeoutMinutes(v.TimeoutMinutes), CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, StartedAt: v.StartedAt, FinishedAt: v.FinishedAt}
}

package jobs

import (
	"context"
	"github.com/google/uuid"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"sync"
	"time"
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
	Percent                              int
	Error                                string
	CreatedAt, UpdatedAt                 time.Time
}
type Reporter interface {
	Progress(stage string, percent int, message string)
}
type reporter struct {
	m  *Manager
	id string
}

func (r reporter) Progress(stage string, percent int, message string) {
	r.m.mu.Lock()
	j := r.m.jobs[r.id]
	j.Stage, j.Percent, j.Message, j.UpdatedAt = stage, percent, message, time.Now().UTC()
	r.m.jobs[r.id] = j
	if r.m.repo != nil {
		_ = r.m.repo.SaveJob(toRecord(j))
	}
	r.m.mu.Unlock()
}

type Manager struct {
	mu    sync.RWMutex
	jobs  map[string]Job
	locks map[string]*sync.Mutex
	repo  Repository
	wg    sync.WaitGroup
}

type Repository interface {
	SaveJob(domain.JobRecord) error
	LoadJob(string) (domain.JobRecord, bool, error)
}

func NewManager() *Manager { return &Manager{jobs: map[string]Job{}, locks: map[string]*sync.Mutex{}} }
func NewPersistentManager(repo Repository) *Manager {
	m := NewManager()
	m.repo = repo
	if recovery, ok := repo.(interface{ RecoverJobs() error }); ok {
		_ = recovery.RecoverJobs()
	}
	return m
}
func (m *Manager) Start(ctx context.Context, instanceID, kind string, fn func(context.Context, Reporter) error) (Job, error) {
	now := time.Now().UTC()
	j := Job{ID: uuid.NewString(), InstanceID: instanceID, Type: kind, Status: Pending, CreatedAt: now, UpdatedAt: now}
	if m.repo != nil {
		if err := m.repo.SaveJob(toRecord(j)); err != nil {
			return Job{}, err
		}
	}
	m.mu.Lock()
	m.jobs[j.ID] = j
	lock := m.locks[instanceID]
	if lock == nil {
		lock = &sync.Mutex{}
		m.locks[instanceID] = lock
	}
	m.wg.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.wg.Done()
		lock.Lock()
		defer lock.Unlock()
		m.setStatus(j.ID, Running, 0, "")
		err := fn(ctx, reporter{m: m, id: j.ID})
		if err != nil {
			m.setStatus(j.ID, Failed, -1, err.Error())
		} else {
			m.setStatus(j.ID, Succeeded, 100, "")
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
func (m *Manager) setStatus(id string, status Status, percent int, message string) {
	m.mu.Lock()
	j := m.jobs[id]
	j.Status, j.UpdatedAt = status, time.Now().UTC()
	if percent >= 0 {
		j.Percent = percent
	}
	j.Error = message
	m.jobs[id] = j
	if m.repo != nil {
		_ = m.repo.SaveJob(toRecord(j))
	}
	m.mu.Unlock()
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

func toRecord(j Job) domain.JobRecord {
	return domain.JobRecord{ID: j.ID, InstanceID: j.InstanceID, Type: j.Type, Stage: j.Stage, Message: j.Message, Status: string(j.Status), Error: j.Error, Percent: j.Percent, CreatedAt: j.CreatedAt, UpdatedAt: j.UpdatedAt}
}
func fromRecord(v domain.JobRecord) Job {
	return Job{ID: v.ID, InstanceID: v.InstanceID, Type: v.Type, Stage: v.Stage, Message: v.Message, Status: Status(v.Status), Error: v.Error, Percent: v.Percent, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}

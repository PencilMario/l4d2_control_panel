package jobs

import (
	"context"
	"github.com/google/uuid"
	"sync"
	"time"
)

type Status string

const (
	Pending   Status = "pending"
	Running   Status = "running"
	Succeeded Status = "succeeded"
	Failed    Status = "failed"
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
	r.m.mu.Unlock()
}

type Manager struct {
	mu    sync.RWMutex
	jobs  map[string]Job
	locks map[string]*sync.Mutex
}

func NewManager() *Manager { return &Manager{jobs: map[string]Job{}, locks: map[string]*sync.Mutex{}} }
func (m *Manager) Start(ctx context.Context, instanceID, kind string, fn func(context.Context, Reporter) error) Job {
	now := time.Now().UTC()
	j := Job{ID: uuid.NewString(), InstanceID: instanceID, Type: kind, Status: Pending, CreatedAt: now, UpdatedAt: now}
	m.mu.Lock()
	m.jobs[j.ID] = j
	lock := m.locks[instanceID]
	if lock == nil {
		lock = &sync.Mutex{}
		m.locks[instanceID] = lock
	}
	m.mu.Unlock()
	go func() {
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
	return j
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
	m.mu.Unlock()
}
func (m *Manager) Get(id string) (Job, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	j, ok := m.jobs[id]
	return j, ok
}

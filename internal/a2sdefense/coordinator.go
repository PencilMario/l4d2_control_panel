package a2sdefense

import (
	"context"
	"errors"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

var ErrNotProtected = errors.New("A2S defense is not synchronized for instance ports")

type Repository interface {
	Instances(context.Context) ([]domain.Instance, error)
	A2SDefenseSettings(context.Context) (domain.A2SDefenseSettings, error)
	SaveA2SDefenseSettings(context.Context, domain.A2SDefenseSettings) error
}

type Coordinator struct {
	mu       sync.Mutex
	repo     Repository
	firewall Firewall
	interval time.Duration
	now      func() time.Time
	cancel   context.CancelFunc
	done     chan struct{}
}

func NewCoordinator(repo Repository, firewall Firewall, interval time.Duration, now func() time.Time) *Coordinator {
	if interval <= 0 {
		interval = time.Minute
	}
	if now == nil {
		now = time.Now
	}
	return &Coordinator{repo: repo, firewall: firewall, interval: interval, now: now}
}

func (c *Coordinator) SetEnabled(ctx context.Context, enabled bool) (Status, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	settings, err := c.repo.A2SDefenseSettings(ctx)
	if err != nil {
		return Status{}, err
	}
	ports, err := c.ports(ctx, enabled)
	if err != nil {
		return Status{}, err
	}
	config := Config{Version: APIVersion, Enabled: enabled, Ports: ports, Revision: settings.Revision + 1}
	if config.Revision < 1 {
		config.Revision = 1
	}
	status, err := c.firewall.Apply(ctx, config)
	if err != nil {
		settings.Pending = true
		settings.LastError = err.Error()
		_ = c.repo.SaveA2SDefenseSettings(ctx, settings)
		return Status{}, err
	}
	settings.Enabled = enabled
	settings.Revision = status.Revision
	settings.Pending = false
	settings.LastError = ""
	settings.LastSyncedAt = c.now().UTC()
	if err := c.repo.SaveA2SDefenseSettings(ctx, settings); err != nil {
		return Status{}, err
	}
	return status, nil
}

func (c *Coordinator) Reconcile(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	settings, err := c.repo.A2SDefenseSettings(ctx)
	if err != nil {
		return err
	}
	actual, err := c.firewall.Status(ctx)
	if err != nil {
		return c.markPending(ctx, settings, err)
	}
	ports, err := c.ports(ctx, settings.Enabled)
	if err != nil {
		return c.markPending(ctx, settings, err)
	}
	if actual.Enabled == settings.Enabled && slices.Equal(actual.Ports, ports) && actual.Revision == settings.Revision && !settings.Pending {
		return nil
	}
	revision := max(settings.Revision, actual.Revision) + 1
	if revision < 1 {
		revision = 1
	}
	status, err := c.firewall.Apply(ctx, Config{Version: APIVersion, Enabled: settings.Enabled, Ports: ports, Revision: revision})
	if err != nil {
		return c.markPending(ctx, settings, err)
	}
	settings.Revision = status.Revision
	settings.Pending = false
	settings.LastError = ""
	settings.LastSyncedAt = c.now().UTC()
	return c.repo.SaveA2SDefenseSettings(ctx, settings)
}

func (c *Coordinator) Desired(ctx context.Context) (domain.A2SDefenseSettings, error) {
	return c.repo.A2SDefenseSettings(ctx)
}

func (c *Coordinator) Actual(ctx context.Context) (Status, error) {
	return c.firewall.Status(ctx)
}

func (c *Coordinator) EnsureProtected(ctx context.Context, instance domain.Instance) error {
	settings, err := c.repo.A2SDefenseSettings(ctx)
	if err != nil || !settings.Enabled {
		return err
	}
	if err := c.Reconcile(ctx); err != nil {
		return err
	}
	settings, err = c.repo.A2SDefenseSettings(ctx)
	if err != nil {
		return err
	}
	actual, err := c.firewall.Status(ctx)
	if err != nil {
		return err
	}
	if !actual.Enabled || actual.Revision != settings.Revision || !containsPort(actual.Ports, instance.GamePort) || instance.SourceTVPort > 0 && !containsPort(actual.Ports, instance.SourceTVPort) {
		return ErrNotProtected
	}
	return nil
}

func (c *Coordinator) Start(parent context.Context) {
	c.mu.Lock()
	if c.cancel != nil {
		c.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	c.cancel = cancel
	c.done = make(chan struct{})
	done := c.done
	c.mu.Unlock()
	go func() {
		defer close(done)
		_ = c.Reconcile(ctx)
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = c.Reconcile(ctx)
			}
		}
	}()
}

func (c *Coordinator) Stop() {
	c.mu.Lock()
	cancel, done := c.cancel, c.done
	c.cancel, c.done = nil, nil
	c.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

func (c *Coordinator) ports(ctx context.Context, enabled bool) ([]int, error) {
	if !enabled {
		return nil, nil
	}
	instances, err := c.repo.Instances(ctx)
	if err != nil {
		return nil, err
	}
	unique := map[int]struct{}{}
	for _, instance := range instances {
		if instance.GamePort > 0 {
			unique[instance.GamePort] = struct{}{}
		}
		if instance.SourceTVPort > 0 {
			unique[instance.SourceTVPort] = struct{}{}
		}
	}
	ports := make([]int, 0, len(unique))
	for port := range unique {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	return ports, nil
}

func (c *Coordinator) markPending(ctx context.Context, settings domain.A2SDefenseSettings, cause error) error {
	settings.Pending = true
	settings.LastError = cause.Error()
	if err := c.repo.SaveA2SDefenseSettings(ctx, settings); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func containsPort(ports []int, target int) bool {
	_, found := slices.BinarySearch(ports, target)
	return found
}

package a2sdefense

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	iptablesPath        = "/sbin/iptables"
	iptablesRestorePath = "/sbin/iptables-restore"
	iptablesSavePath    = "/sbin/iptables-save"
)

var ErrStaleRevision = errors.New("stale A2S defense revision")

type Executor interface {
	Run(context.Context, string, []string, string) (string, error)
}

type CommandExecutor struct{}

func (CommandExecutor) Run(ctx context.Context, name string, args []string, input string) (string, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = strings.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%s failed: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

type Firewall interface {
	Apply(context.Context, Config) (Status, error)
	Status(context.Context) (Status, error)
}

type Manager struct {
	mu       sync.Mutex
	executor Executor
	now      func() time.Time
	config   Config
	status   Status
}

func NewManager(executor Executor, now func() time.Time) *Manager {
	if now == nil {
		now = time.Now
	}
	return &Manager{executor: executor, now: now, status: Status{Compatible: true, PolicyVersion: PolicyVersion}}
}

func (m *Manager) Apply(ctx context.Context, input Config) (Status, error) {
	config, err := NormalizeConfig(input)
	if err != nil {
		return Status{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status.Revision > 0 && config.Revision < m.status.Revision {
		return Status{}, ErrStaleRevision
	}
	if config.Revision == m.status.Revision && configsEqual(config, m.config) {
		return cloneStatus(m.status), nil
	}
	if config.Revision == m.status.Revision && m.status.Revision > 0 {
		return Status{}, ErrStaleRevision
	}

	save, err := m.executor.Run(ctx, iptablesSavePath, []string{"-t", "filter"}, "")
	if err != nil {
		return m.failedStatus(err)
	}
	if config.Enabled {
		inactive := slotA
		if strings.Contains(save, "-A "+InputChain+" -j "+SlotAChain) {
			inactive = slotB
		}
		rules, buildErr := buildEnableRestore(config, inactive)
		if buildErr != nil {
			return m.failedStatus(buildErr)
		}
		if _, err = m.executor.Run(ctx, iptablesRestorePath, []string{"--noflush", "--wait", "5"}, rules); err != nil {
			return m.failedStatus(err)
		}
		if !hasInputJump(save) {
			if _, err = m.executor.Run(ctx, iptablesPath, []string{"--wait", "5", "-I", "INPUT", "1", "-j", InputChain}, ""); err != nil {
				return m.failedStatus(err)
			}
		}
	} else {
		if hasInputJump(save) {
			if _, err = m.executor.Run(ctx, iptablesPath, []string{"--wait", "5", "-D", "INPUT", "-j", InputChain}, ""); err != nil {
				return m.failedStatus(err)
			}
		}
		if _, err = m.executor.Run(ctx, iptablesRestorePath, []string{"--noflush", "--wait", "5"}, BuildDisableRestore()); err != nil {
			return m.failedStatus(err)
		}
	}
	m.config = config
	m.status = Status{
		Compatible:    true,
		Enabled:       config.Enabled,
		Revision:      config.Revision,
		PolicyVersion: PolicyVersion,
		Ports:         append([]int(nil), config.Ports...),
		AppliedAt:     m.now().UTC().Format(time.RFC3339),
	}
	return cloneStatus(m.status), nil
}

func (m *Manager) Status(context.Context) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneStatus(m.status), nil
}

func (m *Manager) failedStatus(err error) (Status, error) {
	m.status.LastError = err.Error()
	return cloneStatus(m.status), err
}

func hasInputJump(save string) bool {
	return strings.Contains(save, "-A INPUT -j "+InputChain)
}

func configsEqual(left, right Config) bool {
	return left.Version == right.Version && left.Enabled == right.Enabled && left.Revision == right.Revision && slices.Equal(left.Ports, right.Ports)
}

func cloneStatus(input Status) Status {
	input.Ports = append([]int(nil), input.Ports...)
	return input
}

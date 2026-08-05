package a2sdefense

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	iptablesPath        = "/usr/sbin/iptables"
	iptablesRestorePath = "/usr/sbin/iptables-restore"
	iptablesSavePath    = "/usr/sbin/iptables-save"
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
	readFile func(string) ([]byte, error)
}

func NewManager(executor Executor, now func() time.Time) *Manager {
	if now == nil {
		now = time.Now
	}
	return &Manager{executor: executor, now: now, readFile: os.ReadFile, status: Status{Compatible: true, PolicyVersion: PolicyVersion}}
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

func (m *Manager) Status(ctx context.Context) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.preflight(ctx); err != nil {
		m.status.Compatible = false
		m.status.LastError = err.Error()
		return cloneStatus(m.status), err
	}
	m.status.Compatible = true
	if !m.status.Enabled {
		return cloneStatus(m.status), nil
	}
	save, err := m.executor.Run(ctx, iptablesSavePath, []string{"-c", "-t", "filter"}, "")
	if err != nil {
		return m.failedStatus(err)
	}
	m.status.Counters = parseCounters(save)
	recent, err := m.readFile("/proc/net/xt_recent/" + RecentName)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return m.failedStatus(err)
	}
	m.status.BlacklistSize = countRecentEntries(string(recent))
	return cloneStatus(m.status), nil
}

func (m *Manager) preflight(ctx context.Context) error {
	if _, err := m.executor.Run(ctx, iptablesPath, []string{"--version"}, ""); err != nil {
		return fmt.Errorf("iptables unavailable: %w", err)
	}
	for _, match := range []string{"u32", "hashlimit", "recent", "multiport"} {
		if _, err := m.executor.Run(ctx, iptablesPath, []string{"-m", match, "-h"}, ""); err != nil {
			return fmt.Errorf("iptables match %s unavailable: %w", match, err)
		}
	}
	return nil
}

func parseCounters(save string) Counters {
	var counters Counters
	for _, line := range strings.Split(save, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasPrefix(fields[0], "[") {
			continue
		}
		packets, ok := parsePacketCounter(fields[0])
		if !ok {
			continue
		}
		name := argumentValue(fields, "--hashlimit-name")
		switch name {
		case "L4D2_A2S_INFO":
			counters.Info += packets
		case "L4D2_A2S_PLAYER":
			counters.Player += packets
		case "L4D2_A2S_RULES":
			counters.Rules += packets
		case "L4D2_A2S_CHALLENGE":
			counters.Challenge += packets
		case "L4D2_A2S_OTHER69":
			counters.Other69 += packets
		case "L4D2_A2S_TOTAL":
			counters.Aggregate += packets
		}
		target := argumentValue(fields, "-j")
		if name == "" && argumentValue(fields, "--name") == RecentName && (target == "DROP" || target == DropChain || target == DropB) {
			counters.Blacklist += packets
		}
	}
	return counters
}

func parsePacketCounter(field string) (uint64, bool) {
	value := strings.Trim(field, "[]")
	packets, _, found := strings.Cut(value, ":")
	if !found {
		return 0, false
	}
	parsed, err := strconv.ParseUint(packets, 10, 64)
	return parsed, err == nil
}

func argumentValue(fields []string, name string) string {
	for index := 0; index+1 < len(fields); index++ {
		if fields[index] == name {
			return fields[index+1]
		}
	}
	return ""
}

func countRecentEntries(input string) int {
	count := 0
	for _, line := range strings.Split(input, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "src=") {
			count++
		}
	}
	return count
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

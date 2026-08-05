package a2sdefense

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type execCall struct {
	name  string
	args  []string
	input string
}

type fakeExecutor struct {
	calls  []execCall
	save   string
	failAt int
	failOn string
}

func (f *fakeExecutor) Run(_ context.Context, name string, args []string, input string) (string, error) {
	f.calls = append(f.calls, execCall{name: name, args: append([]string(nil), args...), input: input})
	if f.failAt > 0 && len(f.calls) == f.failAt {
		return "", errors.New("command failed")
	}
	if f.failOn != "" && strings.Contains(name+" "+strings.Join(args, " "), f.failOn) {
		return "", errors.New("command failed")
	}
	if strings.HasSuffix(name, "iptables-save") {
		return f.save, nil
	}
	return "", nil
}

func TestManagerStatusReadsLiveCountersAndRecentBlacklist(t *testing.T) {
	executor := &fakeExecutor{save: `*filter
[0:0] -A L4D2_A2S_CLASS_A -m hashlimit --hashlimit-name L4D2_A2S_TOTAL -j L4D2_A2S_DROP_A
[3:180] -A L4D2_A2S_CLASS_A -m hashlimit --hashlimit-name L4D2_A2S_INFO -j L4D2_A2S_DROP_A
[7:420] -A L4D2_A2S_CLASS_A -m hashlimit --hashlimit-name L4D2_A2S_PLAYER -j L4D2_A2S_DROP_A
[2:120] -A L4D2_A2S_CLASS_B -m hashlimit --hashlimit-name L4D2_A2S_PLAYER -j L4D2_A2S_DROP_B
[5:300] -A L4D2_A2S_CLASS_A -m hashlimit --hashlimit-name L4D2_A2S_RULES -j L4D2_A2S_DROP_A
[11:660] -A L4D2_A2S_SLOT_A -m recent --name L4D2_A2S_ATTACKER -j L4D2_A2S_DROP_A
[7:420] -A L4D2_A2S_SLOT_B -m recent --name L4D2_A2S_ATTACKER -j L4D2_A2S_DROP_B
COMMIT
`}
	manager := NewManager(executor, time.Now)
	manager.status = Status{Compatible: true, Enabled: true, Revision: 1, PolicyVersion: PolicyVersion}
	manager.readFile = func(path string) ([]byte, error) {
		if path != "/proc/net/xt_recent/"+RecentName {
			t.Fatalf("path=%q", path)
		}
		return []byte("src=192.0.2.1 ttl: 64\nsrc=192.0.2.2 ttl: 64\n"), nil
	}

	status, err := manager.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Counters.Info != 3 || status.Counters.Player != 9 || status.Counters.Rules != 5 || status.Counters.Aggregate != 0 || status.Counters.Blacklist != 18 {
		t.Fatalf("counters=%+v", status.Counters)
	}
	if status.BlacklistSize != 2 {
		t.Fatalf("blacklist_size=%d", status.BlacklistSize)
	}
}

func TestCommandPathsMatchAlpineIptablesPackage(t *testing.T) {
	if iptablesPath != "/usr/sbin/iptables" || iptablesSavePath != "/usr/sbin/iptables-save" || iptablesRestorePath != "/usr/sbin/iptables-restore" {
		t.Fatalf("command paths = %q %q %q", iptablesPath, iptablesSavePath, iptablesRestorePath)
	}
}

func TestParseCountersKeepsLegacyDirectBlacklistDrop(t *testing.T) {
	counters := parseCounters("[4:240] -A L4D2_A2S_SLOT_A -m recent --name L4D2_A2S_ATTACKER -j DROP\n")
	if counters.Blacklist != 4 {
		t.Fatalf("blacklist=%d", counters.Blacklist)
	}
}

func TestManagerStatusReportsIncompatibleRequiredMatch(t *testing.T) {
	executor := &fakeExecutor{failOn: "-m u32 -h"}
	manager := NewManager(executor, time.Now)

	status, err := manager.Status(context.Background())
	if err == nil || status.Compatible || !strings.Contains(status.LastError, "u32") {
		t.Fatalf("status=%+v err=%v", status, err)
	}
}

func TestManagerApplyUsesFixedCommandsAndReturnsEffectiveStatus(t *testing.T) {
	executor := &fakeExecutor{save: "*filter\n:INPUT ACCEPT [0:0]\nCOMMIT\n"}
	manager := NewManager(executor, func() time.Time { return time.Date(2026, 8, 5, 1, 2, 3, 0, time.UTC) })
	status, err := manager.Apply(context.Background(), Config{Version: APIVersion, Enabled: true, Ports: []int{27020, 27015}, Revision: 4})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || status.Revision != 4 || status.AppliedAt != "2026-08-05T01:02:03Z" || len(status.Ports) != 2 || status.Ports[0] != 27015 {
		t.Fatalf("status=%+v", status)
	}
	joined := ""
	for _, call := range executor.calls {
		joined += call.name + " " + strings.Join(call.args, " ") + "\n" + call.input
	}
	for _, required := range []string{iptablesSavePath, iptablesRestorePath, "--noflush --wait 5", InputChain, "--dports 27015,27020", iptablesPath, "-I INPUT 1 -j " + InputChain} {
		if !strings.Contains(joined, required) {
			t.Errorf("commands missing %q\n%s", required, joined)
		}
	}
}

func TestManagerRejectsStaleRevisionWithoutCommands(t *testing.T) {
	executor := &fakeExecutor{save: "*filter\n:INPUT ACCEPT [0:0]\nCOMMIT\n"}
	manager := NewManager(executor, time.Now)
	if _, err := manager.Apply(context.Background(), Config{Version: APIVersion, Enabled: true, Ports: []int{27015}, Revision: 5}); err != nil {
		t.Fatal(err)
	}
	count := len(executor.calls)
	if _, err := manager.Apply(context.Background(), Config{Version: APIVersion, Enabled: true, Ports: []int{27015}, Revision: 4}); !errors.Is(err, ErrStaleRevision) {
		t.Fatalf("err=%v", err)
	}
	if len(executor.calls) != count {
		t.Fatal("stale request executed commands")
	}
}

func TestManagerFailedApplyPreservesPreviousStatus(t *testing.T) {
	executor := &fakeExecutor{save: "*filter\n:INPUT ACCEPT [0:0]\nCOMMIT\n"}
	manager := NewManager(executor, time.Now)
	previous, err := manager.Apply(context.Background(), Config{Version: APIVersion, Enabled: true, Ports: []int{27015}, Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	executor.failAt = len(executor.calls) + 2
	if _, err := manager.Apply(context.Background(), Config{Version: APIVersion, Enabled: true, Ports: []int{27016}, Revision: 2}); err == nil {
		t.Fatal("expected apply failure")
	}
	status, err := manager.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Revision != previous.Revision || status.Ports[0] != previous.Ports[0] || status.LastError == "" {
		t.Fatalf("status=%+v previous=%+v", status, previous)
	}
}

func TestManagerDisableDeletesOnlyProjectJump(t *testing.T) {
	executor := &fakeExecutor{save: "*filter\n:INPUT ACCEPT [0:0]\n-A INPUT -j " + InputChain + "\nCOMMIT\n"}
	manager := NewManager(executor, time.Now)
	status, err := manager.Apply(context.Background(), Config{Version: APIVersion, Enabled: false, Revision: 3})
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled || status.Revision != 3 {
		t.Fatalf("status=%+v", status)
	}
	joined := ""
	for _, call := range executor.calls {
		joined += call.name + " " + strings.Join(call.args, " ") + "\n" + call.input
	}
	if !strings.Contains(joined, "-D INPUT -j "+InputChain) || strings.Contains(joined, "-F INPUT") || strings.Contains(joined, "-P INPUT") {
		t.Fatalf("unsafe disable commands:\n%s", joined)
	}
}

func TestManagerBuildsInactiveSlotBeforeSwitchingDispatcher(t *testing.T) {
	executor := &fakeExecutor{save: "*filter\n:INPUT ACCEPT [0:0]\n:" + InputChain + " - [0:0]\n-A INPUT -j " + InputChain + "\n-A " + InputChain + " -j " + SlotAChain + "\nCOMMIT\n"}
	manager := NewManager(executor, time.Now)
	if _, err := manager.Apply(context.Background(), Config{Version: APIVersion, Enabled: true, Ports: []int{27016}, Revision: 2}); err != nil {
		t.Fatal(err)
	}
	restore := ""
	for _, call := range executor.calls {
		if call.name == iptablesRestorePath {
			restore = call.input
		}
	}
	if !strings.Contains(restore, "-F "+SlotBChain) || !strings.Contains(restore, "-A "+InputChain+" -j "+SlotBChain) {
		t.Fatalf("restore did not build and switch to inactive slot B:\n%s", restore)
	}
	if strings.Contains(restore, "-F "+SlotAChain) {
		t.Fatalf("restore flushed active slot A:\n%s", restore)
	}
}

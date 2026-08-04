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
}

func (f *fakeExecutor) Run(_ context.Context, name string, args []string, input string) (string, error) {
	f.calls = append(f.calls, execCall{name: name, args: append([]string(nil), args...), input: input})
	if f.failAt > 0 && len(f.calls) == f.failAt {
		return "", errors.New("command failed")
	}
	if strings.HasSuffix(name, "iptables-save") {
		return f.save, nil
	}
	return "", nil
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

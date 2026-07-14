package docker

import (
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildContainerSpecUsesManagedHostNetwork(t *testing.T) {
	root := t.TempDir()
	v := domain.Instance{ID: "abc", RuntimeImage: "runtime:v1", GamePort: 27015, StartMap: "c2m1_highway", GameMode: "coop", Tickrate: 100, MaxPlayers: 8}
	spec := BuildContainerSpec(root, v)
	if spec.NetworkMode != "host" || spec.Labels[ManagedLabel] != "true" || spec.Labels[InstanceLabel] != "abc" {
		t.Fatalf("unsafe spec: %#v", spec)
	}
	want := filepath.Join(root, "instances", "abc", "game") + ":/opt/l4d2/game"
	if spec.Mounts[0] != want {
		t.Fatalf("mount=%q want=%q", spec.Mounts[0], want)
	}
	joined := strings.Join(spec.Env, "|")
	if !strings.Contains(joined, "SRCDS_PORT=27015") || !strings.Contains(joined, "SRCDS_MAP=c2m1_highway") {
		t.Fatalf("env=%v", spec.Env)
	}
}
func TestSupervisorExecRejectsUnknownOperation(t *testing.T) {
	if _, err := SupervisorExec("abc", "sh"); err == nil {
		t.Fatal("expected rejection")
	}
	cmd, err := SupervisorExec("abc", "status")
	if err != nil || len(cmd.Args) != 3 || cmd.Args[1] != "status" || cmd.Args[2] != "--json" {
		t.Fatalf("cmd=%#v err=%v", cmd, err)
	}
}

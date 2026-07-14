package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

func TestOpenEnablesWALAndMigrates(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var mode string
	if err := s.DB().QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("journal mode = %q", mode)
	}
	var count int
	if err := s.DB().QueryRow("SELECT count(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("expected applied migration")
	}
}

func TestInstanceCRUD(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	want := domain.Instance{ID: "instance-1", NodeID: "local", Name: "Coop One", GamePort: 27015, StartMap: "c2m1_highway", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "l4d2-server-runtime:latest", DesiredState: domain.StateStopped, ActualState: domain.StateUninstalled}
	if err := s.CreateInstance(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Instance(ctx, want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != want.Name || got.NodeID != "local" || got.GamePort != 27015 {
		t.Fatalf("unexpected instance: %#v", got)
	}
	got.Name = "Renamed"
	if err := s.UpdateInstance(ctx, got); err != nil {
		t.Fatal(err)
	}
	all, err := s.Instances(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Name != "Renamed" {
		t.Fatalf("unexpected instances: %#v", all)
	}
	if err := s.DeleteInstance(ctx, got.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Instance(ctx, got.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPluginPortMigrationSurvivesReopenAndCascades(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	instance := domain.Instance{ID: "instance-ports", NodeID: "local", Name: "Ports", GamePort: 27015, SourceTVPort: 27020, PluginPorts: []int{27021, 27022}, StartMap: "map", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", DesiredState: domain.StateStopped, ActualState: domain.StateUninstalled}
	if err := s.CreateInstance(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	got, err := s.Instance(context.Background(), instance.ID)
	if err != nil || len(got.PluginPorts) != 2 || got.PluginPorts[0] != 27021 || got.PluginPorts[1] != 27022 {
		t.Fatalf("created=%#v err=%v", got, err)
	}
	got.PluginPorts = []int{27031}
	if err := s.UpdateInstance(context.Background(), got); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err = s.Instance(context.Background(), instance.ID)
	if err != nil || len(got.PluginPorts) != 1 || got.PluginPorts[0] != 27031 {
		t.Fatalf("reopened=%#v err=%v", got, err)
	}
	if err := s.DeleteInstance(context.Background(), instance.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.DB().QueryRow(`SELECT count(*) FROM instance_plugin_ports WHERE instance_id=?`, instance.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("orphan count=%d err=%v", count, err)
	}
}

func TestAuditEventsPersist(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	event := domain.AuditRecord{ID: "audit-1", Action: "POST", Target: "/api/instances", Result: "201", Metadata: `{"remote":"127.0.0.1"}`}
	if err := s.RecordAudit(ctx, event); err != nil {
		t.Fatal(err)
	}
	events, err := s.AuditEvents(ctx, 10)
	if err != nil || len(events) != 1 || events[0].Action != "POST" {
		t.Fatalf("events=%#v err=%v", events, err)
	}
}

func TestScheduledTasksPersist(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	task := domain.ScheduledTask{ID: "task-1", InstanceID: "abc", Type: "game_update", Cron: "0 4 * * *", Timezone: "Asia/Hong_Kong", OnlinePolicy: "skip", Payload: `{}`, Enabled: true}
	if err := s.SaveScheduledTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	tasks, err := s.ScheduledTasks(ctx)
	if err != nil || len(tasks) != 1 || tasks[0].Timezone != "Asia/Hong_Kong" {
		t.Fatalf("tasks=%#v err=%v", tasks, err)
	}
	if err := s.DeleteScheduledTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
}

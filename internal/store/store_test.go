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

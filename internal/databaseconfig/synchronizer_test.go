package databaseconfig

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

type syncRepo struct {
	config    Config
	instances []domain.Instance
}

func (r syncRepo) DatabaseConfig(context.Context) (Config, error)       { return r.config, nil }
func (r syncRepo) Instances(context.Context) ([]domain.Instance, error) { return r.instances, nil }

func TestDatabaseSyncWritesInstalledAndDefersUninstalled(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "instances", "one", "game", "left4dead2"), 0750); err != nil {
		t.Fatal(err)
	}
	s := Synchronizer{Root: root, Repository: syncRepo{config: Defaults(), instances: []domain.Instance{{ID: "one", Name: "One"}, {ID: "two", Name: "Two"}}}}
	result, err := s.SyncAll(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Synced != 1 || result.Deferred != 1 {
		t.Fatalf("result=%+v", result)
	}
	raw, err := os.ReadFile(filepath.Join(root, "instances", "one", "game", "left4dead2", "addons", "sourcemod", "configs", "databases.cfg"))
	if err != nil || len(raw) == 0 {
		t.Fatalf("file=%q err=%v", raw, err)
	}
}

package updates

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

type pipelineOverlayResetter struct{ calls [][]string }

func (r *pipelineOverlayResetter) ResetManagedPaths(_ context.Context, _, _ string, paths []string) error {
	r.calls = append(r.calls, append([]string(nil), paths...))
	return nil
}

type pipelineSharedState struct{ state domain.SharedGameState }

func (s pipelineSharedState) SharedGameState(context.Context) (domain.SharedGameState, error) {
	return s.state, nil
}

func TestFullPackageUpdateResetsManagedOverlayPaths(t *testing.T) {
	root := t.TempDir()
	resetter := &pipelineOverlayResetter{}
	pipeline := New(root).WithSharedOverlay(resetter, pipelineSharedState{state: domain.SharedGameState{ActiveReleaseID: "release-1", MigrationState: "ready"}})
	if err := pipeline.Apply(context.Background(), "abc", zipFile(t, map[string]string{"cfg/plugin.cfg": "new"}), "v1", Full); err != nil {
		t.Fatal(err)
	}
	if len(resetter.calls) != 1 || len(resetter.calls[0]) != 1 || resetter.calls[0][0] != "left4dead2/cfg/plugin.cfg" {
		t.Fatalf("calls=%#v", resetter.calls)
	}
}

func TestHotPackageUpdateDoesNotUnmountOverlay(t *testing.T) {
	root := t.TempDir()
	resetter := &pipelineOverlayResetter{}
	pipeline := New(root).WithSharedOverlay(resetter, pipelineSharedState{state: domain.SharedGameState{ActiveReleaseID: "release-1", MigrationState: "ready"}})
	if err := pipeline.Apply(context.Background(), "abc", zipFile(t, map[string]string{"cfg/plugin.cfg": "new"}), "v1", Hot); err != nil {
		t.Fatal(err)
	}
	if len(resetter.calls) != 0 {
		t.Fatalf("calls=%#v", resetter.calls)
	}
}

func TestHotUpdateAppliesAllowedFilesAndPrivateLast(t *testing.T) {
	root := t.TempDir()
	archive := zipFile(t, map[string]string{"cfg/plugin.cfg": "package", "addons/sourcemod/plugins/x.smx": "binary"})
	private := filepath.Join(root, "instances", "abc", "private", "cfg")
	_ = os.MkdirAll(private, 0750)
	_ = os.WriteFile(filepath.Join(private, "plugin.cfg"), []byte("private"), 0640)
	pipeline := New(root)
	if err := pipeline.Apply(context.Background(), "abc", archive, "v1", Hot); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(root, "instances", "abc", "game", "left4dead2", "cfg", "plugin.cfg"))
	if string(raw) != "private" {
		t.Fatalf("got %q", raw)
	}
}

func TestPackageUpdateRebasesPrivateLowerLayer(t *testing.T) {
	root := t.TempDir()
	pipeline := New(root)
	ctx := context.Background()
	if err := pipeline.Apply(ctx, "abc", zipFile(t, map[string]string{"cfg/plugin.cfg": "package-v1"}), "v1", Hot); err != nil {
		t.Fatal(err)
	}
	private := content.NewPrivateManager(root, 1<<20)
	if _, err := private.Save(ctx, "abc", "cfg/plugin.cfg", []byte("private")); err != nil {
		t.Fatal(err)
	}
	if err := private.ApplyChanges(ctx, "abc"); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Apply(ctx, "abc", zipFile(t, map[string]string{"cfg/plugin.cfg": "package-v2"}), "v2", Hot); err != nil {
		t.Fatal(err)
	}
	if err := private.Delete(ctx, "abc", "cfg/plugin.cfg"); err != nil {
		t.Fatal(err)
	}
	if err := private.ApplyChanges(ctx, "abc"); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "instances", "abc", "game", "left4dead2", "cfg", "plugin.cfg")
	if raw, err := os.ReadFile(target); err != nil || string(raw) != "package-v2" {
		t.Fatalf("game=%q err=%v", raw, err)
	}
}

func TestPackageUpdateRebasesDeletedPrivatePath(t *testing.T) {
	root := t.TempDir()
	pipeline := New(root)
	ctx := context.Background()
	if err := pipeline.Apply(ctx, "abc", zipFile(t, map[string]string{"cfg/plugin.cfg": "package-v1"}), "v1", Hot); err != nil {
		t.Fatal(err)
	}
	private := content.NewPrivateManager(root, 1<<20)
	if _, err := private.Save(ctx, "abc", "cfg/plugin.cfg", []byte("private")); err != nil {
		t.Fatal(err)
	}
	if err := private.ApplyChanges(ctx, "abc"); err != nil {
		t.Fatal(err)
	}
	if err := private.Delete(ctx, "abc", "cfg/plugin.cfg"); err != nil {
		t.Fatal(err)
	}
	if err := pipeline.Apply(ctx, "abc", zipFile(t, map[string]string{"cfg/plugin.cfg": "package-v2"}), "v2", Hot); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "instances", "abc", "game", "left4dead2", "cfg", "plugin.cfg")
	if raw, err := os.ReadFile(target); err != nil || string(raw) != "package-v2" {
		t.Fatalf("game=%q err=%v", raw, err)
	}
}

func TestPackageRollbackRestoresPrivateLowerLayer(t *testing.T) {
	root := t.TempDir()
	pipeline := New(root)
	ctx := context.Background()
	if err := pipeline.Apply(ctx, "abc", zipFile(t, map[string]string{"cfg/plugin.cfg": "package-v1"}), "v1", Hot); err != nil {
		t.Fatal(err)
	}
	private := content.NewPrivateManager(root, 1<<20)
	if _, err := private.Save(ctx, "abc", "cfg/plugin.cfg", []byte("private")); err != nil {
		t.Fatal(err)
	}
	if err := private.ApplyChanges(ctx, "abc"); err != nil {
		t.Fatal(err)
	}
	deployment, err := pipeline.Begin(ctx, "abc", zipFile(t, map[string]string{"cfg/plugin.cfg": "package-v2"}), "v2", Hot)
	if err != nil {
		t.Fatal(err)
	}
	if err := deployment.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := private.Delete(ctx, "abc", "cfg/plugin.cfg"); err != nil {
		t.Fatal(err)
	}
	if err := private.ApplyChanges(ctx, "abc"); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "instances", "abc", "game", "left4dead2", "cfg", "plugin.cfg")
	if raw, err := os.ReadFile(target); err != nil || string(raw) != "package-v1" {
		t.Fatalf("game=%q err=%v", raw, err)
	}
}

func TestPipelinePrivateTransactionBlocksConcurrentSave(t *testing.T) {
	root := t.TempDir()
	p := New(root)
	ctx := context.Background()
	m := content.NewPrivateManager(root, 1<<20)
	started := make(chan struct{})
	done := make(chan error, 1)
	p.AfterDeploy = func() error {
		go func() { close(started); _, err := m.Save(ctx, "abc", "cfg/concurrent.cfg", []byte("x")); done <- err }()
		<-started
		select {
		case err := <-done:
			t.Fatalf("save escaped transaction: %v", err)
		case <-time.After(100 * time.Millisecond):
		}
		return errors.New("rollback")
	}
	if err := p.Apply(ctx, "abc", zipFile(t, map[string]string{"cfg/a.cfg": "new"}), "v2", Hot); err == nil {
		t.Fatal("expected rollback")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("save remained blocked")
	}
}

func TestPipelineRollbackDoesNotPrunePrivateSnapshots(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	m := content.NewPrivateManager(root, 1<<20)
	for i := 0; i < 20; i++ {
		if _, err := m.Save(ctx, "abc", "cfg/a.cfg", []byte(strconv.Itoa(i))); err != nil {
			t.Fatal(err)
		}
		if err := m.ApplyChanges(ctx, "abc"); err != nil {
			t.Fatal(err)
		}
	}
	before, err := m.Snapshots(ctx, "abc")
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := New(root).Begin(ctx, "abc", zipFile(t, map[string]string{"cfg/a.cfg": "package"}), "v2", Hot)
	if err != nil {
		t.Fatal(err)
	}
	if err := deployment.Rollback(); err != nil {
		t.Fatal(err)
	}
	after, err := m.Snapshots(ctx, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(before) != fmt.Sprint(after) {
		t.Fatalf("snapshots changed\nbefore=%v\nafter=%v", before, after)
	}
}

func TestPackageRollbackRestoresDeletedAppliedEmptyDirectory(t *testing.T) {
	root := t.TempDir()
	pipeline := New(root)
	ctx := context.Background()
	private := content.NewPrivateManager(root, 1<<20)
	if err := private.MakeDir(ctx, "abc", "cfg/empty"); err != nil {
		t.Fatal(err)
	}
	if err := private.ApplyChanges(ctx, "abc"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "instances", "abc", "private", "cfg", "empty")); err != nil {
		t.Fatal(err)
	}
	deployment, err := pipeline.Begin(ctx, "abc", zipFile(t, map[string]string{"cfg/empty/lower.cfg": "v2"}), "v2", Hot)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "instances", "abc", "game", "left4dead2", "cfg", "empty")
	if raw, err := os.ReadFile(filepath.Join(target, "lower.cfg")); err != nil || string(raw) != "v2" {
		t.Fatalf("lower subtree not deployed: %q %v", raw, err)
	}
	if err := deployment.Rollback(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		t.Fatalf("directory not restored: info=%v err=%v", info, err)
	}
	if entries, err := os.ReadDir(target); err != nil || len(entries) != 0 {
		t.Fatalf("directory not exact: entries=%v err=%v", entries, err)
	}
}

func TestPipelineJournalAvoidsUnrelatedNestedDirectoryBackup(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	base := filepath.Join(root, "instances", "abc")
	game := filepath.Join(base, "game", "left4dead2", "cfg")
	if err := os.MkdirAll(filepath.Join(game, "nested"), 0750); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		if err := os.WriteFile(filepath.Join(game, "nested", fmt.Sprintf("%02d.cfg", i)), []byte(strconv.Itoa(i)), 0640); err != nil {
			t.Fatal(err)
		}
	}
	m := content.NewPrivateManager(root, 1<<20)
	if err := m.MakeDir(ctx, "abc", "cfg"); err != nil {
		t.Fatal(err)
	}
	deployment, err := New(root).Begin(ctx, "abc", zipFile(t, map[string]string{"cfg/package.cfg": "new"}), "v2", Hot)
	if err != nil {
		t.Fatal(err)
	}
	journals, _ := filepath.Glob(filepath.Join(base, "backups", "update-*", "journal.json"))
	value, err := readJournal(journals[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(value.Affected) != 1 || value.Affected[0].Path != "cfg/package.cfg" || value.Affected[0].Existed {
		t.Fatalf("affected=%v", value.Affected)
	}
	backupRoot := filepath.Join(filepath.Dir(journals[0]), "replaced")
	if _, err := os.Lstat(backupRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unrelated directory was backed up: %v", err)
	}
	if err := deployment.Rollback(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		raw, err := os.ReadFile(filepath.Join(game, "nested", fmt.Sprintf("%02d.cfg", i)))
		if err != nil || string(raw) != strconv.Itoa(i) {
			t.Fatalf("%d=%q err=%v", i, raw, err)
		}
	}
}

func TestPipelineRejectsPrivateWorkspaceSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside.cfg")
	if err := os.WriteFile(outside, []byte("outside"), 0640); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "instances", "abc", "private", "cfg", "link.cfg")
	if err := os.MkdirAll(filepath.Dir(link), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := New(root).Apply(context.Background(), "abc", zipFile(t, map[string]string{"cfg/package.cfg": "new"}), "v1", Hot); err == nil {
		t.Fatal("symlink accepted")
	}
	if raw, err := os.ReadFile(outside); err != nil || string(raw) != "outside" {
		t.Fatalf("outside=%q err=%v", raw, err)
	}
}

func TestPipelineRejectsGameTargetSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside.cfg")
	if err := os.WriteFile(outside, []byte("outside"), 0640); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "instances", "abc", "game", "left4dead2", "cfg", "plugin.cfg")
	if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, target); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := New(root).Apply(context.Background(), "abc", zipFile(t, map[string]string{"cfg/plugin.cfg": "new"}), "v1", Hot); err == nil {
		t.Fatal("symlink accepted")
	}
	if raw, err := os.ReadFile(outside); err != nil || string(raw) != "outside" {
		t.Fatalf("outside=%q err=%v", raw, err)
	}
}

func TestPipelineRejectsMalformedPrivateSnapshotName(t *testing.T) {
	root := t.TempDir()
	bad := filepath.Join(root, "instances", "abc", "backups", "private", "snapshots", "not-a-snapshot")
	if err := os.MkdirAll(bad, 0750); err != nil {
		t.Fatal(err)
	}
	if err := New(root).Apply(context.Background(), "abc", zipFile(t, map[string]string{"cfg/a.cfg": "new"}), "v1", Hot); err == nil {
		t.Fatal("malformed snapshot accepted")
	}
	target := filepath.Join(root, "instances", "abc", "game", "left4dead2", "cfg", "a.cfg")
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("game mutated: %v", err)
	}
}
func TestUpdateStripsReleaseWrapperDirectory(t *testing.T) {
	root := t.TempDir()
	archive := zipFile(t, map[string]string{"release/cfg/plugin.cfg": "new"})
	if err := New(root).Apply(context.Background(), "abc", archive, "v1", Hot); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "instances", "abc", "game", "left4dead2", "cfg", "plugin.cfg")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "instances", "abc", "game", "left4dead2", "release")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("wrapper directory exists: %v", err)
	}
}
func TestHotUpdateRejectsBinaryOutsideAllowlist(t *testing.T) {
	pipeline := New(t.TempDir())
	if err := pipeline.Apply(context.Background(), "abc", zipFile(t, map[string]string{"bin/server.so": "x"}), "v1", Hot); err == nil {
		t.Fatal("unsafe hot update accepted")
	}
}
func TestHotUpdateFiltersNonHotFilesFromMixedRelease(t *testing.T) {
	root := t.TempDir()
	archive := zipFile(t, map[string]string{
		"release/cfg/plugin.cfg": "new",
		"release/bin/server.so":  "binary",
		"release/README.md":      "docs",
	})
	if err := New(root).Apply(context.Background(), "abc", archive, "v2", Hot); err != nil {
		t.Fatal(err)
	}
	game := filepath.Join(root, "instances", "abc", "game", "left4dead2")
	if raw, err := os.ReadFile(filepath.Join(game, "cfg", "plugin.cfg")); err != nil || string(raw) != "new" {
		t.Fatalf("hot file=%q err=%v", raw, err)
	}
	if _, err := os.Stat(filepath.Join(game, "bin", "server.so")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("binary was applied: %v", err)
	}
}
func TestFailureRollsBackReplacedFiles(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "instances", "abc", "game", "left4dead2", "cfg")
	_ = os.MkdirAll(target, 0750)
	_ = os.WriteFile(filepath.Join(target, "plugin.cfg"), []byte("old"), 0640)
	pipeline := New(root)
	pipeline.AfterDeploy = func() error { return errors.New("injected failure") }
	err := pipeline.Apply(context.Background(), "abc", zipFile(t, map[string]string{"cfg/plugin.cfg": "new"}), "v2", Full)
	if err == nil {
		t.Fatal("expected failure")
	}
	raw, _ := os.ReadFile(filepath.Join(target, "plugin.cfg"))
	if string(raw) != "old" {
		t.Fatalf("rollback got %q", raw)
	}
}

func TestRecoverRollsBackUncommittedDeployment(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "instances", "abc")
	target := filepath.Join(base, "game", "left4dead2", "cfg")
	if err := os.MkdirAll(target, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "plugin.cfg"), []byte("old"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(filepath.Join(base, "package-manifest.json"), manifest{Version: "v1", Files: map[string]string{"cfg/plugin.cfg": "old-hash"}}); err != nil {
		t.Fatal(err)
	}

	deployment, err := New(root).Begin(context.Background(), "abc", zipFile(t, map[string]string{"cfg/plugin.cfg": "new"}), "v2", Full)
	if err != nil {
		t.Fatal(err)
	}
	if deployment == nil {
		t.Fatal("missing deployment transaction")
	}
	raw, _ := os.ReadFile(filepath.Join(target, "plugin.cfg"))
	if string(raw) != "new" {
		t.Fatalf("deployed=%q", raw)
	}
	if journals, _ := filepath.Glob(filepath.Join(base, "backups", "update-*", "journal.json")); len(journals) != 1 {
		t.Fatalf("journals=%v", journals)
	}

	if err := New(root).Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(filepath.Join(target, "plugin.cfg"))
	if string(raw) != "old" {
		t.Fatalf("recovered=%q", raw)
	}
	if got := readManifest(filepath.Join(base, "package-manifest.json")); got.Version != "v1" {
		t.Fatalf("manifest=%#v", got)
	}
	if journals, _ := filepath.Glob(filepath.Join(base, "backups", "update-*", "journal.json")); len(journals) != 0 {
		t.Fatalf("stale journals=%v", journals)
	}
}

func TestPipelineRecoverRollsBackInterruptedPrivateApply(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "instances", "abc")
	target := filepath.Join(base, "game", "left4dead2", "cfg", "a.cfg")
	work := filepath.Join(base, "backups", "private", "apply-33333333-3333-3333-3333-333333333333")
	if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("mutated"), 0640); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(work, "game.before", "cfg", "a.cfg")
	if err := os.MkdirAll(filepath.Dir(backup), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup, []byte("old"), 0640); err != nil {
		t.Fatal(err)
	}
	journal := []byte(`{"version":1,"instance_id":"abc","stage":"applying","manifest_existed":false,"lower_existed":false,"affected":[{"path":"cfg/a.cfg","kind":"file","existed":true}]}`)
	if err := os.WriteFile(filepath.Join(work, "journal.json"), journal, 0640); err != nil {
		t.Fatal(err)
	}
	if err := New(root).Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(target); err != nil || string(raw) != "old" {
		t.Fatalf("game=%q err=%v", raw, err)
	}
}

func TestRecoverRejectsCorruptUpdateJournalsWithoutSideEffects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*updateJournal)
	}{{"stage", func(j *updateJournal) { j.Stage = "evil" }}, {"mode", func(j *updateJournal) { j.Mode = "evil" }}, {"kind", func(j *updateJournal) { j.Affected = []journalEntry{{Path: "cfg/a.cfg", Existed: true, Kind: "link"}} }}, {"path", func(j *updateJournal) { j.Affected = []journalEntry{{Path: "../outside", Existed: false}} }}, {"snapshot", func(j *updateJournal) { j.PrivateSnapshots = []string{"../../outside"} }}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			base := filepath.Join(root, "instances", "abc")
			work := filepath.Join(base, "backups", "update-55555555-5555-5555-5555-555555555555")
			sentinel := filepath.Join(root, "outside")
			if err := os.MkdirAll(work, 0750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(sentinel, []byte("safe"), 0640); err != nil {
				t.Fatal(err)
			}
			value := updateJournal{Version: 1, InstanceID: "abc", Mode: Hot, Stage: "committed", BackupRoot: "replaced"}
			tc.mutate(&value)
			if err := writeJournal(filepath.Join(work, "journal.json"), value); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(filepath.Join(work, "journal.json"))
			if err := New(root).Recover(context.Background()); err == nil {
				t.Fatal("corrupt journal accepted")
			}
			after, err := os.ReadFile(filepath.Join(work, "journal.json"))
			if err != nil || string(after) != string(before) {
				t.Fatalf("journal changed: %q err=%v", after, err)
			}
			if raw, err := os.ReadFile(sentinel); err != nil || string(raw) != "safe" {
				t.Fatalf("outside=%q err=%v", raw, err)
			}
		})
	}
}

func TestRecoverRejectsSymlinkedUpdateJournal(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "instances", "abc", "backups", "update-66666666-6666-6666-6666-666666666666")
	if err := os.MkdirAll(work, 0750); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside.json")
	value := updateJournal{Version: 1, InstanceID: "abc", Mode: Hot, Stage: "committed", BackupRoot: "replaced"}
	if err := writeJournal(outside, value); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(work, "journal.json")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	before, _ := os.ReadFile(outside)
	if err := New(root).Recover(context.Background()); err == nil {
		t.Fatal("symlink journal accepted")
	}
	after, err := os.ReadFile(outside)
	if err != nil || string(after) != string(before) {
		t.Fatalf("outside changed: %q err=%v", after, err)
	}
}

func TestRecoverRejectsSymlinkedUpdateAncestors(t *testing.T) {
	for _, ancestor := range []string{"instance", "backups"} {
		t.Run(ancestor, func(t *testing.T) {
			root := t.TempDir()
			outside := filepath.Join(root, "outside")
			outsideBackups := filepath.Join(outside, "backups")
			work := filepath.Join(outsideBackups, "update-88888888-8888-8888-8888-888888888888")
			if err := os.MkdirAll(work, 0750); err != nil {
				t.Fatal(err)
			}
			value := updateJournal{Version: 1, InstanceID: "abc", Mode: Hot, Stage: "committed", BackupRoot: "replaced"}
			journal := filepath.Join(work, "journal.json")
			if err := writeJournal(journal, value); err != nil {
				t.Fatal(err)
			}
			sentinel := filepath.Join(outside, "sentinel")
			if err := os.WriteFile(sentinel, []byte("safe"), 0640); err != nil {
				t.Fatal(err)
			}
			if ancestor == "instance" {
				if err := os.MkdirAll(filepath.Join(root, "instances"), 0750); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(root, "instances", "abc")); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			} else {
				instance := filepath.Join(root, "instances", "abc")
				if err := os.MkdirAll(instance, 0750); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outsideBackups, filepath.Join(instance, "backups")); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			}
			before, _ := os.ReadFile(journal)
			if err := New(root).Recover(context.Background()); err == nil {
				t.Fatal("symlink ancestor accepted")
			}
			after, err := os.ReadFile(journal)
			if err != nil || string(after) != string(before) {
				t.Fatalf("journal changed: %q err=%v", after, err)
			}
			if raw, err := os.ReadFile(sentinel); err != nil || string(raw) != "safe" {
				t.Fatalf("outside=%q err=%v", raw, err)
			}
		})
	}
}

func TestRollbackRejectsSymlinkedUpdateAncestor(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	work := filepath.Join(outside, "backups", "update-99999999-9999-9999-9999-999999999999")
	if err := os.MkdirAll(work, 0750); err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join(work, "journal.json")
	value := updateJournal{Version: 1, InstanceID: "abc", Mode: Hot, Stage: "deployed", BackupRoot: "replaced"}
	if err := writeJournal(journal, value); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "instances"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "instances", "abc")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	before, _ := os.ReadFile(journal)
	d := &deployment{pipeline: New(root), journalPath: filepath.Join(root, "instances", "abc", "backups", filepath.Base(work), "journal.json"), journal: value}
	if err := d.Rollback(); err == nil {
		t.Fatal("symlink ancestor accepted")
	}
	after, err := os.ReadFile(journal)
	if err != nil || string(after) != string(before) {
		t.Fatalf("journal changed: %q err=%v", after, err)
	}
}

func TestValidateUpdateJournalRejectsTraversalWorkPath(t *testing.T) {
	root := t.TempDir()
	value := updateJournal{Version: 1, InstanceID: "abc", Mode: Hot, Stage: "committed", BackupRoot: "replaced"}
	journal := filepath.Join(root, "outside", "update-77777777-7777-7777-7777-777777777777", "journal.json")
	if err := validateUpdateJournalPathAndValue(root, journal, value); err == nil {
		t.Fatal("outside work accepted")
	}
}
func TestPipelineFailureRollbackRestoresControlledSharedVPKLink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "instances", "abc", "game", "left4dead2", "addons", "shared.vpk")
	if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
		t.Fatal(err)
	}
	linkTarget := "/opt/l4d2/shared-vpk/shared.vpk"
	if err := os.Symlink(linkTarget, target); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	p := New(root)
	p.AfterDeploy = func() error { return errors.New("injected") }
	if err := p.Apply(context.Background(), "abc", zipFile(t, map[string]string{"addons/shared.vpk": "package"}), "v1", Hot); err == nil {
		t.Fatal("expected failure")
	}
	if got, err := os.Readlink(target); err != nil || filepath.ToSlash(got) != linkTarget {
		t.Fatalf("link=%q err=%v", got, err)
	}
}

func TestPipelineUpdateWithPrivateDirectoryAndControlledSiblingLink(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	m := content.NewPrivateManager(root, 1<<20)
	if _, err := m.Save(ctx, "abc", "addons/sub/file.cfg", []byte("private")); err != nil {
		t.Fatal(err)
	}
	if err := m.ApplyChanges(ctx, "abc"); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "instances", "abc", "game", "left4dead2", "addons", "a.vpk")
	if err := os.Symlink("/opt/l4d2/shared-vpk/a.vpk", link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := New(root).Apply(ctx, "abc", zipFile(t, map[string]string{"cfg/package.cfg": "new"}), "v1", Hot); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(filepath.Join(filepath.Dir(link), "sub", "file.cfg")); err != nil || string(raw) != "private" {
		t.Fatalf("private=%q err=%v", raw, err)
	}
	if got, err := os.Readlink(link); err != nil || filepath.ToSlash(got) != "/opt/l4d2/shared-vpk/a.vpk" {
		t.Fatalf("link=%q err=%v", got, err)
	}
}

func TestPipelineRollbackWithPrivateDirectoryRestoresControlledSiblingLink(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	m := content.NewPrivateManager(root, 1<<20)
	if _, err := m.Save(ctx, "abc", "addons/sub/file.cfg", []byte("private")); err != nil {
		t.Fatal(err)
	}
	if err := m.ApplyChanges(ctx, "abc"); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "instances", "abc", "game", "left4dead2", "addons", "a.vpk")
	if err := os.Symlink("/opt/l4d2/shared-vpk/a.vpk", link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	p := New(root)
	p.AfterDeploy = func() error { return errors.New("injected") }
	if err := p.Apply(ctx, "abc", zipFile(t, map[string]string{"cfg/package.cfg": "new"}), "v1", Hot); err == nil {
		t.Fatal("expected failure")
	}
	if raw, err := os.ReadFile(filepath.Join(filepath.Dir(link), "sub", "file.cfg")); err != nil || string(raw) != "private" {
		t.Fatalf("private=%q err=%v", raw, err)
	}
	if got, err := os.Readlink(link); err != nil || filepath.ToSlash(got) != "/opt/l4d2/shared-vpk/a.vpk" {
		t.Fatalf("link=%q err=%v", got, err)
	}
}

func TestPipelinePrivateDirectoryStillRejectsArbitrarySiblingLink(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	m := content.NewPrivateManager(root, 1<<20)
	if _, err := m.Save(ctx, "abc", "addons/sub/file.cfg", []byte("private")); err != nil {
		t.Fatal(err)
	}
	if err := m.ApplyChanges(ctx, "abc"); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.WriteFile(outside, []byte("safe"), 0640); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "instances", "abc", "game", "left4dead2", "addons", "bad.vpk")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := New(root).Apply(ctx, "abc", zipFile(t, map[string]string{"cfg/package.cfg": "new"}), "v1", Hot); err == nil {
		t.Fatal("arbitrary symlink accepted")
	}
	if raw, err := os.ReadFile(outside); err != nil || string(raw) != "safe" {
		t.Fatalf("outside=%q err=%v", raw, err)
	}
}

func zipFile(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "package.zip")
	file, _ := os.Create(path)
	writer := zip.NewWriter(file)
	for name, value := range files {
		entry, _ := writer.Create(name)
		_, _ = entry.Write([]byte(value))
	}
	_ = writer.Close()
	_ = file.Close()
	return path
}

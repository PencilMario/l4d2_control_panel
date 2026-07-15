package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

func openLegacyInstanceDatabase(t *testing.T, path, packageID string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE instances (
 id TEXT PRIMARY KEY, node_id TEXT NOT NULL DEFAULT 'local', name TEXT NOT NULL UNIQUE, container_id TEXT NOT NULL DEFAULT '',
 game_port INTEGER NOT NULL UNIQUE, sourcetv_port INTEGER NOT NULL DEFAULT 0,
 start_map TEXT NOT NULL, game_mode TEXT NOT NULL, tickrate INTEGER NOT NULL,
 max_players INTEGER NOT NULL, extra_args TEXT NOT NULL DEFAULT '', runtime_image TEXT NOT NULL,
 package_version TEXT NOT NULL DEFAULT '', desired_state TEXT NOT NULL, actual_state TEXT NOT NULL,
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL
)`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = db.Exec(`INSERT INTO instances(id,node_id,name,container_id,game_port,sourcetv_port,start_map,game_mode,tickrate,max_players,extra_args,runtime_image,package_version,desired_state,actual_state,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, "legacy", "local", "Legacy", "", 27015, 0, "map", "coop", 100, 8, "", "runtime", packageID, domain.StateStopped, domain.StateStopped, now, now)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db
}

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

func TestSelectedPackagePersistsIndependentlyFromAppliedPackage(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	value := domain.Instance{
		ID:                "instance-packages",
		NodeID:            "local",
		Name:              "Packages",
		GamePort:          27015,
		StartMap:          "c2m1_highway",
		GameMode:          "coop",
		Tickrate:          100,
		MaxPlayers:        8,
		RuntimeImage:      "runtime",
		SelectedPackageID: "selected-package",
		PackageVersion:    "applied-package",
		DesiredState:      domain.StateStopped,
		ActualState:       domain.StateStopped,
	}
	if err := s.CreateInstance(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	got, err := s.Instance(context.Background(), value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SelectedPackageID != "selected-package" || got.PackageVersion != "applied-package" {
		t.Fatalf("instance=%#v", got)
	}
}

func TestMigrationBackfillsSelectedPackageFromAppliedPackage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	legacy := openLegacyInstanceDatabase(t, path, "package-a")
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.Instance(context.Background(), "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if got.SelectedPackageID != "package-a" || got.PackageVersion != "package-a" {
		t.Fatalf("instance=%#v", got)
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

func TestGitHubSourcesPersistAndDefaultIsSeeded(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	sources, err := s.GitHubSources(ctx)
	if err != nil || len(sources) != 2 {
		t.Fatalf("defaults=%#v err=%v", sources, err)
	}
	foundCompetitive := false
	for _, source := range sources {
		foundCompetitive = foundCompetitive || source.Repository == "PencilMario/L4D2-Competitive-Rework"
	}
	if !foundCompetitive {
		t.Fatalf("missing Competitive Rework source: %#v", sources)
	}
	source := domain.GitHubSource{ID: "custom", Name: "Custom", Repository: "owner/repo", AssetPattern: `^plugins\.zip$`}
	if err := s.SaveGitHubSource(ctx, source); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.GitHubSource(ctx, source.ID)
	if err != nil || loaded.Name != "Custom" {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
	if err := s.DeleteGitHubSource(ctx, source.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GitHubSource(ctx, source.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestJobHistoryMigrationBackfillsLegacySnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	created := time.Date(2026, 7, 16, 1, 0, 0, 0, time.UTC)
	updated := created.Add(2 * time.Minute)
	createLegacyJobsDatabase(t, path, domain.JobRecord{
		ID:        "legacy-failed",
		Status:    "failed",
		Stage:     "steamcmd",
		Error:     "legacy failure",
		CreatedAt: created,
		UpdatedAt: updated,
	})

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	job, found, err := s.LoadJob("legacy-failed")
	if err != nil || !found {
		t.Fatalf("job=%#v found=%v err=%v", job, found, err)
	}
	if job.StartedAt == nil || !job.StartedAt.Equal(created) || job.FinishedAt == nil || !job.FinishedAt.Equal(updated) {
		t.Fatalf("job times=%#v", job)
	}
	events, err := s.JobEvents(job.ID)
	if err != nil || len(events) != 1 || events[0].Kind != "snapshot" ||
		!strings.Contains(events[0].Message, "legacy failure") ||
		!strings.Contains(events[0].Message, "升级前任务") ||
		!strings.Contains(events[0].Message, "执行时间为估算值") {
		t.Fatalf("events=%#v err=%v", events, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	events, err = reopened.JobEvents(job.ID)
	if err != nil || len(events) != 1 {
		t.Fatalf("reopened events=%#v err=%v", events, err)
	}
}

func TestSaveJobWithEventPersistsSnapshotAndEvent(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	created := time.Date(2026, 7, 16, 2, 0, 0, 0, time.UTC)
	started := created.Add(3 * time.Second)
	record := domain.JobRecord{
		ID:         "job-1",
		InstanceID: "instance-1",
		Type:       "game_update",
		Status:     "running",
		Stage:      "steamcmd",
		Percent:    25,
		Message:    "downloading",
		CreatedAt:  created,
		UpdatedAt:  started,
		StartedAt:  &started,
	}
	event := domain.JobEvent{
		JobID:     record.ID,
		Kind:      "progress",
		Stage:     record.Stage,
		Percent:   record.Percent,
		Message:   record.Message,
		CreatedAt: started,
	}
	if err := s.SaveJobWithEvent(record, event); err != nil {
		t.Fatal(err)
	}
	loaded, found, err := s.LoadJob(record.ID)
	if err != nil || !found || loaded.StartedAt == nil || !loaded.StartedAt.Equal(started) || loaded.Message != "downloading" {
		t.Fatalf("loaded=%#v found=%v err=%v", loaded, found, err)
	}
	events, err := s.JobEvents(record.ID)
	if err != nil || len(events) != 1 || events[0].Kind != "progress" || events[0].ID == 0 {
		t.Fatalf("events=%#v err=%v", events, err)
	}
}

func TestSuccessfulJobLimitPrunesOnlyOldestSucceeded(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	limit, err := s.SuccessfulJobLimit()
	if err != nil || limit != 25 {
		t.Fatalf("default limit=%d err=%v", limit, err)
	}

	base := time.Date(2026, 7, 16, 3, 0, 0, 0, time.UTC)
	for index, id := range []string{"oldest-succeeded", "middle-succeeded", "newest-succeeded"} {
		finished := base.Add(time.Duration(index) * time.Minute)
		record := domain.JobRecord{
			ID: id, Type: "fixture", Status: "succeeded", Percent: 100,
			CreatedAt: base, UpdatedAt: finished, StartedAt: &base, FinishedAt: &finished,
		}
		if err := s.SaveJobWithEvent(record, domain.JobEvent{Kind: "succeeded", CreatedAt: finished}); err != nil {
			t.Fatal(err)
		}
	}
	failedAt := base.Add(4 * time.Minute)
	if err := s.SaveJobWithEvent(domain.JobRecord{
		ID: "failed", Type: "fixture", Status: "failed", Error: "boom",
		CreatedAt: base, UpdatedAt: failedAt, StartedAt: &base, FinishedAt: &failedAt,
	}, domain.JobEvent{Kind: "failed", Message: "boom", CreatedAt: failedAt}); err != nil {
		t.Fatal(err)
	}

	if err := s.SetSuccessfulJobLimit(2); err != nil {
		t.Fatal(err)
	}
	limit, err = s.SuccessfulJobLimit()
	if err != nil || limit != 2 {
		t.Fatalf("saved limit=%d err=%v", limit, err)
	}
	assertStoredJob(t, s, "oldest-succeeded", false)
	assertStoredJob(t, s, "middle-succeeded", true)
	assertStoredJob(t, s, "newest-succeeded", true)
	assertStoredJob(t, s, "failed", true)
	events, err := s.JobEvents("oldest-succeeded")
	if err != nil || len(events) != 0 {
		t.Fatalf("deleted job events=%#v err=%v", events, err)
	}
}

func TestSuccessfulJobLimitUsesStableIDOrderForEqualFinishTimes(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	finished := time.Date(2026, 7, 16, 3, 30, 0, 0, time.UTC)
	for _, id := range []string{"success-a", "success-b", "success-c"} {
		if err := s.SaveJobWithEvent(domain.JobRecord{
			ID: id, Status: "succeeded", CreatedAt: finished, UpdatedAt: finished, FinishedAt: &finished,
		}, domain.JobEvent{Kind: "succeeded", CreatedAt: finished}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetSuccessfulJobLimit(2); err != nil {
		t.Fatal(err)
	}
	assertStoredJob(t, s, "success-a", false)
	assertStoredJob(t, s, "success-b", true)
	assertStoredJob(t, s, "success-c", true)
}

func TestSuccessfulJobLimitRollsBackSettingWhenPruneFails(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	base := time.Date(2026, 7, 16, 3, 40, 0, 0, time.UTC)
	for index, id := range []string{"older-success", "newer-success"} {
		finished := base.Add(time.Duration(index) * time.Minute)
		if err := s.SaveJob(domain.JobRecord{
			ID: id, Status: "succeeded", CreatedAt: base, UpdatedAt: finished, FinishedAt: &finished,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec(`CREATE TRIGGER reject_success_delete
BEFORE DELETE ON jobs WHEN OLD.status='succeeded'
BEGIN SELECT RAISE(ABORT,'delete blocked'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSuccessfulJobLimit(1); err == nil {
		t.Fatal("expected pruning failure")
	}
	limit, err := s.SuccessfulJobLimit()
	if err != nil || limit != DefaultSuccessfulJobLimit {
		t.Fatalf("limit=%d err=%v", limit, err)
	}
	assertStoredJob(t, s, "older-success", true)
	assertStoredJob(t, s, "newer-success", true)
}

func TestRecoverJobsRecordsInterruptedEventAndFinishTime(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	created := time.Date(2026, 7, 16, 4, 0, 0, 0, time.UTC)
	if err := s.SaveJob(domain.JobRecord{
		ID: "stale-running", Type: "game_update", Status: "running",
		CreatedAt: created, UpdatedAt: created,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecoverJobs(); err != nil {
		t.Fatal(err)
	}
	job, found, err := s.LoadJob("stale-running")
	if err != nil || !found || job.Status != "interrupted" || job.FinishedAt == nil || job.Error == "" {
		t.Fatalf("job=%#v found=%v err=%v", job, found, err)
	}
	events, err := s.JobEvents(job.ID)
	if err != nil || len(events) != 1 || events[0].Kind != "interrupted" || events[0].Message == "" {
		t.Fatalf("events=%#v err=%v", events, err)
	}
}

func TestPruneSuccessfulJobsUsesSavedLimit(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.SetSuccessfulJobLimit(1); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 7, 16, 5, 0, 0, 0, time.UTC)
	for index, id := range []string{"older-success", "newer-success"} {
		finished := base.Add(time.Duration(index) * time.Minute)
		if err := s.SaveJobWithEvent(domain.JobRecord{
			ID: id, Status: "succeeded", CreatedAt: base, UpdatedAt: finished, FinishedAt: &finished,
		}, domain.JobEvent{Kind: "succeeded", CreatedAt: finished}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.PruneSuccessfulJobs(); err != nil {
		t.Fatal(err)
	}
	assertStoredJob(t, s, "older-success", false)
	assertStoredJob(t, s, "newer-success", true)
}

func TestSuccessfulJobLimitRejectsOutOfRangeValues(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, value := range []int{0, 501} {
		if err := s.SetSuccessfulJobLimit(value); err == nil {
			t.Fatalf("limit %d was accepted", value)
		}
	}
	limit, err := s.SuccessfulJobLimit()
	if err != nil || limit != DefaultSuccessfulJobLimit {
		t.Fatalf("limit=%d err=%v", limit, err)
	}
}

func TestJobsIncludesExecutionTimes(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	created := time.Date(2026, 7, 16, 6, 0, 0, 0, time.UTC)
	started := created.Add(2 * time.Second)
	finished := started.Add(30 * time.Second)
	if err := s.SaveJob(domain.JobRecord{
		ID: "timed-job", Status: "succeeded", CreatedAt: created, UpdatedAt: finished,
		StartedAt: &started, FinishedAt: &finished,
	}); err != nil {
		t.Fatal(err)
	}
	items, err := s.Jobs(context.Background(), 10)
	if err != nil || len(items) != 1 || items[0].StartedAt == nil || items[0].FinishedAt == nil {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	if !items[0].StartedAt.Equal(started) || !items[0].FinishedAt.Equal(finished) {
		t.Fatalf("times=%#v", items[0])
	}
}

func assertStoredJob(t *testing.T, store *Store, id string, want bool) {
	t.Helper()
	_, found, err := store.LoadJob(id)
	if err != nil || found != want {
		t.Fatalf("job %s found=%v want=%v err=%v", id, found, want, err)
	}
}

func createLegacyJobsDatabase(t *testing.T, path string, record domain.JobRecord) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
CREATE TABLE jobs (
 id TEXT PRIMARY KEY, instance_id TEXT NOT NULL, type TEXT NOT NULL, status TEXT NOT NULL,
 stage TEXT NOT NULL DEFAULT '', percent INTEGER NOT NULL DEFAULT 0, message TEXT NOT NULL DEFAULT '',
 error TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL
)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO jobs(id,instance_id,type,status,stage,percent,message,error,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		record.ID, record.InstanceID, record.Type, record.Status, record.Stage, record.Percent, record.Message, record.Error,
		record.CreatedAt.Format(time.RFC3339Nano), record.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
}

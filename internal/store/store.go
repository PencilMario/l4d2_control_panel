package store

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db            *sql.DB
	prunedHookMu  sync.RWMutex
	prunedJobHook func([]string)
}

func (s *Store) SetPrunedJobHook(hook func([]string)) {
	s.prunedHookMu.Lock()
	s.prunedJobHook = hook
	s.prunedHookMu.Unlock()
}

func (s *Store) notifyPrunedJobs(ids []string) {
	if len(ids) == 0 {
		return
	}
	s.prunedHookMu.RLock()
	hook := s.prunedJobHook
	s.prunedHookMu.RUnlock()
	if hook != nil {
		hook(append([]string(nil), ids...))
	}
}

func (s *Store) JobIDs(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM jobs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;" + initialSchema); err != nil {
		db.Close()
		return nil, err
	}
	if err = migrateSelectedPackage(db); err != nil {
		db.Close()
		return nil, err
	}
	if err = migrateGitHubSources(db); err != nil {
		db.Close()
		return nil, err
	}
	if err = migrateCompetitiveSource(db); err != nil {
		db.Close()
		return nil, err
	}
	if err = migrateJobHistory(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func migrateGitHubSources(db *sql.DB) error {
	var applied int
	if err := db.QueryRow(`SELECT count(*) FROM schema_migrations WHERE version=4`).Scan(&applied); err != nil {
		return err
	}
	if applied > 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`INSERT INTO github_sources(id,name,repository,asset_pattern,created_at,updated_at) SELECT 'default-not0721here','Not0721Here Coop 插件包','PencilMario/L4D2-Not0721Here-CoopSvPlugins','^L4D2-Not0721Here-CoopSvPlugins-compiled\.zip$',?,? WHERE NOT EXISTS (SELECT 1 FROM github_sources)`, now, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(4,?)`, now); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateCompetitiveSource(db *sql.DB) error {
	var applied int
	if err := db.QueryRow(`SELECT count(*) FROM schema_migrations WHERE version=5`).Scan(&applied); err != nil {
		return err
	}
	if applied > 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`INSERT OR IGNORE INTO github_sources(id,name,repository,asset_pattern,created_at,updated_at) VALUES('default-competitive-rework','L4D2 Competitive Rework','PencilMario/L4D2-Competitive-Rework','^L4D2-Competitive-Rework-compiled\.zip$',?,?)`, now, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(5,?)`, now); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateSelectedPackage(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(instances)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == "selected_package_id" {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if !found {
		if _, err := tx.Exec(`ALTER TABLE instances ADD COLUMN selected_package_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE instances SET selected_package_id=package_version WHERE selected_package_id=''`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (3, CURRENT_TIMESTAMP)`); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) DB() *sql.DB  { return s.db }
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) CreateInstance(ctx context.Context, v domain.Instance) error {
	now := time.Now().UTC()
	v.CreatedAt, v.UpdatedAt = now, now
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO instances(id,node_id,name,container_id,game_port,sourcetv_port,start_map,game_mode,tickrate,max_players,extra_args,runtime_image,package_version,selected_package_id,desired_state,actual_state,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, fields(v)...); err != nil {
		return err
	}
	if err = replacePluginPorts(ctx, tx, v.ID, v.PluginPorts); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) UpdateInstance(ctx context.Context, v domain.Instance) error {
	v.UpdatedAt = time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, err := tx.ExecContext(ctx, `UPDATE instances SET node_id=?,name=?,container_id=?,game_port=?,sourcetv_port=?,start_map=?,game_mode=?,tickrate=?,max_players=?,extra_args=?,runtime_image=?,package_version=?,selected_package_id=?,desired_state=?,actual_state=?,updated_at=? WHERE id=?`, v.NodeID, v.Name, v.ContainerID, v.GamePort, v.SourceTVPort, v.StartMap, v.GameMode, v.Tickrate, v.MaxPlayers, v.ExtraArgs, v.RuntimeImage, v.PackageVersion, v.SelectedPackageID, v.DesiredState, v.ActualState, v.UpdatedAt.Format(time.RFC3339Nano), v.ID)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if err = replacePluginPorts(ctx, tx, v.ID, v.PluginPorts); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) DeleteInstance(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM instances WHERE id=?", id)
	return err
}
func (s *Store) Instance(ctx context.Context, id string) (domain.Instance, error) {
	v, err := scanInstance(s.db.QueryRowContext(ctx, selectInstance+" WHERE id=?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	v.PluginPorts, err = s.pluginPorts(ctx, id)
	return v, err
}
func (s *Store) Instances(ctx context.Context) ([]domain.Instance, error) {
	rows, err := s.db.QueryContext(ctx, selectInstance+" ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Instance
	for rows.Next() {
		v, e := scanInstance(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	pluginPorts, err := s.allPluginPorts(ctx)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].PluginPorts = pluginPorts[out[i].ID]
		if out[i].PluginPorts == nil {
			out[i].PluginPorts = []int{}
		}
	}
	return out, nil
}

func replacePluginPorts(ctx context.Context, tx *sql.Tx, instanceID string, ports []int) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM instance_plugin_ports WHERE instance_id=?`, instanceID); err != nil {
		return err
	}
	for _, port := range ports {
		if _, err := tx.ExecContext(ctx, `INSERT INTO instance_plugin_ports(instance_id,port) VALUES(?,?)`, instanceID, port); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) pluginPorts(ctx context.Context, instanceID string) ([]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT port FROM instance_plugin_ports WHERE instance_id=? ORDER BY port`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ports := []int{}
	for rows.Next() {
		var port int
		if err := rows.Scan(&port); err != nil {
			return nil, err
		}
		ports = append(ports, port)
	}
	return ports, rows.Err()
}

func (s *Store) allPluginPorts(ctx context.Context) (map[string][]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT instance_id,port FROM instance_plugin_ports ORDER BY instance_id,port`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ports := make(map[string][]int)
	for rows.Next() {
		var instanceID string
		var port int
		if err := rows.Scan(&instanceID, &port); err != nil {
			return nil, err
		}
		ports[instanceID] = append(ports[instanceID], port)
	}
	return ports, rows.Err()
}

const selectInstance = `SELECT id,node_id,name,container_id,game_port,sourcetv_port,start_map,game_mode,tickrate,max_players,extra_args,runtime_image,package_version,selected_package_id,desired_state,actual_state,created_at,updated_at FROM instances`

type scanner interface{ Scan(...any) error }

func scanInstance(s scanner) (domain.Instance, error) {
	var v domain.Instance
	var c, u string
	err := s.Scan(&v.ID, &v.NodeID, &v.Name, &v.ContainerID, &v.GamePort, &v.SourceTVPort, &v.StartMap, &v.GameMode, &v.Tickrate, &v.MaxPlayers, &v.ExtraArgs, &v.RuntimeImage, &v.PackageVersion, &v.SelectedPackageID, &v.DesiredState, &v.ActualState, &c, &u)
	if err == nil {
		v.CreatedAt, _ = time.Parse(time.RFC3339Nano, c)
		v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, u)
	}
	return v, err
}
func fields(v domain.Instance) []any {
	return []any{v.ID, v.NodeID, v.Name, v.ContainerID, v.GamePort, v.SourceTVPort, v.StartMap, v.GameMode, v.Tickrate, v.MaxPlayers, v.ExtraArgs, v.RuntimeImage, v.PackageVersion, v.SelectedPackageID, v.DesiredState, v.ActualState, v.CreatedAt.Format(time.RFC3339Nano), v.UpdatedAt.Format(time.RFC3339Nano)}
}

func (s *Store) LoadCredential() (hash, salt []byte, found bool, err error) {
	err = s.db.QueryRow(`SELECT password_hash,salt FROM administrator WHERE singleton=1`).Scan(&hash, &salt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, false, nil
	}
	return hash, salt, err == nil, err
}

func (s *Store) SaveCredential(hash, salt []byte) error {
	_, err := s.db.Exec(`INSERT INTO administrator(singleton,password_hash,salt,updated_at) VALUES(1,?,?,?) ON CONFLICT(singleton) DO NOTHING`, hash, salt, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) SaveSession(tokenHash []byte, expires time.Time) error {
	_, err := s.db.Exec(`INSERT INTO sessions(token_hash,expires_at,created_at) VALUES(?,?,?)`, tokenHash, expires.UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) SessionValid(tokenHash []byte, now time.Time) (bool, error) {
	var expires string
	err := s.db.QueryRow(`SELECT expires_at FROM sessions WHERE token_hash=?`, tokenHash).Scan(&expires)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, expires)
	return err == nil && now.Before(parsed), err
}

func (s *Store) DeleteSession(tokenHash []byte) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash=?`, tokenHash)
	return err
}

func (s *Store) SaveJob(v domain.JobRecord) error {
	return saveJob(s.db, v)
}
func (s *Store) LoadJob(id string) (domain.JobRecord, bool, error) {
	var v domain.JobRecord
	var created, updated string
	var started, finished sql.NullString
	err := s.db.QueryRow(`SELECT id,instance_id,type,status,stage,percent,message,error,created_at,updated_at,started_at,finished_at FROM jobs WHERE id=?`, id).Scan(&v.ID, &v.InstanceID, &v.Type, &v.Status, &v.Stage, &v.Percent, &v.Message, &v.Error, &created, &updated, &started, &finished)
	if errors.Is(err, sql.ErrNoRows) {
		return v, false, nil
	}
	if err != nil {
		return v, false, err
	}
	v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	v.StartedAt = parseNullableTime(started)
	v.FinishedAt = parseNullableTime(finished)
	return v, true, nil
}
func (s *Store) RecoverJobs() error {
	_, err := s.RecoverJobsWithIDs()
	return err
}

func (s *Store) RecoverJobsWithIDs() ([]string, error) {
	const message = "Panel restarted while this job was active; inspect the managed container and retry or roll back"
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id,stage,percent FROM jobs WHERE status IN ('pending','running') ORDER BY id`)
	if err != nil {
		return nil, err
	}
	type staleJob struct {
		id, stage string
		percent   int
	}
	stale := []staleJob{}
	for rows.Next() {
		var job staleJob
		if err := rows.Scan(&job.id, &job.stage, &job.percent); err != nil {
			rows.Close()
			return nil, err
		}
		stale = append(stale, job)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	finished := time.Now().UTC().Format(time.RFC3339Nano)
	for _, job := range stale {
		if _, err := tx.Exec(`UPDATE jobs SET status='interrupted',error=?,updated_at=?,finished_at=? WHERE id=?`, message, finished, finished, job.id); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`INSERT INTO job_events(job_id,kind,stage,percent,message,created_at) VALUES(?,?,?,?,?,?)`, job.id, "interrupted", job.stage, job.percent, message, finished); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	ids := make([]string, len(stale))
	for index, job := range stale {
		ids[index] = job.id
	}
	return ids, nil
}
func (s *Store) Jobs(ctx context.Context, limit int) ([]domain.JobRecord, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,instance_id,type,status,stage,percent,message,error,created_at,updated_at,started_at,finished_at FROM jobs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.JobRecord{}
	for rows.Next() {
		var v domain.JobRecord
		var created, updated string
		var started, finished sql.NullString
		if err := rows.Scan(&v.ID, &v.InstanceID, &v.Type, &v.Status, &v.Stage, &v.Percent, &v.Message, &v.Error, &created, &updated, &started, &finished); err != nil {
			return nil, err
		}
		v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		v.StartedAt = parseNullableTime(started)
		v.FinishedAt = parseNullableTime(finished)
		result = append(result, v)
	}
	return result, rows.Err()
}

func (s *Store) UpsertVPKRestart(ctx context.Context, v domain.VPKRestart) error {
	now := time.Now().UTC()
	if v.CreatedAt.IsZero() {
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	if v.Status == "" {
		v.Status = "waiting"
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO shared_vpk_restarts(instance_id,container_id,publication_id,status,failures,created_at,updated_at)
VALUES(?,?,?,?,?,?,?) ON CONFLICT(instance_id) DO UPDATE SET publication_id=excluded.publication_id,updated_at=excluded.updated_at`,
		v.InstanceID, v.ContainerID, v.PublicationID, v.Status, v.Failures, v.CreatedAt.Format(time.RFC3339Nano), v.UpdatedAt.Format(time.RFC3339Nano))
	return err
}

func (s *Store) PendingVPKRestarts(ctx context.Context) ([]domain.VPKRestart, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT instance_id,container_id,publication_id,status,failures,created_at,updated_at FROM shared_vpk_restarts WHERE status IN ('waiting','queued','retry') ORDER BY created_at,instance_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.VPKRestart{}
	for rows.Next() {
		var v domain.VPKRestart
		var created, updated string
		if err := rows.Scan(&v.InstanceID, &v.ContainerID, &v.PublicationID, &v.Status, &v.Failures, &created, &updated); err != nil {
			return nil, err
		}
		v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		result = append(result, v)
	}
	return result, rows.Err()
}

func (s *Store) ClaimVPKRestart(ctx context.Context, instanceID string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE shared_vpk_restarts SET status='queued',updated_at=? WHERE instance_id=? AND status IN ('waiting','retry')`, time.Now().UTC().Format(time.RFC3339Nano), instanceID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (s *Store) UpdateVPKRestart(ctx context.Context, instanceID, status string, failures int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE shared_vpk_restarts SET status=?,failures=?,updated_at=? WHERE instance_id=?`, status, failures, time.Now().UTC().Format(time.RFC3339Nano), instanceID)
	return err
}
func (s *Store) RecordAudit(ctx context.Context, v domain.AuditRecord) error {
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit_events(id,action,target,result,metadata,created_at) VALUES(?,?,?,?,?,?)`, v.ID, v.Action, v.Target, v.Result, v.Metadata, v.CreatedAt.Format(time.RFC3339Nano))
	return err
}
func (s *Store) AuditEvents(ctx context.Context, limit int) ([]domain.AuditRecord, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,action,target,result,metadata,created_at FROM audit_events ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.AuditRecord{}
	for rows.Next() {
		var v domain.AuditRecord
		var created string
		if err := rows.Scan(&v.ID, &v.Action, &v.Target, &v.Result, &v.Metadata, &created); err != nil {
			return nil, err
		}
		v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		result = append(result, v)
	}
	return result, rows.Err()
}
func (s *Store) SaveScheduledTask(ctx context.Context, v domain.ScheduledTask) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO scheduled_tasks(id,instance_id,type,cron,timezone,online_policy,payload,enabled,last_run,next_run) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET instance_id=excluded.instance_id,type=excluded.type,cron=excluded.cron,timezone=excluded.timezone,online_policy=excluded.online_policy,payload=excluded.payload,enabled=excluded.enabled,last_run=excluded.last_run,next_run=excluded.next_run`, v.ID, v.InstanceID, v.Type, v.Cron, v.Timezone, v.OnlinePolicy, v.Payload, v.Enabled, formatOptionalTime(v.LastRun), formatOptionalTime(v.NextRun))
	return err
}
func (s *Store) ScheduledTasks(ctx context.Context) ([]domain.ScheduledTask, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,instance_id,type,cron,timezone,online_policy,payload,enabled,last_run,next_run FROM scheduled_tasks ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.ScheduledTask{}
	for rows.Next() {
		var v domain.ScheduledTask
		var last, next string
		if err := rows.Scan(&v.ID, &v.InstanceID, &v.Type, &v.Cron, &v.Timezone, &v.OnlinePolicy, &v.Payload, &v.Enabled, &last, &next); err != nil {
			return nil, err
		}
		v.LastRun = parseOptionalTime(last)
		v.NextRun = parseOptionalTime(next)
		result = append(result, v)
	}
	return result, rows.Err()
}
func (s *Store) DeleteScheduledTask(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM scheduled_tasks WHERE id=?`, id)
	return err
}
func formatOptionalTime(v time.Time) string {
	if v.IsZero() {
		return ""
	}
	return v.UTC().Format(time.RFC3339Nano)
}
func parseOptionalTime(v string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, v)
	return parsed
}
func (s *Store) SaveSecret(ctx context.Context, name string, ciphertext []byte) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO secrets(name,ciphertext,updated_at) VALUES(?,?,?) ON CONFLICT(name) DO UPDATE SET ciphertext=excluded.ciphertext,updated_at=excluded.updated_at`, name, ciphertext, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func (s *Store) LoadSecret(ctx context.Context, name string) ([]byte, bool, error) {
	var value []byte
	err := s.db.QueryRowContext(ctx, `SELECT ciphertext FROM secrets WHERE name=?`, name).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	return value, err == nil, err
}
func (s *Store) DeleteSecret(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM secrets WHERE name=?`, name)
	return err
}

func (s *Store) SaveGitHubSource(ctx context.Context, v domain.GitHubSource) error {
	now := time.Now().UTC()
	if v.CreatedAt.IsZero() {
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `INSERT INTO github_sources(id,name,repository,asset_pattern,created_at,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,repository=excluded.repository,asset_pattern=excluded.asset_pattern,updated_at=excluded.updated_at`, v.ID, v.Name, v.Repository, v.AssetPattern, v.CreatedAt.Format(time.RFC3339Nano), v.UpdatedAt.Format(time.RFC3339Nano))
	return err
}
func (s *Store) GitHubSource(ctx context.Context, id string) (domain.GitHubSource, error) {
	var v domain.GitHubSource
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id,name,repository,asset_pattern,created_at,updated_at FROM github_sources WHERE id=?`, id).Scan(&v.ID, &v.Name, &v.Repository, &v.AssetPattern, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return v, ErrNotFound
	}
	v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return v, err
}
func (s *Store) GitHubSources(ctx context.Context) ([]domain.GitHubSource, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,repository,asset_pattern,created_at,updated_at FROM github_sources ORDER BY name,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.GitHubSource{}
	for rows.Next() {
		var v domain.GitHubSource
		var created, updated string
		if err := rows.Scan(&v.ID, &v.Name, &v.Repository, &v.AssetPattern, &created, &updated); err != nil {
			return nil, err
		}
		v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		result = append(result, v)
	}
	return result, rows.Err()
}
func (s *Store) DeleteGitHubSource(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM github_sources WHERE id=?`, id)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrNotFound
	}
	return nil
}

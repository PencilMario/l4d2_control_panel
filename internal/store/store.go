package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type Store struct{ db *sql.DB }

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
	return &Store{db: db}, nil
}
func (s *Store) DB() *sql.DB  { return s.db }
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) CreateInstance(ctx context.Context, v domain.Instance) error {
	now := time.Now().UTC()
	v.CreatedAt, v.UpdatedAt = now, now
	_, err := s.db.ExecContext(ctx, `INSERT INTO instances(id,node_id,name,container_id,game_port,sourcetv_port,start_map,game_mode,tickrate,max_players,extra_args,runtime_image,package_version,desired_state,actual_state,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, fields(v)...)
	return err
}
func (s *Store) UpdateInstance(ctx context.Context, v domain.Instance) error {
	v.UpdatedAt = time.Now().UTC()
	r, err := s.db.ExecContext(ctx, `UPDATE instances SET node_id=?,name=?,container_id=?,game_port=?,sourcetv_port=?,start_map=?,game_mode=?,tickrate=?,max_players=?,extra_args=?,runtime_image=?,package_version=?,desired_state=?,actual_state=?,updated_at=? WHERE id=?`, v.NodeID, v.Name, v.ContainerID, v.GamePort, v.SourceTVPort, v.StartMap, v.GameMode, v.Tickrate, v.MaxPlayers, v.ExtraArgs, v.RuntimeImage, v.PackageVersion, v.DesiredState, v.ActualState, v.UpdatedAt.Format(time.RFC3339Nano), v.ID)
	if err == nil {
		if n, _ := r.RowsAffected(); n == 0 {
			return ErrNotFound
		}
	}
	return err
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
	return out, rows.Err()
}

const selectInstance = `SELECT id,node_id,name,container_id,game_port,sourcetv_port,start_map,game_mode,tickrate,max_players,extra_args,runtime_image,package_version,desired_state,actual_state,created_at,updated_at FROM instances`

type scanner interface{ Scan(...any) error }

func scanInstance(s scanner) (domain.Instance, error) {
	var v domain.Instance
	var c, u string
	err := s.Scan(&v.ID, &v.NodeID, &v.Name, &v.ContainerID, &v.GamePort, &v.SourceTVPort, &v.StartMap, &v.GameMode, &v.Tickrate, &v.MaxPlayers, &v.ExtraArgs, &v.RuntimeImage, &v.PackageVersion, &v.DesiredState, &v.ActualState, &c, &u)
	if err == nil {
		v.CreatedAt, _ = time.Parse(time.RFC3339Nano, c)
		v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, u)
	}
	return v, err
}
func fields(v domain.Instance) []any {
	return []any{v.ID, v.NodeID, v.Name, v.ContainerID, v.GamePort, v.SourceTVPort, v.StartMap, v.GameMode, v.Tickrate, v.MaxPlayers, v.ExtraArgs, v.RuntimeImage, v.PackageVersion, v.DesiredState, v.ActualState, v.CreatedAt.Format(time.RFC3339Nano), v.UpdatedAt.Format(time.RFC3339Nano)}
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
	_, err := s.db.Exec(`INSERT INTO jobs(id,instance_id,type,status,stage,percent,message,error,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET status=excluded.status,stage=excluded.stage,percent=excluded.percent,message=excluded.message,error=excluded.error,updated_at=excluded.updated_at`, v.ID, v.InstanceID, v.Type, v.Status, v.Stage, v.Percent, v.Message, v.Error, v.CreatedAt.Format(time.RFC3339Nano), v.UpdatedAt.Format(time.RFC3339Nano))
	return err
}
func (s *Store) LoadJob(id string) (domain.JobRecord, bool, error) {
	var v domain.JobRecord
	var created, updated string
	err := s.db.QueryRow(`SELECT id,instance_id,type,status,stage,percent,message,error,created_at,updated_at FROM jobs WHERE id=?`, id).Scan(&v.ID, &v.InstanceID, &v.Type, &v.Status, &v.Stage, &v.Percent, &v.Message, &v.Error, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return v, false, nil
	}
	if err != nil {
		return v, false, err
	}
	v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return v, true, nil
}
func (s *Store) RecoverJobs() error {
	_, err := s.db.Exec(`UPDATE jobs SET status='interrupted',error='Panel restarted while this job was active; inspect the managed container and retry or roll back',updated_at=? WHERE status IN ('pending','running')`, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func (s *Store) Jobs(ctx context.Context, limit int) ([]domain.JobRecord, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,instance_id,type,status,stage,percent,message,error,created_at,updated_at FROM jobs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.JobRecord{}
	for rows.Next() {
		var v domain.JobRecord
		var created, updated string
		if err := rows.Scan(&v.ID, &v.InstanceID, &v.Type, &v.Status, &v.Stage, &v.Percent, &v.Message, &v.Error, &created, &updated); err != nil {
			return nil, err
		}
		v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		result = append(result, v)
	}
	return result, rows.Err()
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

package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

const (
	jobHistoryMigrationVersion = 6

	DefaultCompletedJobLimit    = 25
	MinCompletedJobLimit        = 1
	MaxCompletedJobLimit        = 500
	DefaultGameLogRetentionDays = 14
	MinGameLogRetentionDays     = 1
	MaxGameLogRetentionDays     = 365

	// Keep the original key so existing installations retain their configured value.
	completedJobLimitKey    = "successful_job_limit"
	gameLogRetentionDaysKey = "game_log_retention_days"
)

type jobExecer interface {
	Exec(string, ...any) (sql.Result, error)
	Query(string, ...any) (*sql.Rows, error)
}

func migrateJobHistory(db *sql.DB) error {
	var applied int
	if err := db.QueryRow(`SELECT count(*) FROM schema_migrations WHERE version=?`, jobHistoryMigrationVersion).Scan(&applied); err != nil {
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

	columns, err := jobColumns(tx)
	if err != nil {
		return err
	}
	if !columns["started_at"] {
		if _, err := tx.Exec(`ALTER TABLE jobs ADD COLUMN started_at TEXT`); err != nil {
			return err
		}
	}
	if !columns["finished_at"] {
		if _, err := tx.Exec(`ALTER TABLE jobs ADD COLUMN finished_at TEXT`); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS job_events (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
 kind TEXT NOT NULL,
 stage TEXT NOT NULL DEFAULT '',
 percent INTEGER NOT NULL DEFAULT 0,
 message TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL
)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_job_events_job_id_id ON job_events(job_id,id)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS system_settings (
 name TEXT PRIMARY KEY,
 value TEXT NOT NULL,
 updated_at TEXT NOT NULL
)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE jobs SET started_at=created_at WHERE started_at IS NULL OR started_at=''`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE jobs SET finished_at=updated_at WHERE status IN ('succeeded','failed','interrupted') AND (finished_at IS NULL OR finished_at='')`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO job_events(job_id,kind,stage,percent,message,created_at)
SELECT id,'snapshot',stage,percent,
 (CASE WHEN error<>'' THEN error || ' · ' WHEN message<>'' THEN message || ' · ' ELSE '' END) ||
 '升级前任务，仅保留最终快照，执行时间为估算值',
 updated_at
FROM jobs
WHERE NOT EXISTS (SELECT 1 FROM job_events WHERE job_events.job_id=jobs.id)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(?,?)`, jobHistoryMigrationVersion, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

func jobColumns(tx *sql.Tx) (map[string]bool, error) {
	rows, err := tx.Query(`PRAGMA table_info(jobs)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

func (s *Store) JobEvents(jobID string) ([]domain.JobEvent, error) {
	rows, err := s.db.Query(`SELECT id,job_id,kind,stage,percent,message,created_at FROM job_events WHERE job_id=? ORDER BY id`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []domain.JobEvent{}
	for rows.Next() {
		var event domain.JobEvent
		var created string
		if err := rows.Scan(&event.ID, &event.JobID, &event.Kind, &event.Stage, &event.Percent, &event.Message, &created); err != nil {
			return nil, err
		}
		event.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) HasActiveJob(ctx context.Context, instanceID, kind string) (bool, error) {
	var active bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE instance_id=? AND type=? AND status IN ('pending','running'))`, instanceID, kind).Scan(&active)
	return active, err
}

func (s *Store) SaveJobWithEvent(record domain.JobRecord, event domain.JobEvent) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := saveJob(tx, record); err != nil {
		return err
	}
	if event.JobID == "" {
		event.JobID = record.ID
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = record.UpdatedAt
	}
	if _, err := tx.Exec(`INSERT INTO job_events(job_id,kind,stage,percent,message,created_at) VALUES(?,?,?,?,?,?)`,
		event.JobID, event.Kind, event.Stage, event.Percent, event.Message, event.CreatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CompletedJobLimit() (int, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM system_settings WHERE name=?`, completedJobLimitKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return DefaultCompletedJobLimit, nil
	}
	if err != nil {
		return 0, err
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < MinCompletedJobLimit || limit > MaxCompletedJobLimit {
		return 0, fmt.Errorf("invalid stored completed job limit %q", raw)
	}
	return limit, nil
}

func (s *Store) SetCompletedJobLimit(limit int) error {
	if err := validateCompletedJobLimit(limit); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO system_settings(name,value,updated_at) VALUES(?,?,?)
ON CONFLICT(name) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`,
		completedJobLimitKey, strconv.Itoa(limit), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	deleted, err := pruneCompletedJobs(tx, limit)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.notifyPrunedJobs(deleted)
	return nil
}

func (s *Store) GameLogRetentionDays() (int, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM system_settings WHERE name=?`, gameLogRetentionDaysKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return DefaultGameLogRetentionDays, nil
	}
	if err != nil {
		return 0, err
	}
	days, err := strconv.Atoi(raw)
	if err != nil || days < MinGameLogRetentionDays || days > MaxGameLogRetentionDays {
		return 0, fmt.Errorf("invalid stored game log retention days %q", raw)
	}
	return days, nil
}

func (s *Store) SetGameLogRetentionDays(days int) error {
	if days < MinGameLogRetentionDays || days > MaxGameLogRetentionDays {
		return fmt.Errorf("game log retention days must be between %d and %d", MinGameLogRetentionDays, MaxGameLogRetentionDays)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO system_settings(name,value,updated_at) VALUES(?,?,?)
ON CONFLICT(name) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`,
		gameLogRetentionDaysKey, strconv.Itoa(days), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) PruneCompletedJobs() error {
	limit, err := s.CompletedJobLimit()
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	deleted, err := pruneCompletedJobs(tx, limit)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.notifyPrunedJobs(deleted)
	return nil
}

func validateCompletedJobLimit(limit int) error {
	if limit < MinCompletedJobLimit || limit > MaxCompletedJobLimit {
		return fmt.Errorf("completed job limit must be between %d and %d", MinCompletedJobLimit, MaxCompletedJobLimit)
	}
	return nil
}

func pruneCompletedJobs(exec jobExecer, limit int) ([]string, error) {
	rows, err := exec.Query(`DELETE FROM jobs
WHERE status IN ('succeeded','failed','interrupted') AND id NOT IN (
 SELECT id FROM jobs
 WHERE status IN ('succeeded','failed','interrupted')
 ORDER BY COALESCE(NULLIF(finished_at,''),updated_at) DESC,id DESC
 LIMIT ?
)
RETURNING id`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	deleted := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		deleted = append(deleted, id)
	}
	return deleted, rows.Err()
}

func saveJob(exec jobExecer, v domain.JobRecord) error {
	_, err := exec.Exec(`INSERT INTO jobs(id,instance_id,type,status,stage,percent,message,error,created_at,updated_at,started_at,finished_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
 status=excluded.status,
 stage=excluded.stage,
 percent=excluded.percent,
 message=excluded.message,
 error=excluded.error,
 updated_at=excluded.updated_at,
 started_at=excluded.started_at,
 finished_at=excluded.finished_at`,
		v.ID, v.InstanceID, v.Type, v.Status, v.Stage, v.Percent, v.Message, v.Error,
		v.CreatedAt.UTC().Format(time.RFC3339Nano), v.UpdatedAt.UTC().Format(time.RFC3339Nano),
		formatNullableTime(v.StartedAt), formatNullableTime(v.FinishedAt))
	return err
}

func formatNullableTime(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseNullableTime(value sql.NullString) *time.Time {
	if !value.Valid || value.String == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value.String)
	if err != nil {
		return nil
	}
	return &parsed
}

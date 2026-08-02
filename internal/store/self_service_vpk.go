package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	selfServiceVPKMigrationVersion     = 10
	DefaultSelfServiceVPKRetentionDays = 7
	MinSelfServiceVPKRetentionDays     = 1
	MaxSelfServiceVPKRetentionDays     = 365
	selfServiceVPKEnabledKey           = "self_service_vpk_enabled"
	selfServiceVPKPasswordHashKey      = "self_service_vpk_password_hash"
	selfServiceVPKPasswordVersionKey   = "self_service_vpk_password_version"
	selfServiceVPKAutoDeleteKey        = "self_service_vpk_auto_delete"
	selfServiceVPKRetentionDaysKey     = "self_service_vpk_retention_days"
)

type SelfServiceVPKSettings struct {
	Enabled         bool  `json:"enabled"`
	PasswordSet     bool  `json:"password_set"`
	PasswordVersion int64 `json:"-"`
	AutoDelete      bool  `json:"auto_delete"`
	RetentionDays   int   `json:"retention_days"`
}

type SelfServiceVPK struct {
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	UploadedAt time.Time `json:"uploaded_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func migrateSelfServiceVPK(db *sql.DB) error {
	var applied int
	if err := db.QueryRow(`SELECT count(*) FROM schema_migrations WHERE version=?`, selfServiceVPKMigrationVersion).Scan(&applied); err != nil || applied > 0 {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`CREATE TABLE self_service_vpks (
 name TEXT PRIMARY KEY,
 size INTEGER NOT NULL CHECK(size >= 0),
 uploaded_at TEXT NOT NULL,
 expires_at TEXT NOT NULL
); CREATE INDEX idx_self_service_vpks_uploaded ON self_service_vpks(uploaded_at DESC,name DESC);
CREATE INDEX idx_self_service_vpks_expires ON self_service_vpks(expires_at,name)`); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(?,?)`, selfServiceVPKMigrationVersion, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SelfServiceVPKSettings() (SelfServiceVPKSettings, error) {
	values := map[string]string{}
	rows, err := s.db.Query(`SELECT name,value FROM system_settings WHERE name IN (?,?,?,?,?)`, selfServiceVPKEnabledKey, selfServiceVPKPasswordHashKey, selfServiceVPKPasswordVersionKey, selfServiceVPKAutoDeleteKey, selfServiceVPKRetentionDaysKey)
	if err != nil {
		return SelfServiceVPKSettings{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return SelfServiceVPKSettings{}, err
		}
		values[key] = value
	}
	if err := rows.Err(); err != nil {
		return SelfServiceVPKSettings{}, err
	}
	result := SelfServiceVPKSettings{Enabled: values[selfServiceVPKEnabledKey] == "1", PasswordSet: values[selfServiceVPKPasswordHashKey] != "", AutoDelete: values[selfServiceVPKAutoDeleteKey] == "1", RetentionDays: DefaultSelfServiceVPKRetentionDays}
	if raw := values[selfServiceVPKRetentionDaysKey]; raw != "" {
		result.RetentionDays, err = strconv.Atoi(raw)
		if err != nil {
			return SelfServiceVPKSettings{}, fmt.Errorf("invalid self-service VPK retention days %q", raw)
		}
	}
	if raw := values[selfServiceVPKPasswordVersionKey]; raw != "" {
		result.PasswordVersion, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return SelfServiceVPKSettings{}, fmt.Errorf("invalid self-service VPK password version %q", raw)
		}
	}
	return result, nil
}

func (s *Store) SetSelfServiceVPKSettings(enabled bool, password *string, autoDelete bool, retentionDays int) error {
	if retentionDays < MinSelfServiceVPKRetentionDays || retentionDays > MaxSelfServiceVPKRetentionDays {
		return fmt.Errorf("self-service VPK retention days must be between %d and %d", MinSelfServiceVPKRetentionDays, MaxSelfServiceVPKRetentionDays)
	}
	var hash string
	if password != nil && *password != "" {
		raw, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		hash = string(raw)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	set := func(key, value string) error {
		_, err := tx.Exec(`INSERT INTO system_settings(name,value,updated_at) VALUES(?,?,?) ON CONFLICT(name) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`, key, value, now)
		return err
	}
	boolString := func(v bool) string {
		if v {
			return "1"
		}
		return "0"
	}
	if err = set(selfServiceVPKEnabledKey, boolString(enabled)); err != nil {
		return err
	}
	if err = set(selfServiceVPKAutoDeleteKey, boolString(autoDelete)); err != nil {
		return err
	}
	if err = set(selfServiceVPKRetentionDaysKey, strconv.Itoa(retentionDays)); err != nil {
		return err
	}
	if password != nil {
		var version int64
		var rawVersion string
		scanErr := tx.QueryRow(`SELECT value FROM system_settings WHERE name=?`, selfServiceVPKPasswordVersionKey).Scan(&rawVersion)
		if scanErr != nil && scanErr != sql.ErrNoRows {
			return scanErr
		}
		if rawVersion != "" {
			version, err = strconv.ParseInt(rawVersion, 10, 64)
			if err != nil {
				return err
			}
		}
		if err = set(selfServiceVPKPasswordHashKey, hash); err != nil {
			return err
		}
		if err = set(selfServiceVPKPasswordVersionKey, strconv.FormatInt(version+1, 10)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) VerifySelfServiceVPKPassword(password string) (bool, int64, error) {
	settings, err := s.SelfServiceVPKSettings()
	if err != nil {
		return false, 0, err
	}
	var hash string
	err = s.db.QueryRow(`SELECT value FROM system_settings WHERE name=?`, selfServiceVPKPasswordHashKey).Scan(&hash)
	if err == sql.ErrNoRows || hash == "" {
		return password == "", settings.PasswordVersion, nil
	}
	if err != nil {
		return false, 0, err
	}
	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return false, settings.PasswordVersion, nil
	}
	return err == nil, settings.PasswordVersion, err
}

func (s *Store) SaveSelfServiceVPK(item SelfServiceVPK) error {
	_, err := s.db.Exec(`INSERT INTO self_service_vpks(name,size,uploaded_at,expires_at) VALUES(?,?,?,?)`, item.Name, item.Size, item.UploadedAt.UTC().Format(time.RFC3339Nano), item.ExpiresAt.UTC().Format(time.RFC3339Nano))
	return err
}
func (s *Store) RenameSelfServiceVPK(oldName, newName string) error {
	_, err := s.db.Exec(`UPDATE self_service_vpks SET name=? WHERE name=?`, newName, oldName)
	return err
}
func (s *Store) UpdateSelfServiceVPKSize(name string, size int64) error {
	_, err := s.db.Exec(`UPDATE self_service_vpks SET size=? WHERE name=?`, size, name)
	return err
}
func (s *Store) DeleteSelfServiceVPK(name string) error {
	_, err := s.db.Exec(`DELETE FROM self_service_vpks WHERE name=?`, name)
	return err
}

func (s *Store) ListSelfServiceVPKs(limit, offset int) ([]SelfServiceVPK, int, error) {
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, 0, errors.New("invalid pagination")
	}
	var total int
	if err := s.db.QueryRow(`SELECT count(*) FROM self_service_vpks`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query(`SELECT name,size,uploaded_at,expires_at FROM self_service_vpks ORDER BY uploaded_at DESC,name DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanSelfServiceVPKs(rows)
	return items, total, err
}

func (s *Store) ExpiredSelfServiceVPKs(now time.Time) ([]SelfServiceVPK, error) {
	rows, err := s.db.Query(`SELECT name,size,uploaded_at,expires_at FROM self_service_vpks WHERE expires_at < ? ORDER BY expires_at,name`, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSelfServiceVPKs(rows)
}

func scanSelfServiceVPKs(rows *sql.Rows) ([]SelfServiceVPK, error) {
	items := []SelfServiceVPK{}
	for rows.Next() {
		var item SelfServiceVPK
		var uploaded, expires string
		if err := rows.Scan(&item.Name, &item.Size, &uploaded, &expires); err != nil {
			return nil, err
		}
		var err error
		item.UploadedAt, err = time.Parse(time.RFC3339Nano, uploaded)
		if err != nil {
			return nil, err
		}
		item.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

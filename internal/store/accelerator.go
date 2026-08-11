package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	acceleratorSettingsKey        = "accelerator_settings"
	crashAnalysisSettingsKey      = "crash_analysis_settings"
	acceleratorMigrationVersion   = 13
	defaultAcceleratorDownloadURL = "http://sp2.0721play.icu/d/Discord%E5%85%B1%E4%BA%AB%E6%96%87%E4%BB%B6%E5%A4%B9/accelerator-2.6.0-static-x86-linux.zip"
)

type AcceleratorSettings struct {
	DownloadURL    string `json:"download_url"`
	UseGitHubProxy bool   `json:"use_github_proxy"`
}

type CrashAnalysisSettings struct {
	Endpoint  string `json:"endpoint"`
	Model     string `json:"model"`
	APIKeySet bool   `json:"-"`
}

func migrateAcceleratorCapabilities(db *sql.DB) error {
	var applied int
	if err := db.QueryRow(`SELECT count(*) FROM schema_migrations WHERE version=?`, acceleratorMigrationVersion).Scan(&applied); err != nil {
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
	columns, err := instanceColumns(tx)
	if err != nil {
		return err
	}
	if !columns["accelerator_enabled"] {
		if _, err := tx.Exec(`ALTER TABLE instances ADD COLUMN accelerator_enabled INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if !columns["auto_crash_analysis"] {
		if _, err := tx.Exec(`ALTER TABLE instances ADD COLUMN auto_crash_analysis INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(?,?)`, acceleratorMigrationVersion, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

func instanceColumns(tx *sql.Tx) (map[string]bool, error) {
	rows, err := tx.Query(`PRAGMA table_info(instances)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := make(map[string]bool)
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

func (s *Store) AcceleratorSettings(ctx context.Context) (AcceleratorSettings, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM system_settings WHERE name=?`, acceleratorSettingsKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return AcceleratorSettings{DownloadURL: defaultAcceleratorDownloadURL}, nil
	}
	if err != nil {
		return AcceleratorSettings{}, err
	}
	var settings AcceleratorSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return AcceleratorSettings{}, fmt.Errorf("decode Accelerator settings: %w", err)
	}
	if err := validateAcceleratorSettings(settings); err != nil {
		return AcceleratorSettings{}, fmt.Errorf("invalid stored Accelerator settings: %w", err)
	}
	return settings, nil
}

func (s *Store) SaveAcceleratorSettings(ctx context.Context, settings AcceleratorSettings) error {
	if err := validateAcceleratorSettings(settings); err != nil {
		return err
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO system_settings(name,value,updated_at) VALUES(?,?,?) ON CONFLICT(name) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`, acceleratorSettingsKey, string(raw), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) CrashAnalysisSettings(ctx context.Context) (CrashAnalysisSettings, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM system_settings WHERE name=?`, crashAnalysisSettingsKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return CrashAnalysisSettings{}, nil
	}
	if err != nil {
		return CrashAnalysisSettings{}, err
	}
	var settings CrashAnalysisSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return CrashAnalysisSettings{}, fmt.Errorf("decode crash analysis settings: %w", err)
	}
	if err := validateCrashAnalysisSettings(settings); err != nil {
		return CrashAnalysisSettings{}, fmt.Errorf("invalid stored crash analysis settings: %w", err)
	}
	return settings, nil
}

func (s *Store) SaveCrashAnalysisSettings(ctx context.Context, settings CrashAnalysisSettings) error {
	if err := ValidateCrashAnalysisSettings(settings); err != nil {
		return err
	}
	settings.APIKeySet = false
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO system_settings(name,value,updated_at) VALUES(?,?,?) ON CONFLICT(name) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`, crashAnalysisSettingsKey, string(raw), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func ValidateCrashAnalysisSettings(settings CrashAnalysisSettings) error {
	return validateCrashAnalysisSettings(settings)
}

func validateAcceleratorSettings(settings AcceleratorSettings) error {
	if settings.DownloadURL == "" {
		return nil
	}
	parsed, err := url.Parse(settings.DownloadURL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("download URL must use HTTP or HTTPS without credentials, query, or fragment")
	}
	return nil
}

func validateCrashAnalysisSettings(settings CrashAnalysisSettings) error {
	if !utf8.ValidString(settings.Model) || len(settings.Model) > 256 || strings.ContainsAny(settings.Model, "\x00\r\n") {
		return errors.New("model must be a bounded printable value")
	}
	if settings.Endpoint == "" {
		return nil
	}
	parsed, err := url.Parse(settings.Endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return errors.New("endpoint must use HTTPS or loopback HTTP")
	}
	if parsed.Scheme == "http" && !isLoopbackHost(parsed.Hostname()) {
		return errors.New("HTTP analysis endpoint must be loopback")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

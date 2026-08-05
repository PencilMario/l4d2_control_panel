package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/not0721here/l4d2-control-panel/internal/databaseconfig"
)

const databaseConfigKey = "database_config"

func (s *Store) DatabaseConfig(ctx context.Context) (databaseconfig.Config, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM system_settings WHERE name=?`, databaseConfigKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return databaseconfig.Defaults(), nil
	}
	if err != nil {
		return databaseconfig.Config{}, err
	}
	var config databaseconfig.Config
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return databaseconfig.Config{}, fmt.Errorf("decode database config: %w", err)
	}
	return config, databaseconfig.Validate(config)
}

func (s *Store) SaveDatabaseConfig(ctx context.Context, config databaseconfig.Config) error {
	config = databaseconfig.Normalize(config)
	if err := databaseconfig.Validate(config); err != nil {
		return err
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO system_settings(name,value,updated_at) VALUES(?,?,CURRENT_TIMESTAMP)
		ON CONFLICT(name) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`, databaseConfigKey, string(raw))
	return err
}

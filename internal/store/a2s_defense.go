package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

const a2sDefenseSettingsKey = "a2s_defense"

func (s *Store) A2SDefenseSettings(ctx context.Context) (domain.A2SDefenseSettings, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM system_settings WHERE name=?`, a2sDefenseSettingsKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return domain.A2SDefenseSettings{}, nil
	}
	if err != nil {
		return domain.A2SDefenseSettings{}, err
	}
	var settings domain.A2SDefenseSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return domain.A2SDefenseSettings{}, fmt.Errorf("decode A2S defense settings: %w", err)
	}
	if settings.Revision < 0 {
		return domain.A2SDefenseSettings{}, fmt.Errorf("invalid stored A2S defense revision %d", settings.Revision)
	}
	return settings, nil
}

func (s *Store) SaveA2SDefenseSettings(ctx context.Context, settings domain.A2SDefenseSettings) error {
	if settings.Revision < 0 {
		return fmt.Errorf("A2S defense revision must not be negative")
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO system_settings(name,value,updated_at) VALUES(?,?,?)
ON CONFLICT(name) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`,
		a2sDefenseSettingsKey, string(raw), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

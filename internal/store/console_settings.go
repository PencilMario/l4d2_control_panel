package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"
)

const (
	DefaultConsoleHistoryLines = 8192
	MinConsoleHistoryLines     = 1
	MaxConsoleHistoryLines     = 1_000_000

	consoleHistoryLinesKey = "console_history_lines"
)

func (s *Store) ConsoleHistoryLines() (int, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM system_settings WHERE name=?`, consoleHistoryLinesKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultConsoleHistoryLines, nil
	}
	if err != nil {
		return 0, err
	}
	lines, err := strconv.Atoi(raw)
	if err != nil || lines < MinConsoleHistoryLines || lines > MaxConsoleHistoryLines {
		return 0, fmt.Errorf("invalid stored console history lines %q", raw)
	}
	return lines, nil
}

func (s *Store) SetConsoleHistoryLines(lines int) error {
	if err := validateConsoleHistoryLines(lines); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO system_settings(name,value,updated_at) VALUES(?,?,?)
ON CONFLICT(name) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`,
		consoleHistoryLinesKey, strconv.Itoa(lines), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func validateConsoleHistoryLines(lines int) error {
	if lines < MinConsoleHistoryLines || lines > MaxConsoleHistoryLines {
		return fmt.Errorf("console history lines must be between %d and %d", MinConsoleHistoryLines, MaxConsoleHistoryLines)
	}
	return nil
}

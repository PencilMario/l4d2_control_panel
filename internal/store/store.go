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
	_, err := s.db.ExecContext(ctx, `INSERT INTO instances(id,node_id,name,game_port,sourcetv_port,start_map,game_mode,tickrate,max_players,extra_args,runtime_image,package_version,desired_state,actual_state,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, fields(v)...)
	return err
}
func (s *Store) UpdateInstance(ctx context.Context, v domain.Instance) error {
	v.UpdatedAt = time.Now().UTC()
	r, err := s.db.ExecContext(ctx, `UPDATE instances SET node_id=?,name=?,game_port=?,sourcetv_port=?,start_map=?,game_mode=?,tickrate=?,max_players=?,extra_args=?,runtime_image=?,package_version=?,desired_state=?,actual_state=?,updated_at=? WHERE id=?`, v.NodeID, v.Name, v.GamePort, v.SourceTVPort, v.StartMap, v.GameMode, v.Tickrate, v.MaxPlayers, v.ExtraArgs, v.RuntimeImage, v.PackageVersion, v.DesiredState, v.ActualState, v.UpdatedAt.Format(time.RFC3339Nano), v.ID)
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

const selectInstance = `SELECT id,node_id,name,game_port,sourcetv_port,start_map,game_mode,tickrate,max_players,extra_args,runtime_image,package_version,desired_state,actual_state,created_at,updated_at FROM instances`

type scanner interface{ Scan(...any) error }

func scanInstance(s scanner) (domain.Instance, error) {
	var v domain.Instance
	var c, u string
	err := s.Scan(&v.ID, &v.NodeID, &v.Name, &v.GamePort, &v.SourceTVPort, &v.StartMap, &v.GameMode, &v.Tickrate, &v.MaxPlayers, &v.ExtraArgs, &v.RuntimeImage, &v.PackageVersion, &v.DesiredState, &v.ActualState, &c, &u)
	if err == nil {
		v.CreatedAt, _ = time.Parse(time.RFC3339Nano, c)
		v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, u)
	}
	return v, err
}
func fields(v domain.Instance) []any {
	return []any{v.ID, v.NodeID, v.Name, v.GamePort, v.SourceTVPort, v.StartMap, v.GameMode, v.Tickrate, v.MaxPlayers, v.ExtraArgs, v.RuntimeImage, v.PackageVersion, v.DesiredState, v.ActualState, v.CreatedAt.Format(time.RFC3339Nano), v.UpdatedAt.Format(time.RFC3339Nano)}
}

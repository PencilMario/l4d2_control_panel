package store

const initialSchema = `
CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS instances (
 id TEXT PRIMARY KEY, node_id TEXT NOT NULL DEFAULT 'local', name TEXT NOT NULL UNIQUE,
 game_port INTEGER NOT NULL UNIQUE, sourcetv_port INTEGER NOT NULL DEFAULT 0,
 start_map TEXT NOT NULL, game_mode TEXT NOT NULL, tickrate INTEGER NOT NULL,
 max_players INTEGER NOT NULL, extra_args TEXT NOT NULL DEFAULT '', runtime_image TEXT NOT NULL,
 package_version TEXT NOT NULL DEFAULT '', desired_state TEXT NOT NULL, actual_state TEXT NOT NULL,
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (1, CURRENT_TIMESTAMP);
`

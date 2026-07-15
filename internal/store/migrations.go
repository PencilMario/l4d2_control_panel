package store

const initialSchema = `
CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS instances (
 id TEXT PRIMARY KEY, node_id TEXT NOT NULL DEFAULT 'local', name TEXT NOT NULL UNIQUE, container_id TEXT NOT NULL DEFAULT '',
 game_port INTEGER NOT NULL UNIQUE, sourcetv_port INTEGER NOT NULL DEFAULT 0,
 start_map TEXT NOT NULL, game_mode TEXT NOT NULL, tickrate INTEGER NOT NULL,
 max_players INTEGER NOT NULL, extra_args TEXT NOT NULL DEFAULT '', runtime_image TEXT NOT NULL,
 package_version TEXT NOT NULL DEFAULT '', selected_package_id TEXT NOT NULL DEFAULT '',
 desired_state TEXT NOT NULL, actual_state TEXT NOT NULL,
 created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS administrator (
 singleton INTEGER PRIMARY KEY CHECK(singleton = 1), password_hash BLOB NOT NULL,
 salt BLOB NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
 token_hash BLOB PRIMARY KEY, expires_at TEXT NOT NULL, created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS jobs (
 id TEXT PRIMARY KEY, instance_id TEXT NOT NULL, type TEXT NOT NULL, status TEXT NOT NULL,
 stage TEXT NOT NULL DEFAULT '', percent INTEGER NOT NULL DEFAULT 0, message TEXT NOT NULL DEFAULT '',
 error TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS audit_events (
 id TEXT PRIMARY KEY, action TEXT NOT NULL, target TEXT NOT NULL, result TEXT NOT NULL,
 metadata TEXT NOT NULL DEFAULT '{}', created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS scheduled_tasks (
 id TEXT PRIMARY KEY, instance_id TEXT NOT NULL, type TEXT NOT NULL, cron TEXT NOT NULL,
 timezone TEXT NOT NULL, online_policy TEXT NOT NULL, payload TEXT NOT NULL DEFAULT '{}',
 enabled INTEGER NOT NULL, last_run TEXT NOT NULL DEFAULT '', next_run TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS secrets (name TEXT PRIMARY KEY, ciphertext BLOB NOT NULL, updated_at TEXT NOT NULL);
INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (1, CURRENT_TIMESTAMP);
CREATE TABLE IF NOT EXISTS instance_plugin_ports (
 instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
 port INTEGER NOT NULL CHECK(port BETWEEN 1024 AND 65535),
 PRIMARY KEY(instance_id, port)
);
INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (2, CURRENT_TIMESTAMP);
`

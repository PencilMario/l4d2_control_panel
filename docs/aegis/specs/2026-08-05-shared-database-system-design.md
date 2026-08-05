# Shared Database System Design

## Intent

Add a panel-level **Database System** that owns one SourceMod database configuration for every game instance. Administrators edit structured connection fields rather than raw `databases.cfg`. The generated file is authoritative even when plugin packages or private files contain their own copy.

## Defaults And UI

First-run and restore-default values match the supplied example: default driver `mysql`; connections `default`, `storage-local`, and `clientprefs` with the same fields and values.

The global page provides structured connection cards, add/delete/rename controls, password reveal, restore defaults, save-and-sync, resync, last synchronization state, failed instances, and a link to the background Job. Names must be non-empty and unique.

When `driver` is `sqlite` and `host` is `localhost`, the UI hides and clears `port` and `timeout`; serialization omits both keys.

## Model And Files

One ordered structured model is stored in `system_settings`, including revision and synchronization metadata. Passwords are required in the final plaintext SourceMod file, but must never appear in request logs, Job messages, or diagnostic logs.

A deterministic renderer owns KeyValues quoting, escaping, indentation, and optional-field omission. The browser never submits raw KeyValues.

Installed instances receive `instances/<instance ID>/game/left4dead2/addons/sourcemod/configs/databases.cfg` using a same-directory temporary file and atomic replacement. Uninstalled instances do not receive a fake game tree; they are deferred until installation or startup.

## Observable Background Job

Saving and manual resync create one globally serialized Job of type `database_sync`. Existing Job API, SSE progress, history, and task logs remain the observation surface.

Stages are `validate`, `persist`, `discover`, `render`, `sync`, and `complete`. Per-instance logs identify only instance and outcome. Synchronization continues after individual failures; any failed installed instance fails the Job with a redacted summary while preserving the canonical saved configuration. Deferred uninstalled instances do not fail the Job.

## Reconciliation Boundaries

Lower-layer writers run first and database reconciliation runs last. The authoritative file is reapplied after save/resync, initial provisioning, plugin install/reinstall/upgrade/redeployment, private-file replay/import, overlay recovery/reconstruction, and before instance start or restart.

Safety reconciliation inside another operation reuses its existing Job reporter and never creates a nested Job. Start and restart fail closed when reconciliation fails. A package or private deployment whose final reconciliation fails also fails its enclosing Job.

## API

- `GET /api/settings/databases` returns the canonical model and synchronization summary.
- `PUT /api/settings/databases` validates a complete model and creates a `database_sync` Job.
- `POST /api/settings/databases/sync` creates a Job for the current revision.
- `GET /api/settings/databases/defaults` returns built-in defaults without mutation.

Revision checks prevent stale-tab overwrites.

## Compatibility And Verification

Existing instance, package, private-file, lifecycle, Job, and SSE shapes remain compatible. Existing writers remain, but their `databases.cfg` can no longer become authoritative. This feature does not install database servers, test connectivity, or migrate schemas.

Tests cover defaults, validation, deterministic rendering, SQLite omission, atomic writes, deferred instances, partial failure, credential redaction, Job progress, revision conflicts, every reconciliation hook, structured UI editing, Job observation, and restoration after plugin reinstall.


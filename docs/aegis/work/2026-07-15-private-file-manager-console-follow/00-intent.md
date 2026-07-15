# Task intent

Implement the approved private overlay file manager and console follow behavior from `docs/aegis/specs/2026-07-15-private-file-manager-console-follow-design.md` using the task sequence in `docs/aegis/plans/2026-07-15-private-file-manager-console-follow.md`.

Scope: complete management of `instances/<id>/private/`, staged changes, explicit Job application, lower-layer restoration, 20 snapshots, independent UI Tab, and console follow state.

Non-goals: game-directory browsing, arbitrary Shell, cross-instance file operations, collaboration, or changes to the PTY protocol and overlay priority.

Risk hints: path safety, transactional rollback, lower-layer ownership after package/shared updates, existing-instance baseline migration, snapshot disk use, and distinguishing user scroll from programmatic scroll.

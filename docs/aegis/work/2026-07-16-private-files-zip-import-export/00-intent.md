# Task Intent Draft

Updated: 2026-07-16 +08:00

Add ZIP import/export to the selected instance's private-files page. Import must completely replace the current staged workspace rather than merge, preserve the ZIP's sole top-level directory, leave applied history intact, explain data loss before upload and never apply automatically.

Risk hints: archive path safety, ZIP bombs, file/directory collisions, crash-safe workspace replacement, concurrent private mutations, accidental auto-apply and destructive-warning clarity.

Non-goals: merge import, archive metadata, snapshot deletion, common-root stripping, cross-instance copy and other archive formats.

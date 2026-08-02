# Self-service VPK upload

## Intent

Add an independently accessible `/uploadvpk` page where non-admin users can upload maps into the existing shared VPK repository. Administrators control whether the page is enabled, its optional access password, and whether self-service uploads expire automatically.

The feature must preserve the existing administrator content-repository workflow and reuse its resumable upload, SHA-256 verification, and VPK cleanup behavior.

## User experience

### Administrator settings

System settings gain a self-service VPK section with:

- an enable switch, disabled by default;
- an optional access password, which may be empty;
- an automatic-deletion switch;
- a global retention period in whole days.

The password is write-only. The panel reports whether one is configured but never returns its plaintext value. Changing or clearing it invalidates all previously issued self-service authorizations. Disabling the feature also makes existing authorizations unusable.

### Public page

`/uploadvpk` is an independent page and does not require an administrator session. When the feature is disabled, it shows that self-service upload is unavailable and its APIs reject requests.

When a password is configured, the user must enter it before viewing uploads or starting an upload. Successful verification grants that browser a 24-hour, HttpOnly authorization cookie scoped to the self-service feature. When the password is empty, the page and self-service APIs are publicly accessible.

The authorized page contains:

- a read-only list of VPKs successfully uploaded through this entry point;
- a file picker and resumable upload queue;
- a per-file choice between direct upload and cleanup before upload;
- progress, recovery, completion, and actionable error states.

The list is ordered by upload completion time descending and is paginated. It shows filename, size, upload completion time, and expiration status. It does not expose administrator-uploaded VPKs and offers no download, delete, rename, or overwrite action.

## Upload behavior

Self-service HTTP endpoints are separate from the authenticated administrator API. They reuse the existing upload manager and VPK cleanup implementation so chunking, recovery, hashing, filename validation, and cleanup semantics remain consistent.

A self-service upload follows this flow:

1. Check that the feature is enabled and that self-service authorization is valid when a password is configured.
2. Create or recover a resumable upload session.
3. Receive chunks through the existing offset-based protocol.
4. Verify the completed file size and SHA-256 digest.
5. Optionally clean the VPK before publication.
6. Atomically publish it into the shared VPK repository.
7. Persist self-service provenance and expiration metadata.
8. Refresh the public list.

Any filename collision with the shared repository is rejected. A self-service user cannot overwrite either an administrator upload or another self-service upload.

Only a successfully published VPK receives self-service metadata. Incomplete temporary upload sessions continue to use the existing upload-session expiry and recovery rules. The VPK retention period begins at successful publication, not at the beginning of transfer.

## Persistence and authorization

The existing `system_settings` store owns these settings:

- enabled state;
- password hash;
- password version used to revoke old authorizations;
- automatic-deletion state;
- retention days.

Plaintext passwords are never persisted or returned. Self-service authorization is distinct from administrator authentication and grants no access to administrator APIs.

Persistent metadata for each completed self-service upload records:

- filename;
- size;
- upload completion timestamp;
- fixed expiration timestamp;
- self-service provenance.

The expiration timestamp is calculated from the configured retention period at publication time. Changing retention days affects new uploads only. Renaming an item through the administrator content repository preserves its original expiration timestamp and updates its metadata filename. Administrator deletion removes matching metadata. Cleaning a published VPK preserves its provenance and expiration timestamp while updating its size as needed.

Administrator-uploaded VPKs do not receive self-service metadata and never become eligible for this feature's automatic deletion.

## Expiration and cleanup

A background cleanup pass runs hourly. When automatic deletion is enabled, it deletes every self-service VPK whose fixed expiration timestamp has passed, together with its metadata. It does not check whether a game instance currently references the VPK.

Deletion failures retain metadata so a later hourly pass retries the operation. Disabling automatic deletion pauses deletion without erasing expiration timestamps. The list marks such entries as paused. Re-enabling deletion makes already-expired entries eligible on the next pass.

Disabling the self-service feature does not remove existing VPKs or metadata.

## Errors and security boundaries

The API and page distinguish at least these conditions:

- feature disabled;
- incorrect password;
- missing, expired, or revoked authorization;
- unsafe filename or invalid VPK;
- shared-repository filename collision;
- expired upload session;
- upload offset, size, or hash mismatch;
- cleanup failure;
- publication or storage failure.

Failed resumable tasks remain visible for retry when retry is meaningful. Filename collisions require selecting a different filename and are not blindly retried.

Rate limiting and malware scanning are outside this feature's scope. Operators who configure an empty password intentionally make both the self-service list and upload capability available to every client that can reach the route.

## Compatibility boundary

- Existing administrator shared-VPK routes and UI behavior remain compatible.
- Existing shared-VPK storage paths and game-instance mounts do not change.
- Administrators continue to see and manage self-service VPKs in the content repository.
- Existing administrator VPKs are not backfilled into the self-service list.
- Self-service authorization never substitutes for an administrator session.
- No public download, deletion, rename, overwrite, or game-instance management capability is added.

## Verification

Backend integration coverage must include feature enablement, empty-password public access, password verification, 24-hour authorization, authorization revocation after password changes, list isolation, pagination, resumable upload, direct upload, cleanup-before-upload, collision rejection, and disabled-feature rejection.

Storage coverage must include default settings, password hashing and versioning, metadata lifecycle, rename and cleanup synchronization, stable expiration timestamps, ordering, pagination, restart persistence, expiry deletion, paused cleanup, and deletion retry.

Frontend coverage must include disabled, password, public, authorized, loading, empty, populated, uploading, recoverable error, collision, and authorization-expired states. The main browser journey must verify that a user can unlock the page, inspect the read-only list, upload using either mode, and see the completed item appear.

## Design inputs

### Task intent

Provide controlled self-service map publication without granting panel administrator access. The principal risks are exposing privileged APIs, overwriting managed content, deleting administrator uploads, and losing large transfers.

### Baseline read set

- `CONTEXT.md` defines project terminology and ownership boundaries.
- `internal/httpapi/server.go` owns routing, administrator authentication, settings endpoints, and current VPK handlers.
- `internal/content/uploads.go` owns resumable shared-VPK publication and collision behavior.
- `internal/content/vpk_cleanup.go` owns cleanup-before-upload behavior.
- `internal/store/job_history.go` establishes the persisted system-settings pattern.
- `web/src/vpk/uploadQueue.ts` and the content repository UI establish the current browser upload protocol and experience.

### Impact statement

The change affects public routing, scoped authorization, system settings, shared-VPK metadata, periodic maintenance, and a standalone frontend page. Existing administrator APIs and shared storage remain the compatibility boundary. Public content management beyond upload and list visibility is explicitly excluded.

# Aria2 Release Download Design

## Goal

Replace the in-process GitHub Release asset transfer with mature aria2 segmented downloading. Default to eight connections and let administrators configure one to sixteen connections in System Settings.

## Architecture

Go continues to own GitHub API requests, asset selection, trust validation, package reuse, ZIP inspection, hashing, metadata, and publication. A focused downloader invokes `aria2c` for only the asset bytes and monitors the destination file for existing task progress reporting. Private-asset authentication is exchanged for a GitHub redirect URL before invoking aria2 so the token never appears in process arguments or aria2 logs.

The connection count is stored in the existing `system_settings` table. Each new transfer reads the current value, so saving takes effect on the next download without mutating an active transfer.

## Behavior

- Default connection count: 8; accepted range: 1-16.
- aria2 owns HTTP range splitting, retries, resume behavior, proxy use, and fallback when Range is unavailable.
- Existing 2 GiB default limit, cancellation, archive validation, SHA-256, atomic package publication, package reuse identity, API entry points, and deployment behavior remain unchanged.
- Download progress remains visible through task logs without parsing aria2 console output.
- Signed URLs, GitHub tokens, and absolute local paths are never logged.
- The runtime image includes `aria2`; absence of the executable produces an explicit transfer error.

## Components

- `internal/store`: validated persistent connection setting.
- `internal/releases`: downloader interface, aria2 process adapter, authenticated redirect exchange, and existing Release orchestration.
- `internal/httpapi`: GET/PUT download settings endpoints.
- `web/src/app/App.tsx`: System Settings numeric control.
- `Dockerfile`: runtime aria2 package.

## Error Handling

Cancellation terminates aria2 and removes partial/control files through existing cleanup ownership. Nonzero exit, oversized output, missing output, unsafe redirect hosts, and invalid settings fail the job without publishing a package. GitHub API and asset-selection errors retain their current responses.

## Verification

Unit tests cover arguments, cancellation, size enforcement, token isolation, redirect trust, defaults and validation. HTTP and frontend tests cover settings round trips. Release tests retain reuse, archive, logs, and failure coverage. Full Go tests, frontend tests/build, and Dockerfile assertions provide regression evidence.

## Compatibility And Non-goals

No API route used for Release synchronization or instance deployment changes. No general-purpose download manager, queue, bandwidth control, or active-download connection reconfiguration is added. Direct Go streaming at the GitHub asset site is retired; Go HTTP remains responsible for metadata and authenticated URL exchange.


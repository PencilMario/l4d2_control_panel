# Overview live status and metrics repair

## TaskIntentDraft

- Outcome: make the overview status light, CPU, memory and player count reflect live Docker/A2S observations instead of stale persistence or silent zero fallbacks.
- Scope: Docker stats sampling, the A2S adapter, one authenticated overview observation endpoint, and periodic React overview refresh.
- Non-goals: changing lifecycle persistence, player kick/ban semantics, Docker networking, or the approved instance state machine.
- Risk hints: preserve the existing `/players` action contract; distinguish a legitimate numeric zero from an unavailable observation; do not expose third-party A2S types outside `internal/a2s`.

## ImpactStatementDraft

The change crosses Docker, A2S/player services, HTTP JSON and the React overview. Docker remains the owner of container liveness, A2S_INFO becomes the owner of live map/player capacity/count, and SQLite remains the owner of desired/configured state. The existing detailed player endpoint continues to use A2S_PLAYER plus console `status` for stable UserID mapping.

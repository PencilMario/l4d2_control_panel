# Per-instance A2S defense log design

- Status: approved in conversation
- Date: 2026-08-05
- Scope: sampled IPv4 A2S drop events written into each instance game-log directory

## Objective

Create `instances/<id>/logs/game/a2s_protect.log` for instances whose game or SourceTV port receives a sampled A2S defense drop. Each line identifies the event time, source IPv4 address, destination port, query type, and DROP action. The file must appear in the existing game-log browser and inherit existing retention, size, preview, and download behavior.

## Architecture

The firewall sends a rate-limited copy of actual drop-path packets to a fixed NFLOG group before the terminal DROP. The A2S Helper listens to that group with only its existing `NET_ADMIN` capability, parses the IPv4/UDP metadata and fixed A2S signature, and stores sanitized typed events in a bounded in-memory ring.

The Helper API adds a read-only cursor endpoint over the existing restricted Unix socket. The Panel polls that endpoint, maps each destination port to all configured instances whose `game_port` or non-zero `sourcetv_port` matches, and appends a normalized line to each matching instance log. The Helper never receives the Panel data root, database, Docker socket, or arbitrary file path. The Panel never receives NFLOG, kernel-log, or firewall capability.

## Event contract

An event contains only:

- Helper boot identifier and monotonically increasing sequence;
- UTC timestamp;
- sanitized source IPv4 address;
- destination UDP port;
- one fixed query type: `A2S_INFO`, `A2S_PLAYER`, `A2S_RULES`, `CHALLENGE`, or `OTHER_69`;
- action `DROP`.

The cursor response reports the oldest/newest available sequence and a lost-event count when the requested cursor is older than the bounded ring. Unknown, malformed, non-IPv4, non-UDP, non-protected-port, or unsupported-signature packets are ignored. Requests accept only the typed boot/cursor values and never accept a path, command, rule, group, or raw payload.

## Logging behavior

The Panel appends one ASCII line per sampled event:

```text
2026-08-05T07:42:10Z src=203.0.113.8 dst_port=27015 query=A2S_PLAYER action=DROP
```

When events were overwritten or a Helper restart invalidates a cursor, the Panel writes one summary line with `events_lost=<count>` when the count is known and resumes from the oldest available event. Duplicate port ownership intentionally writes the event to every matching instance. Plugin ports remain excluded.

File creation uses the established instance game-log directory and restrictive file permissions. Appends are serialized, bounded in line length, and do not follow a caller-controlled path. Failure to write one instance does not block firewall operation or other instances; errors are surfaced in Panel logs and retried only for later events, not by duplicating a failed event indefinitely.

## Flood control and availability

NFLOG sampling stays on the actual drop path and is limited to the existing global five events per minute with a burst of five. Both newly rate-limited packets and packets dropped by the active blacklist use the same sampled event path. The terminal DROP remains independent of the listener, Unix socket, Panel, filesystem, or event queue.

The Helper ring holds 256 events. It is intentionally ephemeral: container restart may lose unsent samples and changes the boot identifier. A full ring overwrites the oldest sample and reports loss through the cursor contract. Logging is operational evidence, not an audit-complete packet capture.

## Compatibility and security boundary

- IPv4 only; no IPv6 rules, parser, or log events.
- `a2s-defense-helper` remains the only component with `NET_ADMIN`; no new capability or privileged mode is added.
- The existing Unix socket remains the only Helper API transport.
- Existing desired-state, reconciliation, counters, blacklist, startup gate, and disabled behavior remain unchanged.
- When defense is disabled, no NFLOG rule is installed and the Panel writes no defense events.
- Existing game-log cleanup owns retention and maximum-size enforcement; no second rotation system is introduced.
- Kernel `LOG`, `dmesg`, journald, rsyslog, `/dev/kmsg`, and host log mounts are not used.

## Testing

Unit tests cover packet parsing, fixed query mapping, ring cursors, overflow/restart semantics, strict API decoding, port-to-instance fan-out, safe log formatting, serialized appends, and write failures. Rule tests require NFLOG only on the actual drop path and preserve terminal DROP ordering.

Linux integration runs inside the disposable network namespace on `安可服`, sends a known A2S flood, reads emitted events, verifies source/destination/type, confirms the packet is still dropped, and proves host rules remain unchanged. Compose tests confirm no new capability or mount. HTTP/game-log tests confirm `a2s_protect.log` appears through the existing tree/preview/download endpoints. Browser tests confirm the file can be selected in the existing game-log UI.

## Design inputs

### TaskIntentDraft

Provide a readable per-instance A2S defense log alongside existing game logs without weakening the firewall Helper isolation boundary.

### BaselineReadSetHint

The design is constrained by the approved A2S Helper specification, `internal/a2sdefense`, `cmd/panel/main.go`, `internal/gamelogs`, instance port ownership in SQLite, Docker game-log mounts, and the existing game-log API/UI.

### ImpactStatementDraft

The change affects fixed firewall logging rules, the Helper event listener/ring/API, Panel polling and instance log appends, and focused tests/documentation. It does not change user-adjustable defense policy, instance networking, game container permissions, or host logging configuration.

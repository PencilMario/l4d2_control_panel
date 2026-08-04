# A2S Defense Helper Design

- Status: approved
- Date: 2026-08-05
- Scope: IPv4 A2S query defense for host-networked L4D2 instances

## 1. Objective

Add an optional host-firewall defense managed from the Panel system settings. A dedicated Helper applies fixed IPv4 A2S rate limits to every configured game and SourceTV port while preserving the existing privilege boundary of the Panel and game containers.

The first release provides a global enable switch, automatic port synchronization, defense status, protected ports, drop counters, blacklist size, and the most recent apply error. Rate limits are fixed product policy rather than administrator settings.

## 2. Existing Constraints

- Game instances use `network_mode: host`, so inbound A2S traffic traverses the host `INPUT` chain.
- The Panel and game containers must not receive `NET_ADMIN` or direct firewall access.
- The Panel already communicates with privileged helpers through group-restricted Unix sockets.
- Instance ports are authoritative in SQLite. The Helper must not read the database or inspect Docker.
- The project supports IPv4 only. This feature does not create or modify IPv6 rules.
- Existing host firewall rules, UFW/firewalld state, Docker chains, and administrator-owned chains must remain intact.

This feature deliberately revises the original project non-goal that excluded host firewall management. The exception is narrow: only `a2s-defense-helper` may manage project-owned IPv4 chains through a fixed API.

## 3. Architecture

```text
System settings page
  | GET/PUT /api/settings/a2s-defense
  v
Panel container
  | validated versioned JSON over Unix socket
  v
a2s-defense-helper
  | iptables-restore --noflush --wait
  v
Host IPv4 INPUT chain
  `- L4D2_A2S_DEFENSE project-owned rules
```

The Helper follows the existing helper isolation pattern but joins the host network namespace and owns the minimum capability needed to update its IPv4 firewall chains. It exposes no TCP listener and accepts no shell command, rule text, chain name, executable path, or arbitrary iptables argument from the Panel.

## 4. Helper Isolation

The Compose service has:

- `network_mode: host`;
- `cap_drop: [ALL]` and `cap_add: [NET_ADMIN]`;
- a read-only root filesystem and `no-new-privileges`;
- a dedicated Unix socket volume shared only with the Panel;
- no Docker Socket, host directory, database, or Panel data mount;
- no `privileged`, `SYS_ADMIN`, `SYS_PTRACE`, external port, or IPv6 access path.

The Helper uses fixed absolute executable paths and verifies `iptables`, `iptables-restore`, `iptables-save`, version compatibility, and required matches/targets. Missing support is reported as an incompatibility; the Helper does not install packages or load arbitrary kernel modules.

The socket directory is owned by root and the Panel runtime group. Its directory and socket modes prevent access by unrelated containers or host users.

## 5. API Contract

The Unix-socket HTTP API contains only:

```text
PUT /v1/config
GET /v1/status
```

Example configuration:

```json
{
  "version": 1,
  "enabled": true,
  "ports": [27015, 27020],
  "revision": 42
}
```

Requests use strict JSON decoding. Ports must be unique IPv4 UDP port numbers after normalization and are sorted before rule generation. Unknown fields, unsupported versions, invalid ports, duplicate semantic values, and revisions older than the last successfully applied revision are rejected.

`PUT /v1/config` is idempotent. Success means the complete requested configuration is active. A failed command or validation never reports partial success and must leave the previous effective rules active.

Status reports:

- compatibility and availability;
- enabled state and effective revision;
- rule policy version;
- protected ports;
- last successful application time;
- counters grouped by A2S query type;
- active blacklist entry count;
- the most recent application error.

The Helper retains no user-controlled rule language. Firewall state is the runtime authority; persisted Panel settings are the desired state.

## 6. Protected Ports

When enabled, the Panel derives the desired set from every configured instance:

- `game_port` is always included;
- a non-zero `sourcetv_port` is included;
- plugin ports are excluded because they are not necessarily A2S endpoints;
- stopped and uninstalled instances remain included so their ports are protected before exposure;
- duplicates are removed and the result is sorted.

The Helper never discovers ports independently. This keeps the database as the only configuration owner and avoids granting the privileged component Docker or storage access.

## 7. Firewall Policy

The Helper manages a stable project chain attached before any broad `ESTABLISHED,RELATED` acceptance in `INPUT`. It never changes the `INPUT` policy or flushes unrelated tables or chains.

Conceptually, the managed chain performs:

```text
active blacklist match                    -> DROP
not UDP or not a protected destination    -> RETURN
identify 0x54, 0x55, 0x56, 0x57, 0x69
per-destination aggregate limit
per-source and per-query-type limit
over limit -> mark source -> limited log -> DROP
other traffic                              -> RETURN
```

The matcher recognizes:

- A2S_INFO `0x54`;
- A2S_PLAYER `0x55`;
- A2S_RULES `0x56`;
- challenge `0x57`;
- the retained additional feature `0x69`.

Fixed version 1 policy:

| Query | Per-source rate | Burst |
| --- | ---: | ---: |
| A2S_INFO `0x54` | 20/s | 40 |
| A2S_PLAYER `0x55` | 10/s | 20 |
| A2S_RULES `0x56` | 10/s | 20 |
| Challenge `0x57` | 20/s | 40 |
| Feature `0x69` | 5/s | 10 |
| Aggregate per destination port | 300/s | 600 |

A source is blacklisted for 60 seconds after three recorded per-source violations. Marking occurs before the terminal DROP so the blacklist is effective. Drop logging is limited globally to five records per minute and only occurs on an actual drop path. Normal UDP traffic is never logged by this feature.

The policy mitigates query floods but does not claim to stop line-rate volumetric DDoS or guarantee attribution when source addresses are spoofed. The aggregate destination limit provides bounded protection against distributed or randomized-source traffic.

## 8. Atomic Rule Application

Rule changes use `iptables-restore --noflush --wait` and project-owned staging chains. A new complete ruleset is constructed and validated before the `INPUT` jump is switched. The old effective chain remains reachable until the replacement is ready.

The Helper owns exactly one effective jump and its named chains and limiter names. Reapplying the same revision does not create duplicate jumps. Enabling never clears the previous rules first.

Disabling removes only the project jump and project-owned chains. It does not alter `INPUT`, UFW/firewalld, Docker chains, connection tracking rules, or administrator chains.

Helper process termination does not remove kernel rules. A restart inspects the existing project chains and reports their effective state until the Panel reconciles the desired configuration.

## 9. Panel Settings And Synchronization

`system_settings` stores the desired enabled state, monotonically increasing revision, and last successful synchronization summary. The actual state shown to administrators comes from `GET /v1/status`.

Enabling follows an apply-before-persist transaction:

1. Read every instance and derive the protected ports.
2. Submit the next revision to the Helper.
3. Confirm the effective Helper response.
4. Persist `enabled=true`, the revision, and synchronization summary.

Disabling similarly removes the rules successfully before persisting `enabled=false`. If the Helper is unavailable, the switch operation fails and the existing desired setting remains unchanged.

Instance creation, port modification, and deletion trigger reconciliation after their database transaction. A failed reconciliation does not roll back the instance mutation; the Panel records a pending-reconciliation state and retries. A stale protected port is harmless and is removed when reconciliation recovers.

When defense is enabled, starting an instance is fail closed: the lifecycle service must confirm that the Helper reports the instance game port and non-zero SourceTV port at the current desired revision. An unprotected instance cannot newly start. Already running instances are not automatically stopped during a temporary Helper outage.

The Panel reconciles immediately at startup, after Helper recovery, after relevant instance mutations, and periodically. Manual changes to project-owned chains are restored to the Panel desired configuration and surfaced as a warning.

Firewall degradation does not fail the general `/api/health` endpoint and therefore does not trigger an unrelated deployment rollback.

## 10. System Settings UI

The server settings page provides:

- a single A2S defense toggle;
- normal, synchronizing, pending reconciliation, Helper unavailable, and incompatible states;
- the effective protected port list;
- rule policy version and last successful application time;
- drop counters grouped by query type;
- active blacklist count;
- the most recent application error.

The page does not expose rate, burst, blacklist, chain, matcher, command, or custom-port controls. Errors remain visible and the confirmed toggle value is not changed optimistically when an apply fails.

## 11. Upgrade, Rollback, And Removal

- API `version: 1` and policy version are separate so rule-policy changes do not silently alter the transport contract.
- A Panel rollback remains compatible with the version 1 Helper contract.
- A Helper image update leaves existing kernel rules active while the container restarts.
- A failed new rule application preserves the prior effective revision.
- Compose removal does not silently manipulate the host firewall.
- The project provides an explicit, narrowly scoped cleanup operation that removes only its jump and chains.
- Deployment and update documentation warns that removing the Helper container alone leaves its last kernel rules active until explicit disable or cleanup.

## 12. Testing Strategy

Unit tests cover strict request decoding, port validation and normalization, revision ordering, deterministic rule generation, every query signature, fixed limits, mark-before-drop ordering, logging placement, and cleanup ownership.

Helper tests inject command execution and cover missing tools, incompatible matches, timeouts, partial command failure, status parsing, idempotent apply, stale revisions, and preservation of the last effective configuration.

Linux integration tests use an isolated network namespace and disposable firewall state. They verify normal A2S traffic passes, excess traffic drops, blacklisting activates, non-target ports remain unaffected, aggregate limiting works, and unrelated INPUT, UFW/firewalld-style, Docker-style, and custom chains survive apply and cleanup.

Panel tests cover setting transactions, status degradation, automatic port synchronization, pending reconciliation, startup gating, recovery, and persistence. Compose tests prove only the Helper receives `NET_ADMIN` and that neither Panel nor game containers gain additional privilege.

Browser tests cover the toggle, confirmed-state behavior, protected ports, counters, incompatibility, pending reconciliation, and actionable error presentation.

Production verification covers enable, instance port change, Helper restart, Panel restart, failure injection, disable, and explicit cleanup.

## 13. Compatibility Boundary And Non-Goals

Compatibility requirements:

- existing instance, A2S client, Docker, overlay, update, and health contracts remain unchanged except for the enabled-defense startup gate;
- existing host firewall state outside project-owned chains is preserved;
- disabled installations behave exactly as before;
- the feature supports only Linux IPv4 deployments using compatible iptables extensions.

Non-goals for the first release:

- IPv6;
- administrator-adjustable thresholds;
- manual allowlists or bans;
- automatic kernel-module or package installation;
- native nftables rule generation;
- eBPF/XDP or upstream DDoS mitigation;
- plugin-port protection;
- treating defense degradation as overall Panel unhealthiness;
- protection against link saturation or all spoofed-source attacks.

## 14. Design Inputs And Impact

### TaskIntentDraft

Integrate A2S query-flood defense as an optional Helper controlled from server settings. Use fixed safe defaults, protect configured game and SourceTV ports automatically, retain IPv4-only project scope, and prevent a privileged firewall boundary from spreading into the Panel or game containers.

### BaselineReadSetHint

The design is constrained by `CONTEXT.md`, the main control-panel design, `docker-compose.yml`, deployment privilege tests, the existing Unix-socket overlay helper, instance storage and lifecycle paths, and the system settings API/UI. Implementation must additionally verify actual iptables 1.8 behavior and extension availability in supported Debian and Ubuntu environments.

### ImpactStatementDraft

The change adds a privileged but narrowly scoped Helper image, Unix-socket client and API, IPv4 rule generator, desired-state settings, reconciliation service, lifecycle startup gate, settings UI, Compose permissions, deployment documentation, and isolated Linux firewall tests. It does not grant firewall access to existing components or broaden instance/plugin/network contracts. The principal risks are accidental host firewall interference, stale port protection, and exposing an instance while desired defense is absent; project-owned chains, atomic application, reconciliation, and fail-closed startup address those risks.

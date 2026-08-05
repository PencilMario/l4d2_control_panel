# A2S defense log details design

- Status: approved in conversation
- Date: 2026-08-05
- Scope: sampled IPv4 drop metadata and live blacklist counter accuracy

## Objective

Extend each `a2s_protect.log` sample with the source UDP port, complete IPv4 packet byte length, and the number of NFLOG drop samples observed from the same source IPv4 during the trailing 60 seconds. Correct the settings counter so `blacklist` reports packets matched by the active recent-list rule even though that rule jumps to a sampled drop chain instead of directly to `DROP`.

## Event contract

`Event` adds `source_port`, `packet_bytes`, and `sampled_drops_60s`. `packet_bytes` is the IPv4 total-length field after bounds validation. The rolling count is keyed only by source IPv4, so samples for different protected destination ports and query types contribute to one count. It is sampled operational evidence, not an exact packet total.

The Helper owns aggregation at NFLOG ingestion. It retains at most the timestamps needed for the bounded 60-second window and prunes expired source entries. Out-of-order event timestamps are accepted without allowing an older sample to move the window backward. IPv4-only parsing and the existing bounded event ring remain unchanged.

Example:

```text
2026-08-05T07:42:10Z src=203.0.113.8 src_port=52144 dst_port=27015 packet_bytes=33 query=A2S_PLAYER sampled_drops_60s=3 action=DROP
```

## Counter repair

The canonical counter parser recognizes a blacklist rule when it uses `--name L4D2_A2S_ATTACKER` and targets either fixed sampled drop chain, `L4D2_A2S_DROP_A` or `L4D2_A2S_DROP_B`. Literal `DROP` remains accepted for compatibility with previously installed rules. No new counter or duplicate packet-accounting owner is introduced.

## Compatibility and safety

- NFLOG remains globally limited to five samples per minute with burst five.
- Firewall DROP remains independent of NFLOG, Helper API, Panel, and filesystem health.
- No capability, mount, listener, arbitrary path, IPv6 behavior, or policy threshold changes.
- Existing event JSON consumers remain compatible because the new fields are additive.

## Verification

Unit tests cover packet metadata, rolling-window aggregation across ports, per-IP isolation, expiry and out-of-order timestamps, log formatting, and sampled-chain blacklist counters. Related Go tests and vet must pass. Linux integration on 安可服 must prove the emitted metadata while preserving terminal DROP and host rules.

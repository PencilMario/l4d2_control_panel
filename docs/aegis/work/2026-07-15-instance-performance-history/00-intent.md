# Instance performance information and history

## TaskIntentDraft

- Outcome: show concrete per-instance CPU, memory, network, disk, process, uptime and A2S performance data in overview cards, with a one-hour chart.
- Scope: restricted proxy transport and packet counters, Docker stats expansion, a five-second in-memory sampler, additive authenticated HTTP contracts and the React overview.
- Non-goals: persistent metrics storage, alerts, packet inspection, firewall changes, undeclared-port accounting or cross-host monitoring.
- Risk hints: Host networking prevents reliable Docker per-container network counters; keep `NET_RAW` isolated to the existing proxy and preserve nullable zero semantics.

## ImpactStatementDraft

The change crosses deployment security, Docker runtime observation, an internal proxy protocol, background sampling, HTTP JSON and the overview UI. Docker remains the owner of process/resource counters, the restricted proxy owns declared-port byte counters, A2S owns game responsiveness, and Panel memory owns only the rolling one-hour history.

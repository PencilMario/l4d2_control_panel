# IPv4 A2S Defense Helper Evidence

- Date: 2026-08-05
- Branch: `feature/a2s-defense-helper`
- Scope: optional IPv4-only A2S firewall Helper controlled from system settings

## Implementation Commits

| Commit | Evidence |
| --- | --- |
| `9ed0eda` | Fixed policy and deterministic project-owned IPv4 rules |
| `9433f38` | Helper manager, Unix-socket API/client, process, and image |
| `1799543` | Compose capability and socket isolation |
| `d2df0d6` | Desired-state persistence and reconciliation |
| `fad0477` | Fail-closed instance start gate and mutation reconciliation |
| `5fe9f2c` | Authenticated settings API and React settings panel |
| `9736b9f` | Live counters, compatibility checks, and real Linux rule test |
| `2f29019` | Dependency-free Helper image build and Alpine-compatible command paths |
| `1983ada` | Multiport chunking for more than fifteen protected ports |

## Real IPv4 Rule Evidence

The integration test was cross-compiled for Linux and run on SSH host `安可服` in a disposable Docker network namespace. The test container used `--cap-drop ALL --cap-add NET_ADMIN`; `/proc/self/status` reported `CapEff: 0000000000001000`, which is `CAP_NET_ADMIN` only.

The tagged test applied nft-backed iptables rules for sixteen protected ports (exercising the 15-port `multiport` chunk boundary), sent 200 loopback A2S_PLAYER packets to UDP 27015, observed an initial burst followed by drops, read non-zero live PLAYER counters and one `xt_recent` attacker, disabled the policy, and confirmed all project chains were removed. A host `iptables-save` comparison before and after printed `SIXTEEN_PORT_IMAGE_RULES_HOST_UNCHANGED`.

```text
=== RUN   TestRealIPv4RulesLimitA2SPlayerFloodAndCleanUp
--- PASS: TestRealIPv4RulesLimitA2SPlayerFloodAndCleanUp
PASS
HOST_RULES_UNCHANGED
```

The host had Linux 5.15, iptables 1.8.7 (`nf_tables`), Docker 29.2.1, and loaded `xt_u32`, `xt_hashlimit`, `xt_recent`, and `xt_multiport`. The remote account had no passwordless sudo and no Go installation, so the equivalent disposable Docker network namespace was used instead of changing the host INPUT namespace. No service was deployed and no host firewall rule was added.

## Final Verification

Passed locally:

```text
go test ./internal/a2sdefense ./cmd/a2s-defense-helper -count=1
go test . -run 'Test(A2SDefenseImageBuildsOnlyItsDependencyClosure|ControlServices)' -count=1
go vet ./...
cd web && npm test -- --run                 # 19 files, 203 tests
cd web && npm run build                     # production build completed
cd web && npx playwright test -g "administrator enables A2S defense"
                                                # desktop and mobile, 2 passed
```

The Playwright journey logged in as an administrator, opened system settings, enabled defense through a mocked strict GET/PUT API boundary, confirmed effective state, ports and counters, checked document/panel horizontal overflow, and captured desktop 1463x800 and mobile 390x844 component screenshots. Both screenshots were inspected for clipping and overlap.

Passed on `安可服`:

```text
docker compose --env-file .env.example config --quiet
docker compose --env-file .env.example build a2s-defense-helper
docker run ... l4d2-a2s-defense-helper:latest iptables --version
                                                # iptables v1.8.11 (nf_tables)
tagged real-rule test in the built image         # PASS
host iptables-save comparison                    # unchanged
```

The final image built without downloading unrelated Panel modules. It contains executable `/usr/sbin/iptables`, `/usr/sbin/iptables-save`, and `/usr/sbin/iptables-restore`; the real-rule test used that image with only `CAP_NET_ADMIN`.

Full `go test -count=1 ./...` could not produce a single clean Windows exit because Windows intermittently locked Go test executables and `testing.TempDir` cleanup paths. Multiple serial reruns moved between unrelated existing packages while all A2S packages consistently passed. A remote full Linux run was also blocked before setup because the server could not reach `proxy.golang.org`; packages without external dependencies, including `internal/a2sdefense`, passed there.

The complete pre-existing Playwright journey is currently stale independently of A2S: it sends an obsolete game-log settings payload and receives HTTP 422. Temporarily updating that payload allowed it to advance until another retired `.player-capacity` selector failed; that unrelated experiment was reverted. The new A2S desktop/mobile journey passes inside the same fixture.

## Security Review

- Only `a2s-defense-helper` receives `NET_ADMIN`; Panel and game services do not.
- The Helper accepts versioned enabled/ports/revision JSON only. Executable paths, command arguments, chain names, match names, and rule text remain fixed in code.
- Apply and cleanup use project-owned `L4D2_A2S_*` chains. Tests reject INPUT flushes, policy changes, duplicate privilege, Docker socket access, published Helper ports, and host-rule side effects.
- The Unix socket is shared through a dedicated named volume with group-restricted initialization; the Helper has no database, Panel data, Docker socket, or TCP listener.
- No credential, token, host address, or production firewall output was added to the repository.
- Disabled installations retain the old lifecycle behavior. Enabled installations fail closed only before newly exposing unconfirmed instance ports.

## Residual Risk

- The fixed policy mitigates query floods but cannot prevent link saturation or reliably attribute spoofed source IPs.
- IPv6 is intentionally unsupported and untouched.
- Supported hosts must provide compatible iptables extensions; the Helper reports incompatibility and does not install or load them.
- The full repository test suite remains subject to the existing Windows file-lock flakes described above; A2S-focused, API, frontend, deployment, image, and live-rule checks passed.

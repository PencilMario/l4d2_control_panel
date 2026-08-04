#!/usr/bin/env bash

set -euo pipefail

if [[ "$(id -u)" != "0" ]]; then
  printf 'run as root: sudo scripts/test-a2s-defense-linux.sh\n' >&2
  exit 1
fi

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
namespace="l4d2-a2s-test-$$"
binary="$(mktemp /tmp/l4d2-a2s-defense-test.XXXXXX)"
before="$(iptables-save -t filter | grep 'L4D2_A2S_' || true)"

cleanup() {
  ip netns delete "$namespace" >/dev/null 2>&1 || true
  rm -f -- "$binary"
}
trap cleanup EXIT

cd "$root_dir"
go test -tags=integration -c -o "$binary" ./internal/a2sdefense
ip netns add "$namespace"
ip netns exec "$namespace" ip link set lo up
ip netns exec "$namespace" env L4D2_A2S_DEFENSE_NETNS=1 "$binary" -test.v -test.run TestRealIPv4RulesLimitA2SPlayerFloodAndCleanUp

after="$(iptables-save -t filter | grep 'L4D2_A2S_' || true)"
if [[ "$before" != "$after" ]]; then
  printf 'host firewall project chains changed during isolated test\n' >&2
  exit 1
fi

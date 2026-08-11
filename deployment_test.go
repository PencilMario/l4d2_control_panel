package deployment_test

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func TestControlServicesUseSharedUnixProxyAndPublishOnlyPanel(t *testing.T) {
	raw, err := os.ReadFile("docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	compose := string(raw)
	services := serviceBlocks(t, compose)
	proxyInit := services["proxy-init"]
	proxy := services["socket-proxy"]
	overlayHelper := services["overlay-helper"]
	a2sInit := services["a2s-defense-init"]
	a2sHelper := services["a2s-defense-helper"]
	panel := services["panel"]
	hostNetworkServices := make([]string, 0, 2)
	for name, block := range services {
		if strings.Contains(block, "network_mode: host") {
			hostNetworkServices = append(hostNetworkServices, name)
		}
	}
	slices.Sort(hostNetworkServices)
	if !slices.Equal(hostNetworkServices, []string{"a2s-defense-helper", "panel", "socket-proxy"}) {
		t.Fatalf("host networking services = %v", hostNetworkServices)
	}

	assertContains(t, proxyInit, "panel-proxy-run:/run/l4d2-panel", "proxy initializer shared run volume")
	assertContains(t, proxyInit, "chown 0:10001 /run/l4d2-panel", "proxy initializer ownership")
	assertContains(t, proxyInit, "chmod 0750 /run/l4d2-panel", "proxy initializer mode")
	assertContains(t, proxyInit, "cap_drop: [ALL]", "proxy initializer cap_drop")
	assertContains(t, proxyInit, "cap_add: [CHOWN]", "proxy initializer CHOWN-only cap_add")
	if strings.Count(proxyInit, "cap_add:") != 1 {
		t.Fatal("proxy-init must add CHOWN only")
	}

	assertContains(t, proxy, "network_mode: host", "socket-proxy host networking")
	assertContains(t, proxy, "user: \"0:10001\"", "socket-proxy runtime uid/gid")
	assertContains(t, proxy, "proxy-init:\n        condition: service_completed_successfully", "socket-proxy initializer dependency")
	assertContains(t, proxy, "cap_drop: [ALL]", "socket-proxy cap_drop")
	assertContains(t, proxy, "cap_add: [NET_RAW]", "socket-proxy NET_RAW-only cap_add")
	assertContains(t, proxy, "read_only: true", "socket-proxy read-only root")
	assertContains(t, proxy, "security_opt: [no-new-privileges:true]", "socket-proxy no-new-privileges")
	assertContains(t, proxy, "/var/run/docker.sock:/var/run/docker.sock:ro", "socket-proxy Docker socket read-only mount")
	assertContains(t, proxy, "panel-proxy-run:/run/l4d2-panel", "socket-proxy shared run volume")
	assertContains(t, proxy, "SOCKET_PATH: /run/l4d2-panel/proxy.sock", "socket-proxy Unix path")
	if strings.Count(proxy, "cap_add:") != 1 || strings.Contains(proxy, "NET_ADMIN") || strings.Contains(proxy, "SYS_ADMIN") || strings.Contains(proxy, "privileged:") || strings.Contains(proxy, "pid: host") {
		t.Fatal("socket-proxy must add NET_RAW only and receive no broad privilege")
	}

	assertContains(t, panel, "network_mode: host", "Panel host networking for loopback-only Accelerator ingress")
	if strings.Contains(panel, "/var/run/docker.sock") {
		t.Fatal("Panel must never mount the raw Docker socket")
	}
	assertContains(t, overlayHelper, "network_mode: none", "overlay helper disabled networking")
	assertContains(t, overlayHelper, "cap_drop: [ALL]", "overlay helper cap_drop")
	assertContains(t, overlayHelper, "cap_add: [SYS_ADMIN, DAC_OVERRIDE, FOWNER, MKNOD, CHOWN]", "overlay helper mount and copy-up capabilities")
	assertContains(t, overlayHelper, "apparmor:unconfined", "overlay helper AppArmor exception")
	assertContains(t, overlayHelper, "seccomp:unconfined", "overlay helper seccomp exception")
	assertContains(t, overlayHelper, "read_only: true", "overlay helper read-only root")
	assertContains(t, overlayHelper, "panel-overlay-run:/run/l4d2-panel", "overlay helper socket volume")
	assertContains(t, overlayHelper, ":rshared", "overlay helper shared mount propagation")
	if strings.Contains(panel, "SYS_ADMIN") || strings.Contains(proxy, "SYS_ADMIN") {
		t.Fatal("SYS_ADMIN must be limited to overlay-helper")
	}
	assertContains(t, a2sInit, "panel-a2s-defense-run:/run/l4d2-panel", "A2S defense socket initializer volume")
	assertContains(t, a2sInit, "cap_drop: [ALL]", "A2S defense initializer cap_drop")
	assertContains(t, a2sInit, "cap_add: [CHOWN]", "A2S defense initializer CHOWN-only capability")
	assertContains(t, a2sHelper, "network_mode: host", "A2S defense host network namespace")
	assertContains(t, a2sHelper, "user: \"0:10001\"", "A2S defense restricted group")
	assertContains(t, a2sHelper, "cap_drop: [ALL]", "A2S defense cap_drop")
	assertContains(t, a2sHelper, "cap_add: [NET_ADMIN]", "A2S defense NET_ADMIN-only capability")
	assertContains(t, a2sHelper, "read_only: true", "A2S defense read-only root")
	assertContains(t, a2sHelper, "security_opt: [no-new-privileges:true]", "A2S defense no-new-privileges")
	assertContains(t, a2sHelper, "panel-a2s-defense-run:/run/l4d2-panel", "A2S defense socket volume")
	assertContains(t, a2sHelper, "A2S_DEFENSE_HELPER_SOCKET: /run/l4d2-panel/a2s-defense.sock", "A2S defense socket path")
	assertContains(t, a2sHelper, "XTABLES_LOCKFILE: /run/l4d2-panel/xtables.lock", "writable xtables lock path")
	for _, forbidden := range []string{"/var/run/docker.sock", "privileged:", "SYS_ADMIN", "SYS_PTRACE", "ports:"} {
		if strings.Contains(a2sHelper, forbidden) {
			t.Fatalf("A2S defense helper contains forbidden privilege %q", forbidden)
		}
	}
	assertContains(t, panel, "panel-overlay-run:/run/l4d2-panel-overlay", "Panel overlay helper socket volume")
	assertContains(t, panel, "panel-a2s-defense-run:/run/l4d2-panel-a2s-defense", "Panel A2S defense socket volume")
	assertContains(t, panel, "L4D2_PANEL_A2S_DEFENSE_SOCKET: /run/l4d2-panel-a2s-defense/a2s-defense.sock", "Panel A2S defense socket path")
	assertContains(t, panel, "panel-proxy-run:/run/l4d2-panel", "Panel shared run volume")
	assertContains(t, panel, "DOCKER_HOST: unix:///run/l4d2-panel/proxy.sock", "Panel Unix Docker host")
	if strings.Contains(panel, "ports:") {
		t.Fatal("Panel host networking must not use Docker port mappings")
	}
	assertContains(t, panel, "L4D2_PANEL_LISTEN: 0.0.0.0:${L4D2_PANEL_HTTP_PORT:-18081}", "Panel host HTTP listener")
	assertContains(t, panel, "L4D2_PANEL_CRASH_REPORT_TOKEN: ${L4D2_PANEL_CRASH_REPORT_TOKEN:-}", "Panel crash report token")
	assertContains(t, panel, "L4D2_PANEL_CRASH_RETENTION_DAYS: ${L4D2_PANEL_CRASH_RETENTION_DAYS:-90}", "Panel crash report retention")
	assertContains(t, panel, "L4D2_PANEL_STACKWALK_PATH: ${L4D2_PANEL_STACKWALK_PATH:-/usr/local/bin/minidump_stackwalk}", "Panel stackwalk tool path")
	assertContains(t, panel, "L4D2_PANEL_DUMP_SYMS_PATH: ${L4D2_PANEL_DUMP_SYMS_PATH:-/usr/local/bin/dump_syms}", "Panel dump_syms tool path")
	assertContains(t, panel, "cap_add: [CHOWN]", "Panel CHOWN-only capability")
	if strings.Contains(panel, "SYS_ADMIN") || strings.Contains(panel, "NET_ADMIN") || strings.Contains(panel, "SYS_PTRACE") || strings.Contains(panel, "privileged:") {
		t.Fatal("Panel must not receive broad capabilities")
	}
	for name, block := range services {
		if name != "panel" && strings.Contains(block, "panel/crash-dumps") {
			t.Fatalf("game/control service %q must not mount panel crash reports", name)
		}
	}
	assertContains(t, panel, "L4D2_PANEL_GAME_HOST: ${L4D2_PANEL_GAME_HOST:?", "required SRCDS host")
	assertContains(t, panel, `extra_hosts:
      - "host.docker.internal:host-gateway"`, "Panel host gateway mapping for A2S")
	assertContains(t, panel, "HTTPS_PROXY: ${L4D2_PANEL_DOWNLOAD_PROXY:-}", "Panel download proxy")

	for _, retired := range []string{"LISTEN_ADDR", "23750", "tcp://socket-proxy"} {
		if strings.Contains(compose, retired) {
			t.Fatalf("retired TCP proxy configuration remains: %s", retired)
		}
	}
	if strings.Contains(proxy, "ports:") {
		t.Fatal("socket-proxy must not publish ports")
	}
	if !strings.Contains(compose, "\nvolumes:\n  panel-proxy-run:") {
		t.Fatal("Compose must define the panel-proxy-run named volume")
	}
	if !strings.Contains(compose, "  panel-a2s-defense-run:") {
		t.Fatal("Compose must define the panel-a2s-defense-run named volume")
	}
}

func TestSocketProxyImageDoesNotExposeRetiredTCPPort(t *testing.T) {
	raw, err := os.ReadFile("socket-proxy/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "EXPOSE") {
		t.Fatal("socket-proxy image must not advertise a TCP port")
	}
}

func TestPanelImageContainsConfiguredStackwalkTool(t *testing.T) {
	raw, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	dockerfile := string(raw)
	assertContains(t, dockerfile, "FROM ${ALPINE_IMAGE} AS breakpad", "Breakpad build stage")
	assertContains(t, dockerfile, "git clone --depth 1 --branch", "configurable Breakpad source")
	assertContains(t, dockerfile, "build-base ca-certificates git libtool linux-headers zlib-dev", "Breakpad zlib headers")
	assertContains(t, dockerfile, "make -j2 src/processor/minidump_stackwalk", "Breakpad stackwalk build")
	assertContains(t, dockerfile, "COPY assets/breakpad/stab.h /usr/local/include/stab.h", "Alpine STABS compatibility header")
	assertContains(t, dockerfile, "COPY --from=breakpad /src/breakpad/src/processor/minidump_stackwalk", "Breakpad stackwalk installation")
	assertContains(t, dockerfile, "minidump_stackwalk", "Breakpad stackwalk binary")
	assertContains(t, dockerfile, "libstdc++", "Breakpad runtime library")
}

func TestA2SDefenseImageBuildsOnlyItsDependencyClosure(t *testing.T) {
	raw, err := os.ReadFile("a2s-defense-helper/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	dockerfile := string(raw)
	if strings.Contains(dockerfile, "go mod download") {
		t.Fatal("A2S defense image must not download unrelated panel modules")
	}
	if strings.Contains(dockerfile, "COPY go.mod go.sum") || !strings.Contains(dockerfile, "COPY a2s-defense-helper/go.mod a2s-defense-helper/go.sum ./") {
		t.Fatal("A2S defense image must use its isolated helper module and checksum file")
	}
	module, err := os.ReadFile("a2s-defense-helper/go.mod")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(module), "github.com/florianl/go-nflog/v2 v2.3.0") {
		t.Fatal("A2S defense helper must pin the approved NFLOG dependency")
	}
	assertContains(t, dockerfile, "internal/a2sdefense/sample_window.go", "A2S defense sample window source")
	assertContains(t, dockerfile, "go build", "A2S defense helper build")
}

func serviceBlocks(t *testing.T, compose string) map[string]string {
	t.Helper()
	marker := "services:\n"
	start := strings.Index(compose, marker)
	if start < 0 {
		t.Fatal("services section not found")
	}
	services := make(map[string]string)
	var name string
	var block []string
	flush := func() {
		if name != "" {
			services[name] = strings.Join(block, "\n")
		}
	}
	for _, line := range strings.Split(compose[start+len(marker):], "\n") {
		if line == "" {
			if name != "" {
				block = append(block, line)
			}
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent == 0 {
			break
		}
		if indent == 2 && strings.HasSuffix(line, ":") {
			flush()
			name = strings.TrimSuffix(strings.TrimSpace(line), ":")
			block = nil
			continue
		}
		if name != "" {
			block = append(block, line)
		}
	}
	flush()
	for _, required := range []string{"socket-proxy", "overlay-helper", "panel"} {
		if _, ok := services[required]; !ok {
			t.Fatalf("service %q not found", required)
		}
	}
	return services
}

func assertContains(t *testing.T, block, expected, description string) {
	t.Helper()
	if !strings.Contains(block, expected) {
		t.Fatalf("missing %s", description)
	}
}

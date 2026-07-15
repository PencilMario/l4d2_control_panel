package deployment_test

import (
	"os"
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
	proxy := services["socket-proxy"]
	panel := services["panel"]
	hostNetworkServices := make([]string, 0, 1)
	for name, block := range services {
		if strings.Contains(block, "network_mode: host") {
			hostNetworkServices = append(hostNetworkServices, name)
		}
	}
	if len(hostNetworkServices) != 1 || hostNetworkServices[0] != "socket-proxy" {
		t.Fatalf("host networking services = %v, want [socket-proxy]", hostNetworkServices)
	}

	assertContains(t, proxy, "network_mode: host", "socket-proxy host networking")
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

	if strings.Contains(panel, "network_mode: host") {
		t.Fatal("Panel must not use host networking")
	}
	if strings.Contains(panel, "/var/run/docker.sock") {
		t.Fatal("Panel must never mount the raw Docker socket")
	}
	assertContains(t, panel, "panel-proxy-run:/run/l4d2-panel", "Panel shared run volume")
	assertContains(t, panel, "DOCKER_HOST: unix:///run/l4d2-panel/proxy.sock", "Panel Unix Docker host")
	assertContains(t, panel, `ports:
      - "${L4D2_PANEL_HTTP_PORT:-18081}:8080"`, "Panel published HTTP port")
	assertContains(t, panel, "L4D2_PANEL_GAME_HOST: ${L4D2_PANEL_GAME_HOST:?", "required SRCDS host")
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
	for _, required := range []string{"socket-proxy", "panel"} {
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

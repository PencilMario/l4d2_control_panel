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
	if !strings.Contains(compose, `ports:
      - "${L4D2_PANEL_HTTP_PORT:-18081}:8080"`) {
		t.Fatal("Panel must publish the configured HTTP port")
	}
	if !strings.Contains(compose, "L4D2_PANEL_GAME_HOST: ${L4D2_PANEL_GAME_HOST:?") {
		t.Fatal("Compose must require the SRCDS-reachable host address")
	}
	if !strings.Contains(compose, "panel-proxy-run:") || strings.Count(compose, "panel-proxy-run:/run/l4d2-panel") != 2 {
		t.Fatal("Panel and proxy must share the named Unix socket volume")
	}
	if !strings.Contains(compose, "network_mode: host") || strings.Count(compose, "network_mode: host") != 1 {
		t.Fatal("Only socket-proxy must use host networking")
	}
	if !strings.Contains(compose, "DOCKER_HOST: unix:///run/l4d2-panel/proxy.sock") {
		t.Fatal("Panel must reach the Docker socket over the shared Unix path")
	}
	if !strings.Contains(compose, "SOCKET_PATH: /run/l4d2-panel/proxy.sock") {
		t.Fatal("Proxy must configure its Unix socket path")
	}
	if strings.Contains(compose, "LISTEN_ADDR") || strings.Contains(compose, "23750") || strings.Contains(compose, "tcp://socket-proxy") {
		t.Fatal("TCP proxy deployment configuration must be retired")
	}
	if !strings.Contains(compose, "cap_add: [NET_RAW]") {
		t.Fatal("Proxy must add NET_RAW for capture")
	}
	for _, forbidden := range []string{"NET_ADMIN", "SYS_ADMIN", "privileged:", "pid: host"} {
		if strings.Contains(compose, forbidden) {
			t.Fatalf("Compose must not grant socket-proxy %s", forbidden)
		}
	}
	if !strings.Contains(compose, "cap_drop: [ALL]") || !strings.Contains(compose, "read_only: true") || !strings.Contains(compose, "security_opt: [no-new-privileges:true]") {
		t.Fatal("Proxy hardening must be retained")
	}
	panelSection := strings.SplitN(compose, "  panel:", 2)[1]
	if strings.Contains(panelSection, "/var/run/docker.sock") {
		t.Fatal("Panel must never mount the Docker socket")
	}
	if !strings.Contains(compose, "HTTPS_PROXY: ${L4D2_PANEL_DOWNLOAD_PROXY:-}") {
		t.Fatal("Panel GitHub downloads must use the configured download proxy")
	}
}

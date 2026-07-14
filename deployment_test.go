package deployment_test

import (
	"os"
	"strings"
	"testing"
)

func TestHostNetworkControlServicesBindLoopback(t *testing.T) {
	raw, err := os.ReadFile("docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	compose := string(raw)
	if !strings.Contains(compose, "L4D2_PANEL_LISTEN: 127.0.0.1:${L4D2_PANEL_HTTP_PORT") {
		t.Fatal("host-network Panel must bind loopback for the TLS reverse proxy")
	}
	if !strings.Contains(compose, "LISTEN_ADDR: 127.0.0.1:${L4D2_PANEL_DOCKER_PROXY_PORT") {
		t.Fatal("Docker socket proxy must bind loopback only")
	}
}

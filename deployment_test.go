package deployment_test

import (
	"os"
	"strings"
	"testing"
)

func TestControlServicesUsePrivateBridgeAndPublishOnlyPanel(t *testing.T) {
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
	if !strings.Contains(compose, "LISTEN_ADDR: 0.0.0.0:23750") {
		t.Fatal("Docker socket proxy must listen only inside the private Compose network")
	}
	if !strings.Contains(compose, "DOCKER_HOST: tcp://socket-proxy:23750") {
		t.Fatal("Panel must reach the Docker socket proxy by service name")
	}
	if !strings.Contains(compose, "HTTPS_PROXY: ${L4D2_PANEL_DOWNLOAD_PROXY:-}") {
		t.Fatal("Panel GitHub downloads must use the configured download proxy")
	}
	if strings.Count(compose, "network_mode: host") != 0 {
		t.Fatal("Compose control services must not use host networking")
	}
}

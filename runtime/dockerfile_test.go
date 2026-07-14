package runtimeimage

import (
	"os"
	"strings"
	"testing"
)

func TestDockerfileReusesSteamCMDUser(t *testing.T) {
	raw, err := os.ReadFile("Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "&& useradd -m -u 10001 steam") || !strings.Contains(text, "id -u steam") {
		t.Fatal("Dockerfile must reuse the SteamCMD image's steam user")
	}
}

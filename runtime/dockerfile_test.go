package runtimeimage

import (
	"os"
	"os/exec"
	"runtime"
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
	if !strings.Contains(text, "usermod -u 10001 steam") || !strings.Contains(text, "USER steam") {
		t.Fatal("runtime must align persistent-data UID and run SRCDS as non-root")
	}
	supervisor, _ := os.ReadFile("supervisor.py")
	if !strings.Contains(string(supervisor), "STEAM_USERNAME") || !strings.Contains(string(supervisor), "app_info_update") {
		t.Fatal("runtime must support licensed SteamCMD installs")
	}
}

func TestAnonymousFirstInstallBootstrapsWindowsBeforeLinuxValidate(t *testing.T) {
	raw, err := os.ReadFile("supervisor.py")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	windows := strings.Index(text, "'+@sSteamCmdForcePlatformType','windows'")
	linux := strings.Index(text, "'+@sSteamCmdForcePlatformType','linux'")
	if windows < 0 || linux < 0 || windows >= linux {
		t.Fatal("anonymous first install must bootstrap the Windows depot before Linux validate")
	}
	bootstrap := text[windows:linux]
	if !strings.Contains(bootstrap, "'+app_update','222860'") {
		t.Fatal("Windows bootstrap must install App 222860")
	}
	validate := text[linux:]
	if !strings.Contains(validate, "'+app_update','222860','validate'") {
		t.Fatal("Windows bootstrap must be followed by a Linux validation")
	}
}

func TestSupervisorSelfTest(t *testing.T) {
	raw, err := os.ReadFile("supervisor.py")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "def selftest") {
		t.Fatal("supervisor has no PTY self-test")
	}
	if runtime.GOOS == "windows" {
		t.Skip("PTY integration requires POSIX")
	}
	command := exec.Command("python3", "supervisor.py", "selftest")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("selftest failed: %v\n%s", err, output)
	}
}

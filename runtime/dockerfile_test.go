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

func TestSourceTVCommandIsEnabledOnlyForDeclaredPort(t *testing.T) {
	raw, err := os.ReadFile("supervisor.py")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"SRCDS_TV_PORT", "+tv_enable", "+tv_port"} {
		if !strings.Contains(string(raw), required) {
			t.Fatalf("supervisor source is missing %q", required)
		}
	}
	if runtime.GOOS == "windows" {
		t.Skip("runtime command integration requires POSIX Python")
	}
	run := func(port string) string {
		command := exec.Command("python3", "-c", `import os, supervisor; os.environ["SRCDS_TV_PORT"] = os.environ["TEST_TV_PORT"]; print(" ".join(supervisor.srcds_command()))`)
		command.Env = append(os.Environ(), "TEST_TV_PORT="+port)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("port=%s err=%v output=%s", port, err, output)
		}
		return string(output)
	}
	if output := run("0"); strings.Contains(output, "tv_enable") || strings.Contains(output, "tv_port") {
		t.Fatalf("SourceTV unexpectedly enabled: %s", output)
	}
	if output := run("27020"); !strings.Contains(output, "+tv_enable 1 +tv_port 27020") {
		t.Fatalf("SourceTV command missing: %s", output)
	}
}

func TestSupervisorPrefersValidatedJSONExtraArguments(t *testing.T) {
	raw, err := os.ReadFile("supervisor.py")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, required := range []string{"SRCDS_EXTRA_ARGS_JSON", "json.loads", "SRCDS_EXTRA_ARGS"} {
		if !strings.Contains(text, required) {
			t.Fatalf("supervisor source is missing %q", required)
		}
	}
	if runtime.GOOS == "windows" {
		t.Skip("runtime command integration requires POSIX Python")
	}
	command := exec.Command("python3", "-c", `import os, supervisor; os.environ["SRCDS_EXTRA_ARGS_JSON"]='["-strictportbind","+hostname","Night Coop"]'; print(repr(supervisor.srcds_command()[-3:]))`)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("err=%v output=%s", err, output)
	}
	if !strings.Contains(string(output), "['-strictportbind', '+hostname', 'Night Coop']") {
		t.Fatalf("output=%s", output)
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

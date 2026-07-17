package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func dockerLogFrame(stream byte, body string) []byte {
	header := make([]byte, 8)
	header[0] = stream
	binary.BigEndian.PutUint32(header[4:], uint32(len(body)))
	return append(header, []byte(body)...)
}

func TestFollowLogsDecodesMultiplexedFramesAndPartialLines(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.44/containers/abc/logs" || r.URL.Query().Get("follow") != "1" || r.URL.Query().Get("tail") != "200" {
			t.Fatalf("request=%s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
		_, _ = w.Write(bytes.Join([][]byte{
			dockerLogFrame(1, "2026-07-16T11:42:13.357043779Z first\npart"),
			dockerLogFrame(2, "warning\n"),
			dockerLogFrame(1, "ial\nlast"),
		}, nil))
	}))
	defer server.Close()
	lines := []string{}
	if err := NewEngine(server.URL).FollowLogs(context.Background(), "abc", func(line string) error {
		lines = append(lines, line)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{"first", "warning", "partial", "last"}
	if !slices.Equal(lines, want) {
		t.Fatalf("lines=%v want=%v", lines, want)
	}
}

func TestUnixEngineUsesSocketTransport(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "docker.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.44/info" || r.URL.RawQuery != "foo=bar" {
			t.Fatalf("request = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Name":"unix"}`))
	}))
	server.Listener = listener
	server.Start()
	defer server.Close()
	var out struct{ Name string }
	if err := NewEngine("unix://"+socketPath).do(context.Background(), http.MethodGet, "/info", url.Values{"foo": []string{"bar"}}, nil, &out); err != nil {
		t.Fatal(err)
	}
	if out.Name != "unix" {
		t.Fatalf("name = %q", out.Name)
	}
}

func TestUnixEngineAttachSupervisorUsesSocketTransport(t *testing.T) {
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("l4d2-attach-%d.sock", time.Now().UnixNano()))
	t.Cleanup(func() { _ = os.Remove(socketPath) })
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/exec") {
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": "exec-attach"})
			return
		}
		if r.URL.Path != "/v1.44/exec/exec-attach/start" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		hijacker := w.(http.Hijacker)
		conn, rw, err := hijacker.Hijack()
		if err != nil {
			t.Fatal(err)
		}
		_, _ = rw.WriteString("HTTP/1.1 101 UPGRADED\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
		_ = rw.Flush()
		go func() {
			defer conn.Close()
			line, _ := bufio.NewReader(conn).ReadString('\n')
			_, _ = io.WriteString(conn, "echo:"+line)
		}()
	}))
	server.Listener = listener
	server.Start()
	defer server.Close()

	stream, err := NewEngine("unix://"+socketPath).AttachSupervisor(context.Background(), "container-1")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	_, _ = io.WriteString(stream, "status\n")
	raw := make([]byte, len("echo:status\n"))
	if _, err := io.ReadFull(stream, raw); err != nil {
		t.Fatal(err)
	}
	if string(raw) != "echo:status\n" {
		t.Fatalf("got %q", raw)
	}
}

func TestEngineCreatesRestrictedManagedContainer(t *testing.T) {
	var got createRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.44/containers/create" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.URL.Query().Get("name") != "l4d2-abc" {
			t.Fatalf("name=%s", r.URL.Query().Get("name"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"Id": "container-1"})
	}))
	defer server.Close()
	client := NewEngine(server.URL, WithDownloadProxy("http://proxy:7890"), WithSteamCredentials(func() (string, string) { return "owner", "password" }))
	spec, err := BuildContainerSpec("/srv/l4d2-panel", domain.Instance{ID: "abc", RuntimeImage: "runtime:v1"})
	if err != nil {
		t.Fatal(err)
	}
	id, err := client.Create(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if id != "container-1" || got.HostConfig.NetworkMode != "host" || got.Labels[ManagedLabel] != "true" || got.HostConfig.Privileged {
		t.Fatalf("unsafe request: %#v", got)
	}
	if !slices.Contains(got.Env, "HTTPS_PROXY=http://proxy:7890") {
		t.Fatalf("env=%v", got.Env)
	}
	if !slices.Contains(got.Env, "STEAM_USERNAME=owner") || !slices.Contains(got.Env, "STEAM_PASSWORD=password") {
		t.Fatalf("steam env=%v", got.Env)
	}
}

func TestAttachSupervisorHijacksFixedExecStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/exec") {
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": "exec-attach"})
			return
		}
		if r.URL.Path != "/v1.44/exec/exec-attach/start" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		hijacker := w.(http.Hijacker)
		conn, rw, err := hijacker.Hijack()
		if err != nil {
			t.Fatal(err)
		}
		_, _ = rw.WriteString("HTTP/1.1 101 UPGRADED\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
		_ = rw.Flush()
		go func() {
			defer conn.Close()
			line, _ := bufio.NewReader(conn).ReadString('\n')
			_, _ = io.WriteString(conn, "echo:"+line)
		}()
	}))
	defer server.Close()
	stream, err := NewEngine(server.URL).AttachSupervisor(context.Background(), "container-1")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	_, _ = io.WriteString(stream, "status\n")
	raw := make([]byte, len("echo:status\n"))
	if _, err := io.ReadFull(stream, raw); err != nil {
		t.Fatal(err)
	}
	if string(raw) != "echo:status\n" {
		t.Fatalf("got %q", raw)
	}
}

func TestPlayerCommandRejectsArbitraryConsoleInput(t *testing.T) {
	if err := NewEngine("http://127.0.0.1:1").PlayerCommand(context.Background(), "container", "quit"); err == nil {
		t.Fatal("arbitrary console command accepted")
	}
}

func TestExtractConsoleResponseIgnoresHistoryAndEchoedMarkerCommands(t *testing.T) {
	raw := "old status\n# 9 1 \"Old Player\" STEAM_1:0:1 active\r\n" +
		"echo START_MARKER\r\nstatus\r\necho END_MARKER\r\n" +
		"START_MARKER\r\n# 2 1 \"Sir.P\" STEAM_1:0:526095818 active\r\nEND_MARKER\r\nnew noise\n"

	response, ok := extractConsoleResponse(raw, "START_MARKER", "END_MARKER")
	if !ok || response != "# 2 1 \"Sir.P\" STEAM_1:0:526095818 active" {
		t.Fatalf("ok=%v response=%q", ok, response)
	}
}

func TestGameUpdateUsesFixedSteamCMDMaintenanceContainer(t *testing.T) {
	var created createRequest
	var paths []string
	root := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
			_ = json.NewEncoder(w).Encode([]Container{})
		case strings.HasSuffix(r.URL.Path, "/containers/create"):
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": "maintenance"})
		case strings.HasSuffix(r.URL.Path, "/wait"):
			_ = json.NewEncoder(w).Encode(map[string]int{"StatusCode": 0})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	instance := domain.Instance{ID: "abc", RuntimeImage: "runtime:v1"}
	if err := NewEngine(server.URL).UpdateGame(context.Background(), root, instance); err != nil {
		t.Fatal(err)
	}
	if strings.Join(created.Entrypoint, " ") != "/home/steam/steamcmd/steamcmd.sh" || strings.Join(created.Cmd, " ") != "+@sSteamCmdForcePlatformType linux +force_install_dir /opt/l4d2/game +login anonymous +app_info_update 1 +app_update 222860 validate +quit" || created.HostConfig.NetworkMode != "bridge" || created.Labels[RoleLabel] != "maintenance" {
		t.Fatalf("request=%#v", created)
	}
	if len(paths) != 6 || !slices.Contains(paths, "GET /v1.44/containers/maintenance/logs") {
		t.Fatalf("paths=%v", paths)
	}
	wantCache := filepath.Join(root, "panel", "steamcmd", "Steam") + ":/home/steam/Steam"
	if !slices.Contains(created.HostConfig.Binds, wantCache) {
		t.Fatalf("binds=%v want cache bind %q", created.HostConfig.Binds, wantCache)
	}
}

func TestRunMaintenanceReturnsExitCodeAndRecentOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
			_ = json.NewEncoder(w).Encode([]Container{})
		case strings.HasSuffix(r.URL.Path, "/containers/create"):
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": "maintenance"})
		case strings.HasSuffix(r.URL.Path, "/logs"):
			w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
			_, _ = w.Write(dockerLogFrame(2, "ERROR! Failed to install app '222860' (Missing configuration)\n"))
		case strings.HasSuffix(r.URL.Path, "/wait"):
			_ = json.NewEncoder(w).Encode(map[string]int{"StatusCode": 8})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	command := []string{steamCMDEntrypoint, "+login", "anonymous", "+quit"}
	result, err := NewEngine(server.URL).runMaintenance(context.Background(), t.TempDir(), domain.Instance{ID: "abc", RuntimeImage: "runtime:v1"}, command)
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != 8 || !strings.Contains(result.Output, "Missing configuration") {
		t.Fatalf("result=%#v", result)
	}
}

func TestGameUpdateOverridesRuntimeEntrypointWithSteamCMD(t *testing.T) {
	var created struct {
		Entrypoint []string `json:"Entrypoint"`
		Cmd        []string `json:"Cmd"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
			_ = json.NewEncoder(w).Encode([]Container{})
		case strings.HasSuffix(r.URL.Path, "/containers/create"):
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": "maintenance"})
		case strings.HasSuffix(r.URL.Path, "/wait"):
			_ = json.NewEncoder(w).Encode(map[string]int{"StatusCode": 0})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	instance := domain.Instance{ID: "abc", RuntimeImage: "runtime:v1"}
	if err := NewEngine(server.URL).UpdateGame(context.Background(), t.TempDir(), instance); err != nil {
		t.Fatal(err)
	}
	if strings.Join(created.Entrypoint, " ") != "/home/steam/steamcmd/steamcmd.sh" {
		t.Fatalf("entrypoint=%v", created.Entrypoint)
	}
	if len(created.Cmd) == 0 || created.Cmd[0] == "steamcmd" {
		t.Fatalf("cmd=%v", created.Cmd)
	}
}

func TestGameInstallBootstrapsWindowsBeforeLinuxWithoutValidate(t *testing.T) {
	var created createRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
			_ = json.NewEncoder(w).Encode([]Container{})
		case strings.HasSuffix(r.URL.Path, "/containers/create"):
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": "maintenance"})
		case strings.HasSuffix(r.URL.Path, "/wait"):
			_ = json.NewEncoder(w).Encode(map[string]int{"StatusCode": 0})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	instance := domain.Instance{ID: "abc", RuntimeImage: "runtime:v1"}
	if err := NewEngine(server.URL).InstallGame(context.Background(), t.TempDir(), instance); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(created.Cmd, " ")
	windows := strings.Index(joined, "+@sSteamCmdForcePlatformType windows")
	linux := strings.Index(joined, "+@sSteamCmdForcePlatformType linux")
	if windows < 0 || linux < 0 || windows >= linux || !strings.Contains(joined, "+login anonymous +app_info_update 1 +app_update 222860 +@sSteamCmdForcePlatformType linux +app_update 222860 +quit") || strings.Contains(joined, "validate") {
		t.Fatalf("command=%q", joined)
	}
}

func TestAnonymousInstallRetriesMissingConfigurationThenSucceeds(t *testing.T) {
	createCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
			_ = json.NewEncoder(w).Encode([]Container{})
		case strings.HasSuffix(r.URL.Path, "/containers/create"):
			createCount++
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": fmt.Sprintf("maintenance-%d", createCount)})
		case strings.HasSuffix(r.URL.Path, "/logs"):
			w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
			if strings.Contains(r.URL.Path, "maintenance-1") {
				_, _ = w.Write(dockerLogFrame(2, "ERROR! Failed to install app '222860' (Missing configuration)\n"))
			}
		case strings.HasSuffix(r.URL.Path, "/wait"):
			status := 0
			if strings.Contains(r.URL.Path, "maintenance-1") {
				status = 8
			}
			_ = json.NewEncoder(w).Encode(map[string]int{"StatusCode": status})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	err := NewEngine(server.URL).InstallGame(context.Background(), t.TempDir(), domain.Instance{ID: "abc", RuntimeImage: "runtime:v1"})
	if err != nil {
		t.Fatal(err)
	}
	if createCount != 2 {
		t.Fatalf("createCount=%d", createCount)
	}
}

func TestAnonymousInstallDoesNotRetryOtherFailure(t *testing.T) {
	createCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
			_ = json.NewEncoder(w).Encode([]Container{})
		case strings.HasSuffix(r.URL.Path, "/containers/create"):
			createCount++
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": "maintenance"})
		case strings.HasSuffix(r.URL.Path, "/logs"):
			w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
			_, _ = w.Write(dockerLogFrame(2, "ERROR! Disk write failure\n"))
		case strings.HasSuffix(r.URL.Path, "/wait"):
			_ = json.NewEncoder(w).Encode(map[string]int{"StatusCode": 8})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	err := NewEngine(server.URL).InstallGame(context.Background(), t.TempDir(), domain.Instance{ID: "abc", RuntimeImage: "runtime:v1"})
	if err == nil || !strings.Contains(err.Error(), "steamcmd exited with code 8") {
		t.Fatalf("err=%v", err)
	}
	if createCount != 1 {
		t.Fatalf("createCount=%d", createCount)
	}
}

func TestAnonymousInstallDoesNotRetryDockerFailure(t *testing.T) {
	createCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
			_ = json.NewEncoder(w).Encode([]Container{})
		case strings.HasSuffix(r.URL.Path, "/containers/create"):
			createCount++
			http.Error(w, "docker unavailable", http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	err := NewEngine(server.URL).InstallGame(context.Background(), t.TempDir(), domain.Instance{ID: "abc", RuntimeImage: "runtime:v1"})
	if err == nil || !strings.Contains(err.Error(), "docker unavailable") {
		t.Fatalf("err=%v", err)
	}
	if createCount != 1 {
		t.Fatalf("createCount=%d", createCount)
	}
}

func TestAnonymousInstallStopsAfterThreeMissingConfigurationFailures(t *testing.T) {
	createCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
			_ = json.NewEncoder(w).Encode([]Container{})
		case strings.HasSuffix(r.URL.Path, "/containers/create"):
			createCount++
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": fmt.Sprintf("maintenance-%d", createCount)})
		case strings.HasSuffix(r.URL.Path, "/logs"):
			w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
			_, _ = w.Write(dockerLogFrame(2, "ERROR! Failed to install app '222860' (Missing configuration)\n"))
		case strings.HasSuffix(r.URL.Path, "/wait"):
			_ = json.NewEncoder(w).Encode(map[string]int{"StatusCode": 8})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	err := NewEngine(server.URL).InstallGame(context.Background(), t.TempDir(), domain.Instance{ID: "abc", RuntimeImage: "runtime:v1"})
	if err == nil || !strings.Contains(err.Error(), "after 3 attempts") || !strings.Contains(err.Error(), "code 8") {
		t.Fatalf("err=%v", err)
	}
	if createCount != 3 {
		t.Fatalf("createCount=%d", createCount)
	}
}

func TestCredentialedGameInstallDoesNotValidate(t *testing.T) {
	var created createRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
			_ = json.NewEncoder(w).Encode([]Container{})
		case strings.HasSuffix(r.URL.Path, "/containers/create"):
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": "maintenance"})
		case strings.HasSuffix(r.URL.Path, "/wait"):
			_ = json.NewEncoder(w).Encode(map[string]int{"StatusCode": 0})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	engine := NewEngine(server.URL, WithSteamCredentials(func() (string, string) { return "owner", "password" }))
	instance := domain.Instance{ID: "abc", RuntimeImage: "runtime:v1"}
	if err := engine.InstallGame(context.Background(), t.TempDir(), instance); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(created.Cmd, " ")
	if joined != "+@sSteamCmdForcePlatformType linux +force_install_dir /opt/l4d2/game +login owner password +app_info_update 1 +app_update 222860 +quit" || strings.Contains(joined, "validate") {
		t.Fatalf("command=%q", joined)
	}
}

func TestGameUpdateAdoptsExistingMaintenanceContainer(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
			_ = json.NewEncoder(w).Encode([]Container{{ID: "maintenance-existing", State: "running", Labels: map[string]string{ManagedLabel: "true", InstanceLabel: "abc", RoleLabel: "maintenance"}}})
		case strings.HasSuffix(r.URL.Path, "/wait"):
			_ = json.NewEncoder(w).Encode(map[string]int{"StatusCode": 0})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	instance := domain.Instance{ID: "abc", RuntimeImage: "runtime:v1"}
	if err := NewEngine(server.URL).UpdateGame(context.Background(), t.TempDir(), instance); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if path == "POST /v1.44/containers/create" || path == "POST /v1.44/containers/maintenance-existing/start" {
			t.Fatalf("existing maintenance container was not adopted: %v", paths)
		}
	}
	for _, want := range []string{"GET /v1.44/containers/json", "POST /v1.44/containers/maintenance-existing/wait", "GET /v1.44/containers/maintenance-existing/logs", "DELETE /v1.44/containers/maintenance-existing"} {
		if !slices.Contains(paths, want) {
			t.Fatalf("paths=%v missing=%s", paths, want)
		}
	}
	if paths[len(paths)-1] != "DELETE /v1.44/containers/maintenance-existing" {
		t.Fatalf("container deleted before logs drained: %v", paths)
	}
}

func TestGameUpdateKeepsUnclassifiedMaintenanceContainerForRetry(t *testing.T) {
	var deleted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
			_ = json.NewEncoder(w).Encode([]Container{})
		case strings.HasSuffix(r.URL.Path, "/containers/create"):
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": "maintenance-new"})
		case strings.HasSuffix(r.URL.Path, "/wait"):
			http.Error(w, "interrupted", http.StatusServiceUnavailable)
		case r.Method == http.MethodDelete:
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	err := NewEngine(server.URL).UpdateGame(context.Background(), t.TempDir(), domain.Instance{ID: "abc", RuntimeImage: "runtime:v1"})
	if err == nil {
		t.Fatal("interrupted wait unexpectedly succeeded")
	}
	if deleted {
		t.Fatal("unclassified maintenance container was deleted instead of retained for adoption")
	}
}

func TestStatsCalculatesCPUAndMemory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("stream") != "false" || r.URL.Query().Has("one-shot") {
			t.Fatalf("query=%s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"cpu_stats":    map[string]any{"cpu_usage": map[string]any{"total_usage": 300.0, "percpu_usage": []int{1, 2}}, "system_cpu_usage": 1000.0},
			"precpu_stats": map[string]any{"cpu_usage": map[string]any{"total_usage": 100.0}, "system_cpu_usage": 500.0},
			"memory_stats": map[string]any{"usage": 1073741824.0, "limit": 2147483648.0, "stats": map[string]any{"cache": 268435456.0}},
			"blkio_stats": map[string]any{"io_service_bytes_recursive": []map[string]any{
				{"op": "Read", "value": 1024.0},
				{"op": "read", "value": 2048.0},
				{"op": "WRITE", "value": 4096.0},
				{"op": "Write", "value": 8192.0},
				{"op": "Sync", "value": 16384.0},
			}},
			"pids_stats": map[string]any{"current": 17.0},
		})
	}))
	defer server.Close()
	stats, err := NewEngine(server.URL).Stats(context.Background(), "container")
	if err != nil {
		t.Fatal(err)
	}
	if stats.CPUPercent != 80 || stats.MemoryBytes != 805306368 || stats.MemoryLimitBytes != 2147483648 || stats.BlockReadBytes != 3072 || stats.BlockWriteBytes != 12288 || stats.PIDs != 17 {
		t.Fatalf("stats=%#v", stats)
	}
}

func TestRuntimeParsesRunningAndStartedAt(t *testing.T) {
	startedAt := "2026-07-15T08:09:10.123456789Z"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/v1.44/containers/container%2Fname/json" {
			t.Fatalf("path=%s", r.URL.EscapedPath())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"State": map[string]any{"Running": true, "StartedAt": startedAt}})
	}))
	defer server.Close()

	runtime, err := NewEngine(server.URL).Runtime(context.Background(), "container/name")
	if err != nil {
		t.Fatal(err)
	}
	wantStartedAt, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !runtime.Running || !runtime.StartedAt.Equal(wantStartedAt) {
		t.Fatalf("runtime=%#v", runtime)
	}
}

func TestImageSize(t *testing.T) {
	t.Run("returns inspected image size", func(t *testing.T) {
		var paths []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.URL.EscapedPath())
			switch r.URL.EscapedPath() {
			case "/v1.44/containers/container%2Fname/json":
				_ = json.NewEncoder(w).Encode(map[string]any{"Image": "sha256:image"})
			case "/v1.44/images/sha256:image/json":
				_ = json.NewEncoder(w).Encode(map[string]any{"Size": uint64(5368709120)})
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()

		size, err := NewEngine(server.URL).ImageSize(context.Background(), "container/name")
		if err != nil || size != 5368709120 {
			t.Fatalf("size=%d err=%v", size, err)
		}
		want := []string{"/v1.44/containers/container%2Fname/json", "/v1.44/images/sha256:image/json"}
		if strings.Join(paths, "|") != strings.Join(want, "|") {
			t.Fatalf("paths=%v", paths)
		}
	})

	t.Run("rejects empty container image", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"Image": ""})
		}))
		defer server.Close()

		if _, err := NewEngine(server.URL).ImageSize(context.Background(), "container"); err == nil {
			t.Fatal("expected empty image error")
		}
	})

	t.Run("returns image inspect failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/containers/") {
				_ = json.NewEncoder(w).Encode(map[string]any{"Image": "sha256:image"})
				return
			}
			http.Error(w, "inspect failed", http.StatusInternalServerError)
		}))
		defer server.Close()

		if _, err := NewEngine(server.URL).ImageSize(context.Background(), "container"); err == nil {
			t.Fatal("expected image inspect error")
		}
	})
}

func TestPingReadsDockerInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1.44/info" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ServerVersion": "29.0", "ContainersRunning": 3})
	}))
	defer server.Close()
	info, err := NewEngine(server.URL).Info(context.Background())
	if err != nil || info.ServerVersion != "29.0" || info.ContainersRunning != 3 {
		t.Fatalf("info=%#v err=%v", info, err)
	}
}

func TestStopTreatsAlreadyStoppedContainerAsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1.44/containers/container-1/stop" {
			t.Fatalf("request=%s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	if err := NewEngine(server.URL).Stop(context.Background(), "container-1", 15); err != nil {
		t.Fatal(err)
	}
}

func TestStopPropagatesDockerFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "proxy failed", http.StatusBadGateway)
	}))
	defer server.Close()

	err := NewEngine(server.URL).Stop(context.Background(), "container-1", 15)
	if err == nil || !strings.Contains(err.Error(), "502 Bad Gateway") {
		t.Fatalf("err=%v", err)
	}
}

func TestEngineUsesOnlyFixedLifecycleEndpoints(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/exec") {
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": "exec-1"})
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	c := NewEngine(server.URL)
	ctx := context.Background()
	if err := c.Start(ctx, "container-1"); err != nil {
		t.Fatal(err)
	}
	if err := c.Stop(ctx, "container-1", 15); err != nil {
		t.Fatal(err)
	}
	if err := c.Remove(ctx, "container-1"); err != nil {
		t.Fatal(err)
	}
	id, err := c.CreateSupervisorExec(ctx, "container-1", "status")
	if err != nil || id != "exec-1" {
		t.Fatalf("id=%s err=%v", id, err)
	}
	want := []string{"POST /v1.44/containers/container-1/start", "POST /v1.44/containers/container-1/stop", "DELETE /v1.44/containers/container-1", "POST /v1.44/containers/container-1/exec"}
	if strings.Join(paths, "|") != strings.Join(want, "|") {
		t.Fatalf("paths=%v", paths)
	}
}

func TestListManagedIncludesGameAndMaintenanceRoles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var filters map[string][]string
		if err := json.Unmarshal([]byte(r.URL.Query().Get("filters")), &filters); err != nil {
			t.Fatalf("filters: %v", err)
		}
		if got := strings.Join(filters["label"], ","); got != ManagedLabel+"=true" {
			t.Fatalf("role-specific filter hides maintenance containers: %q", got)
		}
		_ = json.NewEncoder(w).Encode([]Container{
			{ID: "game", Labels: map[string]string{ManagedLabel: "true", InstanceLabel: "abc", RoleLabel: "game"}, State: "running"},
			{ID: "maintenance", Labels: map[string]string{ManagedLabel: "true", InstanceLabel: "abc", RoleLabel: "maintenance"}, State: "running"},
		})
	}))
	defer server.Close()
	items, err := NewEngine(server.URL).ListManaged(context.Background())
	if err != nil || len(items) != 2 || items[0].InstanceID() != "abc" || items[1].Labels[RoleLabel] != "maintenance" {
		t.Fatalf("items=%#v err=%v", items, err)
	}
}

package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

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
	id, err := client.Create(context.Background(), BuildContainerSpec("/srv/l4d2-panel", domain.Instance{ID: "abc", RuntimeImage: "runtime:v1"}))
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

func TestGameUpdateUsesFixedSteamCMDMaintenanceContainer(t *testing.T) {
	var created createRequest
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		switch {
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
	if strings.Join(created.Cmd, " ") != "steamcmd +@sSteamCmdForcePlatformType linux +force_install_dir /opt/l4d2/game +login anonymous +app_info_update 1 +app_update 222860 validate +quit" || created.HostConfig.NetworkMode != "bridge" || created.Labels[RoleLabel] != "maintenance" {
		t.Fatalf("request=%#v", created)
	}
	if len(paths) != 4 {
		t.Fatalf("paths=%v", paths)
	}
}

func TestStatsCalculatesCPUAndMemory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"cpu_stats": map[string]any{"cpu_usage": map[string]any{"total_usage": 300.0, "percpu_usage": []int{1, 2}}, "system_cpu_usage": 1000.0}, "precpu_stats": map[string]any{"cpu_usage": map[string]any{"total_usage": 100.0}, "system_cpu_usage": 500.0}, "memory_stats": map[string]any{"usage": 1073741824.0, "stats": map[string]any{"cache": 268435456.0}}})
	}))
	defer server.Close()
	stats, err := NewEngine(server.URL).Stats(context.Background(), "container")
	if err != nil {
		t.Fatal(err)
	}
	if stats.CPUPercent != 80 || stats.MemoryBytes != 805306368 {
		t.Fatalf("stats=%#v", stats)
	}
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

func TestListManagedFiltersByLabels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "filters=") {
			t.Fatal("missing label filters")
		}
		_ = json.NewEncoder(w).Encode([]Container{{ID: "one", Labels: map[string]string{ManagedLabel: "true", InstanceLabel: "abc"}, State: "running"}})
	}))
	defer server.Close()
	items, err := NewEngine(server.URL).ListManaged(context.Background())
	if err != nil || len(items) != 1 || items[0].InstanceID() != "abc" {
		t.Fatalf("items=%#v err=%v", items, err)
	}
}

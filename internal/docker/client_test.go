package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"io"
	"net/http"
	"net/http/httptest"
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
	client := NewEngine(server.URL)
	id, err := client.Create(context.Background(), BuildContainerSpec("/srv/l4d2-panel", domain.Instance{ID: "abc", RuntimeImage: "runtime:v1"}))
	if err != nil {
		t.Fatal(err)
	}
	if id != "container-1" || got.HostConfig.NetworkMode != "host" || got.Labels[ManagedLabel] != "true" || got.HostConfig.Privileged {
		t.Fatalf("unsafe request: %#v", got)
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

package socketproxy

import (
	"net/http"
	"net/url"
	"testing"
)

func TestPolicyAllowsRequiredDockerEndpointsOnly(t *testing.T) {
	allowed := [][2]string{{"GET", "/v1.44/info"}, {"GET", "/v1.44/containers/json"}, {"POST", "/v1.44/containers/create"}, {"POST", "/v1.44/containers/abc/start"}, {"POST", "/v1.44/containers/abc/exec"}, {"POST", "/v1.44/exec/id/start"}, {"DELETE", "/v1.44/containers/abc"}, {"GET", "/v1.44/containers/abc/stats"}}
	for _, item := range allowed {
		if !Allowed(item[0], item[1]) {
			t.Fatalf("required endpoint denied: %v", item)
		}
	}
	denied := [][2]string{{"GET", "/v1.44/volumes"}, {"POST", "/v1.44/containers/abc/archive"}, {"POST", "/v1.44/networks/create"}, {"DELETE", "/v1.44/images/base"}, {"GET", "/v1.44/system/df"}, {"GET", "/_panel/traffic/instance-1"}, {"PUT", "/_panel/traffic/instance-1"}, {"DELETE", "/_panel/traffic/instance-1"}}
	for _, item := range denied {
		if Allowed(item[0], item[1]) {
			t.Fatalf("dangerous endpoint allowed: %v", item)
		}
	}
}

func TestAllowedArchiveEndpointRequiresSystemLibraryPath(t *testing.T) {
	for _, path := range []string{
		"/v1.44/containers/game/archive",
		"/containers/game/archive",
	} {
		if !Allowed(http.MethodGet, path) {
			t.Fatalf("archive endpoint denied: %s", path)
		}
	}
	for _, query := range []url.Values{
		{"path": {"/lib/i386-linux-gnu/libc.so.6"}},
		{"path": {"/usr/lib/x86_64-linux-gnu/libstdc++.so.6"}},
	} {
		if !AllowedArchiveQuery(query) {
			t.Fatalf("safe archive query denied: %v", query)
		}
	}
	for _, query := range []url.Values{
		{"path": {"/etc/passwd"}},
		{"path": {"/lib/../etc/passwd"}},
		{"path": {"/lib/libc.so.6"}, "foo": {"bar"}},
	} {
		if AllowedArchiveQuery(query) {
			t.Fatalf("unsafe archive query accepted: %v", query)
		}
	}
}

func TestLogQueryRequiresFixedSafeOptions(t *testing.T) {
	allowed := url.Values{"stdout": {"1"}, "stderr": {"1"}, "follow": {"1"}, "timestamps": {"1"}, "tail": {"200"}}
	if !AllowedLogQuery(allowed) {
		t.Fatalf("safe query denied: %v", allowed)
	}
	for _, query := range []url.Values{
		{"stdout": {"1"}},
		{"stdout": {"1"}, "stderr": {"1"}, "follow": {"0"}, "timestamps": {"1"}, "tail": {"200"}},
		{"stdout": {"1"}, "stderr": {"1"}, "follow": {"1"}, "timestamps": {"1"}, "tail": {"all"}},
		{"stdout": {"1"}, "stderr": {"1"}, "follow": {"1"}, "timestamps": {"1"}, "tail": {"200"}, "since": {"0"}},
	} {
		if AllowedLogQuery(query) {
			t.Fatalf("unsafe query allowed: %v", query)
		}
	}
}

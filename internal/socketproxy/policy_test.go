package socketproxy

import "testing"

func TestPolicyAllowsRequiredDockerEndpointsOnly(t *testing.T) {
	allowed := [][2]string{{"GET", "/v1.44/info"}, {"GET", "/v1.44/containers/json"}, {"POST", "/v1.44/containers/create"}, {"POST", "/v1.44/containers/abc/start"}, {"POST", "/v1.44/containers/abc/exec"}, {"POST", "/v1.44/exec/id/start"}, {"DELETE", "/v1.44/containers/abc"}, {"GET", "/v1.44/containers/abc/stats"}}
	for _, item := range allowed {
		if !Allowed(item[0], item[1]) {
			t.Fatalf("required endpoint denied: %v", item)
		}
	}
	denied := [][2]string{{"GET", "/v1.44/volumes"}, {"POST", "/v1.44/containers/abc/archive"}, {"POST", "/v1.44/networks/create"}, {"DELETE", "/v1.44/images/base"}, {"GET", "/v1.44/system/df"}}
	for _, item := range denied {
		if Allowed(item[0], item[1]) {
			t.Fatalf("dangerous endpoint allowed: %v", item)
		}
	}
}

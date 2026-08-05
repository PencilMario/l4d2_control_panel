package a2sdefense

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

type fakeFirewall struct {
	config Config
	status Status
	err    error
}

func (f *fakeFirewall) Apply(_ context.Context, config Config) (Status, error) {
	f.config = config
	return f.status, f.err
}

func (f *fakeFirewall) Status(context.Context) (Status, error) { return f.status, f.err }

func TestServerPutConfigUsesStrictJSONAndReturnsStatus(t *testing.T) {
	firewall := &fakeFirewall{status: Status{Compatible: true, Enabled: true, Revision: 2, PolicyVersion: PolicyVersion, Ports: []int{27015}}}
	server := NewServer(firewall)
	request := httptest.NewRequest(http.MethodPut, "/v1/config", strings.NewReader(`{"version":1,"enabled":true,"ports":[27015],"revision":2}`))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || firewall.config.Revision != 2 || !strings.Contains(response.Body.String(), `"policy_version":1`) {
		t.Fatalf("status=%d config=%+v body=%s", response.Code, firewall.config, response.Body.String())
	}
	for _, body := range []string{
		`{"version":1,"enabled":true,"ports":[27015],"revision":2,"command":"flush"}`,
		`{"version":1,"enabled":true,"ports":[27015],"revision":2}{}`,
	} {
		response = httptest.NewRecorder()
		server.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/v1/config", strings.NewReader(body)))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d response=%s", body, response.Code, response.Body.String())
		}
	}
}

func TestServerGetStatusAndErrorMapping(t *testing.T) {
	firewall := &fakeFirewall{status: Status{Compatible: true, PolicyVersion: PolicyVersion}}
	server := NewServer(firewall)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/status", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"compatible":true`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	firewall.err = ErrStaleRevision
	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/v1/config", strings.NewReader(`{"version":1,"enabled":false,"ports":[],"revision":1}`)))
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	firewall.err = errors.New("iptables unavailable")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/status", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestServerRejectsOtherMethodsAndPaths(t *testing.T) {
	server := NewServer(&fakeFirewall{})
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/v1/config", nil),
		httptest.NewRequest(http.MethodGet, "/v1/config", nil),
		httptest.NewRequest(http.MethodGet, "/v1/missing", nil),
	} {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != http.StatusMethodNotAllowed && response.Code != http.StatusNotFound {
			t.Fatalf("%s %s status=%d", request.Method, request.URL.Path, response.Code)
		}
	}
}

func TestServerReturnsStrictEventCursorBatch(t *testing.T) {
	ring := NewEventRing("boot-a", 4)
	if err := ring.Add(Event{Source: netip.MustParseAddr("192.0.2.3"), DestinationPort: 27015, Query: QueryPlayer}); err != nil {
		t.Fatal(err)
	}
	server := NewServer(&fakeFirewall{}, ring)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/events?boot=boot-a&after=0", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"query":"A2S_PLAYER"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	for _, target := range []string{"/v1/events?after=-1", "/v1/events?after=1&extra=x", "/v1/events?after=1&after=2"} {
		response = httptest.NewRecorder()
		server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("target=%s status=%d", target, response.Code)
		}
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/events", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status=%d", response.Code)
	}
}

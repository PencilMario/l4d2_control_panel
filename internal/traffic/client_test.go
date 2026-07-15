package traffic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerAndClientContract(t *testing.T) {
	counter := NewCounter()
	server := httptest.NewServer(NewHandler(counter, nil))
	t.Cleanup(server.Close)
	client := NewClient(server.URL, server.Client())
	ctx := context.Background()

	if err := client.Register(ctx, Session{InstanceID: "instance-1", RunID: "run-1", Ports: []int{27015}}); err != nil {
		t.Fatal(err)
	}
	counter.Observe(Packet{Length: 42, DstPort: 27015})
	got, err := client.Totals(ctx, "instance-1")
	if err != nil {
		t.Fatal(err)
	}
	if got != (Totals{RunID: "run-1", RXBytes: 42}) {
		t.Fatalf("Totals() = %+v", got)
	}
	if err := client.Stop(ctx, "instance-1", "run-1"); err != nil {
		t.Fatal(err)
	}
}

func TestNewClientEnforcesBoundedTimeout(t *testing.T) {
	client := NewClient("http://socket-proxy", &http.Client{})
	if client.http.Timeout <= 0 {
		t.Fatal("NewClient accepted an unbounded HTTP client")
	}
}

func TestClientDrainsErrorResponseForConnectionReuse(t *testing.T) {
	body := bytes.NewReader(bytes.Repeat([]byte("x"), 8192))
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Status:     "502 Bad Gateway",
			Header:     make(http.Header),
			Body:       io.NopCloser(body),
		}, nil
	})}
	_, err := NewClient("http://socket-proxy", httpClient).Totals(context.Background(), "instance-1")
	if err == nil {
		t.Fatal("Totals succeeded for an error response")
	}
	if body.Len() != 0 {
		t.Fatalf("error response retained %d unread bytes", body.Len())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestHandlerRejectsMismatchedIDUnknownRunAndBadBodies(t *testing.T) {
	counter := NewCounter()
	handler := NewHandler(counter, nil)
	tests := []struct {
		method, path, body string
		status             int
	}{
		{http.MethodPut, "/_panel/traffic/path-id", `{"instance_id":"body-id","run_id":"run-1","ports":[27015]}`, http.StatusConflict},
		{http.MethodPut, "/_panel/traffic/instance-1", `{"instance_id":"instance-1","run_id":"run-1","ports":[27015]} trailing`, http.StatusBadRequest},
		{http.MethodPut, "/_panel/traffic/instance-1", `{"instance_id":"instance-1","run_id":"run-1","ports":[27015],"secret":"x"}`, http.StatusBadRequest},
		{http.MethodDelete, "/_panel/traffic/missing", `{"run_id":"run-1"}`, http.StatusNotFound},
		{http.MethodGet, "/_panel/traffic/missing", ``, http.StatusNotFound},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != tt.status {
			t.Errorf("%s %s status = %d, want %d: %s", tt.method, tt.path, res.Code, tt.status, res.Body.String())
		}
	}
}

func TestHandlerUnavailableAndBoundedBody(t *testing.T) {
	handler := NewHandler(NewCounter(), func() error { return errors.New("capture unavailable") })
	req := httptest.NewRequest(http.MethodGet, "/_panel/traffic/instance-1", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("unavailable status = %d, want 503", res.Code)
	}

	handler = NewHandler(NewCounter(), nil)
	large := `{"instance_id":"instance-1","run_id":"run-1","ports":[` + strings.Repeat("27015,", 20000) + `27015]}`
	req = httptest.NewRequest(http.MethodPut, "/_panel/traffic/instance-1", strings.NewReader(large))
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("oversized status = %d, want 400", res.Code)
	}
}

func TestTotalsJSONContainsOnlyRunAndByteCounts(t *testing.T) {
	counter := NewCounter()
	mustRegister(t, counter, Session{InstanceID: "instance-1", RunID: "run-1", Ports: []int{27015}})
	req := httptest.NewRequest(http.MethodGet, "/_panel/traffic/instance-1", nil)
	res := httptest.NewRecorder()
	NewHandler(counter, nil).ServeHTTP(res, req)
	var body map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 3 || body["run_id"] != "run-1" {
		t.Fatalf("unexpected public fields: %#v", body)
	}
}

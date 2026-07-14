package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/not0721here/l4d2-control-panel/internal/auth"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
	"github.com/not0721here/l4d2-control-panel/internal/store"
)

type fakeLifecycle struct{ action string }

func (f *fakeLifecycle) Start(context.Context, string) error        { f.action = "start"; return nil }
func (f *fakeLifecycle) Stop(context.Context, string) error         { f.action = "stop"; return nil }
func (f *fakeLifecycle) Restart(context.Context, string) error      { f.action = "restart"; return nil }
func (f *fakeLifecycle) Rebuild(context.Context, string) error      { f.action = "rebuild"; return nil }
func (f *fakeLifecycle) Delete(context.Context, string, bool) error { f.action = "delete"; return nil }

type fakeAttacher struct{ peer net.Conn }

func (f *fakeAttacher) AttachSupervisor(context.Context, string) (io.ReadWriteCloser, error) {
	client, peer := net.Pipe()
	f.peer = peer
	return client, nil
}

func testServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	a := auth.NewService()
	if err := a.Bootstrap("correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	return New(db, a), db
}
func loginCookie(t *testing.T, s *Server) *http.Cookie {
	t.Helper()
	body := bytes.NewBufferString(`{"password":"correct horse battery staple"}`)
	r := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	return w.Result().Cookies()[0]
}

func TestProtectedRoutesRequireSession(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/instances", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestCreateAndListInstance(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	cookie := loginCookie(t, s)
	payload := map[string]any{"name": "Coop One", "game_port": 27015, "start_map": "c2m1_highway", "game_mode": "coop", "tickrate": 100, "max_players": 8}
	raw, _ := json.Marshal(payload)
	r := httptest.NewRequest(http.MethodPost, "/api/instances", bytes.NewReader(raw))
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	r = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/instances", nil)
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte("Coop One")) {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	events, err := db.AuditEvents(context.Background(), 10)
	if err != nil || len(events) != 1 || events[0].Target != "/api/instances" || events[0].Result != "201" {
		t.Fatalf("audit=%#v err=%v", events, err)
	}
}

func TestCreateRejectsInvalidPort(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	cookie := loginCookie(t, s)
	r := httptest.NewRequest(http.MethodPost, "/api/instances", bytes.NewBufferString(`{"name":"Bad","game_port":80}`))
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestInstanceActionRunsAsPersistentJob(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	cookie := loginCookie(t, s)
	v := map[string]any{"name": "Coop", "game_port": 27015, "start_map": "map", "game_mode": "coop", "tickrate": 100, "max_players": 8}
	raw, _ := json.Marshal(v)
	r := httptest.NewRequest(http.MethodPost, "/api/instances", bytes.NewReader(raw))
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	var created struct {
		ID string `json:"ID"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	life := &fakeLifecycle{}
	s = New(db, s.auth, WithOperations(life, jobs.NewPersistentManager(db)))
	r = httptest.NewRequest(http.MethodPost, "/api/instances/"+created.ID+"/actions", bytes.NewBufferString(`{"action":"start"}`))
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var job jobs.Job
	_ = json.Unmarshal(w.Body.Bytes(), &job)
	deadline := time.Now().Add(time.Second)
	for life.action == "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if life.action != "start" {
		t.Fatalf("action=%q", life.action)
	}
	r = httptest.NewRequest(http.MethodGet, "/api/jobs/"+job.ID, nil)
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"succeeded"`)) {
		t.Fatalf("job: %d %s", w.Code, w.Body.String())
	}
}

func TestStopActionRequiresConfirmation(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	life := &fakeLifecycle{}
	s = New(db, s.auth, WithOperations(life, jobs.NewPersistentManager(db)))
	cookie := loginCookie(t, s)
	r := httptest.NewRequest(http.MethodPost, "/api/instances/abc/actions", bytes.NewBufferString(`{"action":"stop"}`))
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusPreconditionRequired {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestConsoleWebSocketProxiesSupervisorAttach(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	v := domain.Instance{ID: "abc", NodeID: "local", Name: "one", ContainerID: "container-1", GamePort: 27015, StartMap: "map", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", DesiredState: domain.StateRunning, ActualState: domain.StateRunning}
	if err := db.CreateInstance(context.Background(), v); err != nil {
		t.Fatal(err)
	}
	attacher := &fakeAttacher{}
	s = New(db, s.auth, WithConsole(attacher))
	token, err := s.auth.Login("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	header := http.Header{"Cookie": []string{sessionCookie + "=" + token}}
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/api/instances/abc/console", header)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer attacher.peer.Close()
		raw := make([]byte, 7)
		_, _ = io.ReadFull(attacher.peer, raw)
		_, _ = attacher.peer.Write([]byte("console output"))
	}()
	if err := conn.WriteMessage(websocket.TextMessage, []byte("status\n")); err != nil {
		t.Fatal(err)
	}
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "console output" {
		t.Fatalf("got %q", raw)
	}
	<-done
}

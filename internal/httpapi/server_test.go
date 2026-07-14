package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
	"github.com/not0721here/l4d2-control-panel/internal/scheduler"
	"github.com/not0721here/l4d2-control-panel/internal/store"
)

type fakeLifecycle struct{ action string }

func (f *fakeLifecycle) Start(context.Context, string) error        { f.action = "start"; return nil }
func (f *fakeLifecycle) Stop(context.Context, string) error         { f.action = "stop"; return nil }
func (f *fakeLifecycle) Restart(context.Context, string) error      { f.action = "restart"; return nil }
func (f *fakeLifecycle) Rebuild(context.Context, string) error      { f.action = "rebuild"; return nil }
func (f *fakeLifecycle) Delete(context.Context, string, bool) error { f.action = "delete"; return nil }

type fakeAttacher struct{ peer net.Conn }

type fakeScheduleDispatcher struct{}

func (fakeScheduleDispatcher) Dispatch(context.Context, domain.ScheduledTask) error { return nil }

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

func TestCreateAndUpdateExposeSourceTVAndPluginPorts(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	cookie := loginCookie(t, s)
	create := `{"name":"Ports","game_port":27015,"sourcetv_port":27020,"plugin_ports":[27021,27022],"start_map":"c2m1_highway","game_mode":"coop","tickrate":100,"max_players":8}`
	r := httptest.NewRequest(http.MethodPost, "/api/instances", bytes.NewBufferString(create))
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"sourcetv_port":27020`) || !strings.Contains(w.Body.String(), `"plugin_ports":[27021,27022]`) {
		t.Fatalf("create: status=%d body=%s", w.Code, w.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id, _ := created["ID"].(string)
	update := `{"name":"Ports","game_port":27015,"sourcetv_port":27030,"plugin_ports":[27031],"start_map":"c2m1_highway","game_mode":"coop","tickrate":100,"max_players":8,"extra_args":""}`
	r = httptest.NewRequest(http.MethodPut, "/api/instances/"+id, bytes.NewBufferString(update))
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"sourcetv_port":27030`) || !strings.Contains(w.Body.String(), `"plugin_ports":[27031]`) {
		t.Fatalf("update: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateRejectsDuplicateDeclaredPorts(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	cookie := loginCookie(t, s)
	r := httptest.NewRequest(http.MethodPost, "/api/instances", bytes.NewBufferString(`{"name":"Bad Ports","game_port":27015,"sourcetv_port":27015,"plugin_ports":[27020,27020],"start_map":"map","game_mode":"coop","tickrate":100,"max_players":8}`))
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateRejectsInvalidPluginPort(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	cookie := loginCookie(t, s)
	r := httptest.NewRequest(http.MethodPost, "/api/instances", bytes.NewBufferString(`{"name":"Bad Plugin Port","game_port":27015,"plugin_ports":[80],"start_map":"map","game_mode":"coop","tickrate":100,"max_players":8}`))
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestScheduleAcceptsSnakeCaseJSONAndRejectsUnknownFields(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	schedules := scheduler.NewService(db, fakeScheduleDispatcher{})
	defer schedules.Stop()
	s = New(db, s.auth, WithScheduler(schedules))
	cookie := loginCookie(t, s)

	t.Run("snake case", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/api/schedules", bytes.NewBufferString(`{"instance_id":"abc","type":"game_update","cron":"0 4 * * *","timezone":"Asia/Hong_Kong","online_policy":"skip","enabled":true,"payload":"{}"}`))
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"instance_id":"abc"`) || !strings.Contains(w.Body.String(), `"online_policy":"skip"`) {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("unknown field", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/api/schedules", bytes.NewBufferString(`{"instance_id":"abc","type":"game_update","cron":"0 4 * * *","timezone":"UTC","online_policy":"skip","enabled":true,"payload":"{}","surprise":true}`))
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "unknown field") {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	})
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

func TestContentReadRoutesReturnUnavailableWithoutManagers(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	cookie := loginCookie(t, s)
	for _, path := range []string{
		"/api/content/vpk/missing.vpk/download",
		"/api/instances/abc/private",
		"/api/instances/abc/private/history/cfg/server.cfg",
		"/api/instances/abc/private/file/cfg/server.cfg",
	} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: status=%d body=%s", path, w.Code, w.Body.String())
		}
	}
}

func TestContentReadRoutesAndJobFeed(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	root := t.TempDir()
	uploads, err := content.NewUploadManager(root)
	if err != nil {
		t.Fatal(err)
	}
	private := content.NewPrivateManager(root, 1024)
	s = New(db, s.auth, WithContent(uploads, private, nil, nil, nil))
	cookie := loginCookie(t, s)

	if _, err := private.Save(context.Background(), "abc", "cfg/server.cfg", []byte("first")); err != nil {
		t.Fatal(err)
	}
	if _, err := private.Save(context.Background(), "abc", "cfg/server.cfg", []byte("second")); err != nil {
		t.Fatal(err)
	}
	vpk := []byte("vpk-content")
	digest := sha256.Sum256(vpk)
	session, err := uploads.Begin("maps.vpk", int64(len(vpk)), hex.EncodeToString(digest[:]))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uploads.Write(session.ID, 0, bytes.NewReader(vpk)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := uploads.Complete(session.ID); err != nil {
		t.Fatal(err)
	}

	checks := []struct {
		path string
		want string
	}{
		{"/api/content/vpk/maps.vpk/download", "vpk-content"},
		{"/api/instances/abc/private", "cfg/server.cfg"},
		{"/api/instances/abc/private/file/cfg/server.cfg", "second"},
		{"/api/instances/abc/private/history/cfg/server.cfg", "cfg/server.cfg."},
	}
	for _, check := range checks {
		r := httptest.NewRequest(http.MethodGet, check.path, nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), check.want) {
			t.Fatalf("%s: status=%d body=%s", check.path, w.Code, w.Body.String())
		}
		if check.path == "/api/instances/abc/private" && !strings.Contains(w.Body.String(), `"path":"cfg/server.cfg"`) {
			t.Fatalf("private list must use stable lower-case fields: %s", w.Body.String())
		}
		if strings.Contains(w.Body.String(), root) {
			t.Fatalf("%s leaked data root in %s", check.path, w.Body.String())
		}
	}

	now := time.Now().UTC()
	if err := db.SaveJob(domain.JobRecord{ID: "job-1", InstanceID: "abc", Type: "update", Status: "running", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "job-1") {
		t.Fatalf("jobs: status=%d body=%s", w.Code, w.Body.String())
	}

	token, err := s.auth.Login("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	remote := httptest.NewServer(s.Handler())
	defer remote.Close()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remote.URL+"/api/jobs/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	response, err := remote.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(response.Body)
	var event strings.Builder
	for event.Len() < 4096 {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			t.Fatal(readErr)
		}
		event.WriteString(line)
		if line == "\n" {
			break
		}
	}
	cancel()
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.Contains(event.String(), "event: jobs") || !strings.Contains(event.String(), "job-1") {
		t.Fatalf("SSE status=%d event=%q", response.StatusCode, event.String())
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

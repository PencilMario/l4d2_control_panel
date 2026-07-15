package httpapi

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	"github.com/not0721here/l4d2-control-panel/internal/updates"
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

func TestPrivateFileAPIContract(t *testing.T) {
	s, db := testServer(t)
	t.Cleanup(func() { _ = db.Close() })
	if err := db.CreateInstance(context.Background(), domain.Instance{ID: "abc", NodeID: "local", Name: "abc"}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	private := content.NewPrivateManager(root, 1<<20)
	s = New(db, s.auth, WithContent(nil, private, nil, nil, nil), WithPrivateUploads(content.NewPrivateUploadManager(root, 8<<20)))
	cookie := loginCookie(t, s)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.AddCookie(cookie)
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	if w := do(http.MethodPost, "/api/instances/missing/private/directories", `{"path":"cfg"}`); w.Code != 404 || !strings.Contains(w.Body.String(), `"code":"instance_not_found"`) {
		t.Fatalf("missing instance: %d %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodPost, "/api/instances/abc/private/directories", `{"path":"cfg"}`); w.Code != 201 {
		t.Fatalf("mkdir: %d %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodGet, "/api/instances/abc/private/tree", ""); w.Code != 200 {
		t.Fatalf("tree: %d %s", w.Code, w.Body.String())
	}
	if w := do(http.MethodGet, "/api/instances/abc/private/diff", ""); w.Code != 200 {
		t.Fatalf("diff: %d %s", w.Code, w.Body.String())
	}
	hash := sha256.Sum256([]byte("abcdef"))
	w := do(http.MethodPost, "/api/instances/abc/private/uploads", fmt.Sprintf(`{"path":"addons/file.bin","size":6,"sha256":"%x"}`, hash))
	if w.Code != 201 {
		t.Fatalf("begin: %d %s", w.Code, w.Body.String())
	}
	var session content.PrivateUploadSession
	if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPatch, "/api/instances/abc/private/uploads/"+session.ID, strings.NewReader("abcdef"))
	r.AddCookie(cookie)
	r.Header.Set("Upload-Offset", "0")
	r.Header.Set("Content-Type", "text/plain")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 415 {
		t.Fatalf("media type: %d %s", w.Code, w.Body.String())
	}
	r = httptest.NewRequest(http.MethodPatch, "/api/instances/abc/private/uploads/"+session.ID, strings.NewReader("abcdef"))
	r.AddCookie(cookie)
	r.Header.Set("Upload-Offset", "0")
	r.Header.Set("Content-Type", "application/offset+octet-stream")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 204 || w.Header().Get("Upload-Offset") != "6" {
		t.Fatalf("patch: %d %s offset=%s", w.Code, w.Body.String(), w.Header().Get("Upload-Offset"))
	}
	if w = do(http.MethodPost, "/api/instances/abc/private/uploads/"+session.ID+"/complete", ""); w.Code != 204 {
		t.Fatalf("complete: %d %s", w.Code, w.Body.String())
	}
	if w = do(http.MethodGet, "/api/instances/abc/private/file/addons/file.bin", ""); w.Code != 200 || w.Body.String() != "abcdef" {
		t.Fatalf("download: %d %q", w.Code, w.Body.String())
	}
}

func testServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	a := auth.NewService()
	if err := a.Bootstrap("correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	packages, err := content.NewPackageManager(root)
	if err != nil {
		t.Fatal(err)
	}
	addTestPackage(t, packages, "default.zip", "default")
	pipeline := updates.New(root)
	return New(db, a, WithContent(nil, nil, packages, pipeline, nil)), db
}

func addTestPackage(t *testing.T, manager *content.PackageManager, name, version string) string {
	t.Helper()
	var raw bytes.Buffer
	writer := zip.NewWriter(&raw)
	file, err := writer.Create("cfg/plugin.cfg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("sm_cvar fixture 1")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	item, err := manager.AddUpload(name, version, bytes.NewReader(raw.Bytes()), int64(raw.Len()))
	if err != nil {
		t.Fatal(err)
	}
	return item.ID
}

func defaultPackageID(t *testing.T, server *Server) string {
	t.Helper()
	items, err := server.packages.List()
	if err != nil || len(items) == 0 {
		t.Fatalf("packages=%#v err=%v", items, err)
	}
	return items[0].ID
}

func authenticatedJSON(t *testing.T, server *Server, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func waitForJob(t *testing.T, manager *jobs.Manager, id string) jobs.Job {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, ok := manager.Get(id)
		if ok && (job.Status == jobs.Succeeded || job.Status == jobs.Failed) {
			return job
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("job did not finish")
	return jobs.Job{}
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

func TestLoginCookieSecurityCanBeDisabled(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	s = New(db, s.auth, WithSecureCookie(false))
	cookie := loginCookie(t, s)
	if cookie.Secure {
		t.Fatal("HTTP deployments must be able to disable the Secure cookie attribute")
	}
}

func TestCreateAndListInstance(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	cookie := loginCookie(t, s)
	payload := map[string]any{"name": "Coop One", "game_port": 27015, "start_map": "c2m1_highway", "game_mode": "coop", "tickrate": 100, "max_players": 8, "package_id": defaultPackageID(t, s)}
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
	packageID := defaultPackageID(t, s)
	create := fmt.Sprintf(`{"name":"Ports","game_port":27015,"sourcetv_port":27020,"plugin_ports":[27021,27022],"start_map":"c2m1_highway","game_mode":"coop","tickrate":100,"max_players":8,"package_id":%q}`, packageID)
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
	update := fmt.Sprintf(`{"name":"Ports","game_port":27015,"sourcetv_port":27030,"plugin_ports":[27031],"start_map":"c2m1_highway","game_mode":"coop","tickrate":100,"max_players":8,"extra_args":"","package_id":%q}`, packageID)
	r = httptest.NewRequest(http.MethodPut, "/api/instances/"+id, bytes.NewBufferString(update))
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"sourcetv_port":27030`) || !strings.Contains(w.Body.String(), `"plugin_ports":[27031]`) {
		t.Fatalf("update: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreatePersistsPackageAndExtraArgs(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	cookie := loginCookie(t, s)
	packageID := defaultPackageID(t, s)
	body := fmt.Sprintf(`{"name":"Startup","game_port":27015,"start_map":"c2m1_highway","game_mode":"coop","tickrate":100,"max_players":8,"extra_args":"-strictportbind +hostname \"Night Coop\"","package_id":%q}`, packageID)
	response := authenticatedJSON(t, s, cookie, http.MethodPost, "/api/instances", body)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	items, err := db.Instances(context.Background())
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	if items[0].SelectedPackageID != packageID || items[0].PackageVersion != "" || items[0].ExtraArgs != `-strictportbind +hostname "Night Coop"` {
		t.Fatalf("instance=%#v", items[0])
	}
}

func TestCreateRejectsMissingPackageAndReservedArguments(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	cookie := loginCookie(t, s)

	missing := authenticatedJSON(t, s, cookie, http.MethodPost, "/api/instances", `{"name":"Missing","game_port":27015,"start_map":"map","game_mode":"coop","tickrate":100,"max_players":8,"package_id":"00000000-0000-0000-0000-000000000000"}`)
	if missing.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing: status=%d body=%s", missing.Code, missing.Body.String())
	}

	reservedBody := fmt.Sprintf(`{"name":"Reserved","game_port":27015,"start_map":"map","game_mode":"coop","tickrate":100,"max_players":8,"extra_args":"-port 27016","package_id":%q}`, defaultPackageID(t, s))
	reserved := authenticatedJSON(t, s, cookie, http.MethodPost, "/api/instances", reservedBody)
	if reserved.Code != http.StatusUnprocessableEntity || !strings.Contains(reserved.Body.String(), "managed by the Panel") {
		t.Fatalf("reserved: status=%d body=%s", reserved.Code, reserved.Body.String())
	}
}

func TestCreateRejectsRuntimeImageOverride(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	cookie := loginCookie(t, s)
	body := fmt.Sprintf(`{"name":"Image Override","game_port":27015,"start_map":"map","game_mode":"coop","tickrate":100,"max_players":8,"package_id":%q,"runtime_image":"attacker/runtime:latest"}`, defaultPackageID(t, s))
	response := authenticatedJSON(t, s, cookie, http.MethodPost, "/api/instances", body)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "unknown field") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestUpdatePlansOnlyRequiredRuntimeWork(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	packageA := defaultPackageID(t, s)
	packageB := addTestPackage(t, s.packages, "second.zip", "second")
	value := domain.Instance{ID: "configured", NodeID: "local", Name: "Configured", ContainerID: "container", GamePort: 27015, StartMap: "map", GameMode: "coop", Tickrate: 100, MaxPlayers: 8, RuntimeImage: "runtime", SelectedPackageID: packageA, PackageVersion: packageA, DesiredState: domain.StateStopped, ActualState: domain.StateStopped}
	if err := db.CreateInstance(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	life := &fakeLifecycle{}
	manager := jobs.NewPersistentManager(db)
	coordinator := &updates.Coordinator{Lifecycle: life, Deployer: s.updates, Instances: db}
	s = New(db, s.auth, WithOperations(life, manager), WithContent(nil, nil, s.packages, s.updates, coordinator))
	cookie := loginCookie(t, s)

	nameOnly := fmt.Sprintf(`{"name":"Renamed","game_port":27015,"start_map":"map","game_mode":"coop","tickrate":100,"max_players":8,"extra_args":"","package_id":%q}`, packageA)
	response := authenticatedJSON(t, s, cookie, http.MethodPut, "/api/instances/"+value.ID, nameOnly)
	if response.Code != http.StatusOK || life.action != "" {
		t.Fatalf("name only: status=%d action=%q body=%s", response.Code, life.action, response.Body.String())
	}

	runtimeOnly := fmt.Sprintf(`{"name":"Renamed","game_port":27015,"start_map":"map","game_mode":"coop","tickrate":100,"max_players":8,"extra_args":"-strictportbind","package_id":%q}`, packageA)
	response = authenticatedJSON(t, s, cookie, http.MethodPut, "/api/instances/"+value.ID, runtimeOnly)
	if response.Code != http.StatusAccepted {
		t.Fatalf("runtime: status=%d body=%s", response.Code, response.Body.String())
	}
	var runtimeJob jobs.Job
	if err := json.Unmarshal(response.Body.Bytes(), &runtimeJob); err != nil {
		t.Fatal(err)
	}
	if got := waitForJob(t, manager, runtimeJob.ID); got.Status != jobs.Succeeded || life.action != "rebuild" {
		t.Fatalf("job=%#v action=%q", got, life.action)
	}

	life.action = ""
	combined := fmt.Sprintf(`{"name":"Renamed","game_port":27015,"start_map":"map","game_mode":"coop","tickrate":100,"max_players":8,"extra_args":"+hostname \"Changed\"","package_id":%q}`, packageB)
	response = authenticatedJSON(t, s, cookie, http.MethodPut, "/api/instances/"+value.ID, combined)
	if response.Code != http.StatusAccepted {
		t.Fatalf("combined: status=%d body=%s", response.Code, response.Body.String())
	}
	var combinedJob jobs.Job
	if err := json.Unmarshal(response.Body.Bytes(), &combinedJob); err != nil {
		t.Fatal(err)
	}
	if got := waitForJob(t, manager, combinedJob.ID); got.Status != jobs.Succeeded || life.action != "rebuild" {
		t.Fatalf("job=%#v action=%q", got, life.action)
	}
	stored, err := db.Instance(context.Background(), value.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.SelectedPackageID != packageB || stored.PackageVersion != packageB || stored.ExtraArgs != `+hostname "Changed"` {
		t.Fatalf("instance=%#v", stored)
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
	v := map[string]any{"name": "Coop", "game_port": 27015, "start_map": "map", "game_mode": "coop", "tickrate": 100, "max_players": 8, "package_id": defaultPackageID(t, s)}
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
	if err := db.CreateInstance(context.Background(), domain.Instance{ID: "abc", NodeID: "local", Name: "abc"}); err != nil {
		t.Fatal(err)
	}
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
	if err := db.CreateInstance(context.Background(), domain.Instance{ID: "abc", NodeID: "local", Name: "abc"}); err != nil {
		t.Fatal(err)
	}
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

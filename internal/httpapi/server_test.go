package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/not0721here/l4d2-control-panel/internal/auth"
	"github.com/not0721here/l4d2-control-panel/internal/store"
)

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

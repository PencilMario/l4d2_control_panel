package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/not0721here/l4d2-control-panel/internal/auth"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/store"
)

const sessionCookie = "l4d2_panel_session"

type Server struct {
	store  *store.Store
	auth   *auth.Service
	router http.Handler
}

func New(db *store.Store, a *auth.Service) *Server {
	s := &Server{store: db, auth: a}
	r := chi.NewRouter()
	r.Post("/api/auth/login", s.login)
	r.Group(func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Post("/api/auth/logout", s.logout)
		r.Get("/api/session", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
		})
		r.Get("/api/instances", s.listInstances)
		r.Post("/api/instances", s.createInstance)
	})
	s.router = r
	return s
}
func (s *Server) Handler() http.Handler { return s.router }
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
	}
	if decodeJSON(w, r, &in) != nil {
		return
	}
	token, err := s.auth.Login(in.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", err.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: 86400})
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.auth.Logout(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || !s.auth.Valid(c.Value) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) listInstances(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.Instances(r.Context())
	if err != nil {
		writeError(w, 500, "store_error", err.Error())
		return
	}
	if items == nil {
		items = []domain.Instance{}
	}
	writeJSON(w, 200, items)
}
func (s *Server) createInstance(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name       string `json:"name"`
		GamePort   int    `json:"game_port"`
		StartMap   string `json:"start_map"`
		GameMode   string `json:"game_mode"`
		Tickrate   int    `json:"tickrate"`
		MaxPlayers int    `json:"max_players"`
	}
	if decodeJSON(w, r, &in) != nil {
		return
	}
	if in.Name == "" || in.GamePort < 1024 || in.GamePort > 65535 || in.StartMap == "" || in.GameMode == "" || in.Tickrate < 30 || in.Tickrate > 128 || in.MaxPlayers < 1 || in.MaxPlayers > 32 {
		writeError(w, 422, "invalid_instance", "name, valid port, map, mode, tickrate and player limit are required")
		return
	}
	v := domain.Instance{ID: uuid.NewString(), NodeID: "local", Name: in.Name, GamePort: in.GamePort, StartMap: in.StartMap, GameMode: in.GameMode, Tickrate: in.Tickrate, MaxPlayers: in.MaxPlayers, RuntimeImage: "l4d2-server-runtime:latest", DesiredState: domain.StateStopped, ActualState: domain.StateUninstalled}
	if err := s.store.CreateInstance(r.Context(), v); err != nil {
		writeError(w, 409, "instance_conflict", err.Error())
		return
	}
	writeJSON(w, 201, v)
}
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		writeError(w, 400, "invalid_json", err.Error())
		return err
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

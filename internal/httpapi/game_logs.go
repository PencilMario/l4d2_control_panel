package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/not0721here/l4d2-control-panel/internal/store"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

func (s *Server) ensureLogInstance(r *http.Request) error {
	_, err := s.store.Instance(r.Context(), chi.URLParam(r, "id"))
	return err
}
func (s *Server) gameLogsTree(w http.ResponseWriter, r *http.Request) {
	if err := s.ensureLogInstance(r); err != nil {
		writeJSON(w, 404, map[string]string{"error": "instance not found"})
		return
	}
	if s.gameLogs == nil {
		writeJSON(w, 503, map[string]string{"error": "game logs unavailable"})
		return
	}
	v, err := s.gameLogs.Tree(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, 200, v)
}
func validLogQuery(r *http.Request) (string, string, error) {
	q := r.URL.Query()
	kind, path := q.Get("kind"), q.Get("path")
	if kind != "game" && kind != "sourcemod" {
		return "", "", errors.New("invalid kind")
	}
	if path == "" || strings.IndexByte(path, 0) >= 0 || filepath.IsAbs(path) || strings.Contains(path, "..") {
		return "", "", errors.New("invalid path")
	}
	return kind, path, nil
}
func (s *Server) gameLogsPreview(w http.ResponseWriter, r *http.Request) {
	if err := s.ensureLogInstance(r); err != nil {
		writeJSON(w, 404, map[string]string{"error": "instance not found"})
		return
	}
	if s.gameLogs == nil {
		writeJSON(w, 503, nil)
		return
	}
	k, p, err := validLogQuery(r)
	if err != nil {
		writeJSON(w, 422, map[string]string{"error": err.Error()})
		return
	}
	v, err := s.gameLogs.Preview(r.Context(), chi.URLParam(r, "id"), k, p, 10*1024*1024)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, context.Canceled) {
			writeJSON(w, 404, map[string]string{"error": "not found"})
		} else {
			writeJSON(w, 404, map[string]string{"error": "not found"})
		}
		return
	}
	writeJSON(w, 200, v)
}
func (s *Server) gameLogsDownload(w http.ResponseWriter, r *http.Request) {
	if err := s.ensureLogInstance(r); err != nil {
		writeJSON(w, 404, nil)
		return
	}
	if s.gameLogs == nil {
		writeJSON(w, 503, nil)
		return
	}
	k, p, err := validLogQuery(r)
	if err != nil {
		writeJSON(w, 422, map[string]string{"error": err.Error()})
		return
	}
	f, info, err := s.gameLogs.ResolveDownload(chi.URLParam(r, "id"), k, p)
	if err != nil {
		writeJSON(w, 404, nil)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/plain")
	name := filepath.Base(p)
	fallback := regexp.MustCompile(`[^A-Za-z0-9._-]`).ReplaceAllString(name, "_")
	if fallback == "" {
		fallback = "download.log"
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+fallback+`"; filename*=UTF-8''`+url.PathEscape(name))
	http.ServeContent(w, r, filepath.Base(p), info.ModTime(), f)
}
func (s *Server) getGameLogSettings(w http.ResponseWriter, r *http.Request) {
	d, err := s.store.GameLogRetentionDays()
	if err != nil {
		writeJSON(w, 500, nil)
		return
	}
	writeJSON(w, 200, map[string]int{"retention_days": d})
}
func decodeStrict(r *http.Request, v any) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("trailing data")
	}
	return nil
}
func (s *Server) putGameLogSettings(w http.ResponseWriter, r *http.Request) {
	if s.gameLogScheduler == nil {
		writeJSON(w, 503, nil)
		return
	}
	var in struct {
		RetentionDays int `json:"retention_days"`
	}
	if err := decodeStrict(r, &in); err != nil || in.RetentionDays < 1 || in.RetentionDays > 365 {
		writeJSON(w, 422, nil)
		return
	}
	if err := s.store.SetGameLogRetentionDays(in.RetentionDays); err != nil {
		writeJSON(w, 422, nil)
		return
	}
	result := s.gameLogScheduler.EnqueueAll(context.Background())
	writeJSON(w, 200, map[string]any{"retention_days": in.RetentionDays, "enqueue": result})
}
func (s *Server) cleanupGameLogs(w http.ResponseWriter, r *http.Request) {
	if s.gameLogScheduler == nil {
		writeJSON(w, 503, nil)
		return
	}
	v := s.gameLogScheduler.EnqueueAll(context.Background())
	writeJSON(w, 200, v)
}

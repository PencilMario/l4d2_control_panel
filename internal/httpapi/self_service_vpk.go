package httpapi

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const selfServiceVPKCookie = "l4d2_self_service_vpk"

func (s *Server) selfServiceKey() []byte {
	if len(s.selfServiceVPKKey) == 0 {
		s.selfServiceVPKKey = make([]byte, 32)
		if _, err := rand.Read(s.selfServiceVPKKey); err != nil {
			panic(err)
		}
	}
	return s.selfServiceVPKKey
}

func (s *Server) selfServiceVPKStatus(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.SelfServiceVPKSettings()
	if err != nil {
		writeError(w, 500, "settings_error", err.Error())
		return
	}
	authorized := settings.Enabled && !settings.PasswordSet
	if settings.Enabled && settings.PasswordSet {
		authorized, _ = s.hasSelfServiceVPKAccess(r)
	}
	writeJSON(w, 200, map[string]any{"enabled": settings.Enabled, "password_required": settings.PasswordSet, "authorized": authorized, "auto_delete": settings.AutoDelete})
}

func (s *Server) authorizeSelfServiceVPK(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.SelfServiceVPKSettings()
	if err != nil {
		writeError(w, 500, "settings_error", err.Error())
		return
	}
	if !settings.Enabled {
		writeError(w, 403, "self_service_disabled", "self-service VPK upload is disabled")
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if decodeJSON(w, r, &input) != nil {
		return
	}
	ok, version, err := s.store.VerifySelfServiceVPKPassword(input.Password)
	if err != nil {
		writeError(w, 500, "authorization_error", err.Error())
		return
	}
	if !ok {
		writeError(w, 401, "invalid_password", "invalid password")
		return
	}
	expires := time.Now().UTC().Add(24 * time.Hour)
	http.SetCookie(w, &http.Cookie{Name: selfServiceVPKCookie, Value: s.signSelfServiceVPKAuthorization(version, expires), Path: "/", HttpOnly: true, Secure: s.secureCookie, SameSite: http.SameSiteStrictMode, Expires: expires, MaxAge: 24 * 60 * 60})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) signSelfServiceVPKAuthorization(version int64, expires time.Time) string {
	payload := fmt.Sprintf("%d:%d", version, expires.Unix())
	mac := hmac.New(sha256.New, s.selfServiceKey())
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *Server) hasSelfServiceVPKAccess(r *http.Request) (bool, int) {
	settings, err := s.store.SelfServiceVPKSettings()
	if err != nil {
		return false, http.StatusInternalServerError
	}
	if !settings.Enabled {
		return false, http.StatusForbidden
	}
	if !settings.PasswordSet {
		return true, 0
	}
	cookie, err := r.Cookie(selfServiceVPKCookie)
	if err != nil {
		return false, http.StatusUnauthorized
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return false, http.StatusUnauthorized
	}
	payload, err1 := base64.RawURLEncoding.DecodeString(parts[0])
	signature, err2 := base64.RawURLEncoding.DecodeString(parts[1])
	if err1 != nil || err2 != nil {
		return false, http.StatusUnauthorized
	}
	mac := hmac.New(sha256.New, s.selfServiceKey())
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return false, http.StatusUnauthorized
	}
	fields := strings.Split(string(payload), ":")
	if len(fields) != 2 {
		return false, http.StatusUnauthorized
	}
	version, err1 := strconv.ParseInt(fields[0], 10, 64)
	expires, err2 := strconv.ParseInt(fields[1], 10, 64)
	if err1 != nil || err2 != nil || version != settings.PasswordVersion || time.Now().Unix() >= expires {
		return false, http.StatusUnauthorized
	}
	return true, 0
}

func (s *Server) requireSelfServiceVPK(w http.ResponseWriter, r *http.Request) bool {
	ok, status := s.hasSelfServiceVPKAccess(r)
	if ok {
		return true
	}
	code, message := "authorization_required", "self-service authorization required"
	if status == http.StatusForbidden {
		code, message = "self_service_disabled", "self-service VPK upload is disabled"
	}
	if status == http.StatusInternalServerError {
		code, message = "settings_error", "self-service settings unavailable"
	}
	writeError(w, status, code, message)
	return false
}

func (s *Server) getSelfServiceVPKSettings(w http.ResponseWriter, _ *http.Request) {
	settings, err := s.store.SelfServiceVPKSettings()
	if err != nil {
		writeError(w, 500, "settings_error", err.Error())
		return
	}
	writeJSON(w, 200, settings)
}

func (s *Server) putSelfServiceVPKSettings(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Enabled       bool    `json:"enabled"`
		Password      *string `json:"password"`
		AutoDelete    bool    `json:"auto_delete"`
		RetentionDays int     `json:"retention_days"`
	}
	if decodeJSON(w, r, &input) != nil {
		return
	}
	if err := s.store.SetSelfServiceVPKSettings(input.Enabled, input.Password, input.AutoDelete, input.RetentionDays); err != nil {
		writeError(w, 422, "invalid_settings", err.Error())
		return
	}
	s.getSelfServiceVPKSettings(w, r)
}

func (s *Server) listSelfServiceVPK(w http.ResponseWriter, r *http.Request) {
	if !s.requireSelfServiceVPK(w, r) {
		return
	}
	if s.selfServiceVPK == nil {
		writeError(w, 503, "content_unavailable", "self-service VPK manager unavailable")
		return
	}
	limit, offset := 20, 0
	var err error
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil {
			writeError(w, 422, "invalid_pagination", "limit must be numeric")
			return
		}
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil {
			writeError(w, 422, "invalid_pagination", "offset must be numeric")
			return
		}
	}
	items, total, err := s.selfServiceVPK.List(limit, offset)
	if err != nil {
		writeError(w, 422, "invalid_pagination", err.Error())
		return
	}
	settings, _ := s.store.SelfServiceVPKSettings()
	writeJSON(w, 200, map[string]any{"items": items, "total": total, "limit": limit, "offset": offset, "auto_delete": settings.AutoDelete})
}

func (s *Server) beginSelfServiceVPK(w http.ResponseWriter, r *http.Request) {
	if !s.requireSelfServiceVPK(w, r) {
		return
	}
	s.beginVPK(w, r)
}
func (s *Server) recoverSelfServiceVPKUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requireSelfServiceVPK(w, r) {
		return
	}
	s.recoverVPKUpload(w, r)
}
func (s *Server) writeSelfServiceVPK(w http.ResponseWriter, r *http.Request) {
	if !s.requireSelfServiceVPK(w, r) {
		return
	}
	s.writeVPK(w, r)
}
func (s *Server) cancelSelfServiceVPKUpload(w http.ResponseWriter, r *http.Request) {
	if !s.requireSelfServiceVPK(w, r) {
		return
	}
	s.cancelVPKUpload(w, r)
}

func (s *Server) completeSelfServiceVPK(w http.ResponseWriter, r *http.Request) {
	if !s.requireSelfServiceVPK(w, r) {
		return
	}
	if s.selfServiceVPK == nil {
		writeError(w, 503, "content_unavailable", "self-service VPK manager unavailable")
		return
	}
	var input struct {
		Clean bool `json:"clean"`
	}
	if decodeJSON(w, r, &input) != nil {
		return
	}
	item, err := s.selfServiceVPK.Complete(chi.URLParam(r, "id"), input.Clean, time.Now().UTC())
	if err != nil {
		writeError(w, 422, "upload_incomplete", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"item": item, "duplicate": false})
}

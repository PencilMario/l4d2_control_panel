package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/not0721here/l4d2-control-panel/internal/auth"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/docker"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
	"github.com/not0721here/l4d2-control-panel/internal/players"
	"github.com/not0721here/l4d2-control-panel/internal/releases"
	"github.com/not0721here/l4d2-control-panel/internal/scheduler"
	"github.com/not0721here/l4d2-control-panel/internal/secrets"
	"github.com/not0721here/l4d2-control-panel/internal/store"
	"github.com/not0721here/l4d2-control-panel/internal/updates"
)

const sessionCookie = "l4d2_panel_session"

type Server struct {
	store             *store.Store
	auth              *auth.Service
	router            http.Handler
	lifecycle         Lifecycle
	jobs              *jobs.Manager
	console           ConsoleAttacher
	players           PlayerService
	uploads           *content.UploadManager
	private           *content.PrivateManager
	updates           *updates.Pipeline
	packages          *content.PackageManager
	updateCoordinator *updates.Coordinator
	releases          releases.Client
	gameUpdates       *updates.GameCoordinator
	schedules         *scheduler.Service
	secrets           *secrets.Service
	resources         ResourceProvider
	system            SystemProvider
}

type Lifecycle interface {
	Start(context.Context, string) error
	Stop(context.Context, string) error
	Restart(context.Context, string) error
	Rebuild(context.Context, string) error
	Delete(context.Context, string, bool) error
}
type Option func(*Server)

func WithOperations(lifecycle Lifecycle, manager *jobs.Manager) Option {
	return func(s *Server) { s.lifecycle = lifecycle; s.jobs = manager }
}

type ConsoleAttacher interface {
	AttachSupervisor(context.Context, string) (io.ReadWriteCloser, error)
}

func WithConsole(attacher ConsoleAttacher) Option { return func(s *Server) { s.console = attacher } }

type PlayerService interface {
	Online(context.Context, string) (players.Snapshot, error)
	Kick(context.Context, string, int) error
	Ban(context.Context, string, int, int) error
}

func WithPlayers(service PlayerService) Option { return func(s *Server) { s.players = service } }
func WithContent(uploads *content.UploadManager, private *content.PrivateManager, packages *content.PackageManager, pipeline *updates.Pipeline, coordinator *updates.Coordinator) Option {
	return func(s *Server) {
		s.uploads = uploads
		s.private = private
		s.packages = packages
		s.updates = pipeline
		s.updateCoordinator = coordinator
		s.releases = releases.Client{}
	}
}
func WithGameUpdates(coordinator *updates.GameCoordinator) Option {
	return func(s *Server) { s.gameUpdates = coordinator }
}
func WithScheduler(service *scheduler.Service) Option {
	return func(s *Server) { s.schedules = service }
}
func WithSecrets(service *secrets.Service) Option { return func(s *Server) { s.secrets = service } }

type ResourceProvider interface {
	Stats(context.Context, string) (docker.ResourceStats, error)
}

func WithResources(provider ResourceProvider) Option {
	return func(s *Server) { s.resources = provider }
}

type SystemProvider interface {
	Info(context.Context) (docker.Info, error)
}

func WithSystem(provider SystemProvider) Option { return func(s *Server) { s.system = provider } }

func New(db *store.Store, a *auth.Service, options ...Option) *Server {
	s := &Server{store: db, auth: a}
	for _, option := range options {
		option(s)
	}
	r := chi.NewRouter()
	r.Post("/api/auth/login", s.login)
	r.Get("/api/health", s.health)
	r.Group(func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Use(s.auditMutations)
		r.Post("/api/auth/logout", s.logout)
		r.Get("/api/session", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
		})
		r.Get("/api/instances", s.listInstances)
		r.Post("/api/instances", s.createInstance)
		r.Put("/api/instances/{id}", s.updateInstance)
		r.Delete("/api/instances/{id}", s.deleteInstance)
		r.Post("/api/instances/{id}/actions", s.instanceAction)
		r.Get("/api/jobs/{id}", s.getJob)
		r.Get("/api/jobs", s.listJobs)
		r.Get("/api/jobs/events", s.jobEvents)
		r.Get("/api/instances/{id}/console", s.consoleSocket)
		r.Get("/api/instances/{id}/players", s.onlinePlayers)
		r.Get("/api/instances/{id}/resources", s.instanceResources)
		r.Post("/api/instances/{id}/players/{userID}/actions", s.playerAction)
		r.Get("/api/audit", s.auditEvents)
		r.Get("/api/content/vpk", s.listVPK)
		r.Get("/api/content/vpk/{name}/download", s.downloadVPK)
		r.Post("/api/content/vpk/uploads", s.beginVPK)
		r.Patch("/api/content/vpk/uploads/{id}", s.writeVPK)
		r.Post("/api/content/vpk/uploads/{id}/complete", s.completeVPK)
		r.Post("/api/content/vpk/{name}/rename", s.renameVPK)
		r.Delete("/api/content/vpk/{name}", s.deleteVPK)
		r.Put("/api/instances/{id}/private/*", s.savePrivate)
		r.Get("/api/instances/{id}/private", s.listPrivate)
		r.Get("/api/instances/{id}/private/history/*", s.privateHistory)
		r.Get("/api/instances/{id}/private/file/*", s.downloadPrivate)
		r.Delete("/api/instances/{id}/private/file/*", s.deletePrivate)
		r.Post("/api/instances/{id}/private/apply", s.applyPrivate)
		r.Get("/api/packages", s.listPackages)
		r.Post("/api/packages/uploads", s.uploadPackage)
		r.Post("/api/packages/github", s.fetchRelease)
		r.Post("/api/instances/{id}/updates", s.updatePackage)
		r.Post("/api/instances/{id}/game-update", s.updateGame)
		r.Get("/api/schedules", s.listSchedules)
		r.Post("/api/schedules", s.saveSchedule)
		r.Delete("/api/schedules/{id}", s.deleteSchedule)
		r.Post("/api/schedules/{id}/run", s.runSchedule)
		r.Get("/api/settings/github-token", s.githubTokenStatus)
		r.Put("/api/settings/github-token", s.setGithubToken)
		r.Delete("/api/settings/github-token", s.deleteGithubToken)
		r.Get("/api/settings/steam", s.steamCredentialStatus)
		r.Put("/api/settings/steam", s.setSteamCredentials)
		r.Delete("/api/settings/steam", s.deleteSteamCredentials)
	})
	s.router = r
	return s
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.Jobs(r.Context(), 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "jobs_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) jobEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "sse_unavailable", "streaming unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		items, err := s.store.Jobs(r.Context(), 50)
		if err != nil {
			return
		}
		raw, err := json.Marshal(items)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintf(w, "event: jobs\ndata: %s\n\n", raw)
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) downloadVPK(w http.ResponseWriter, r *http.Request) {
	if s.uploads == nil {
		writeError(w, http.StatusServiceUnavailable, "content_unavailable", "content manager unavailable")
		return
	}
	path, err := s.uploads.Path(chi.URLParam(r, "name"))
	if err != nil {
		writeError(w, http.StatusNotFound, "vpk_not_found", err.Error())
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(path)+`"`)
	http.ServeFile(w, r, path)
}

func (s *Server) listPrivate(w http.ResponseWriter, r *http.Request) {
	if s.private == nil {
		writeError(w, http.StatusServiceUnavailable, "content_unavailable", "private manager unavailable")
		return
	}
	items, err := s.private.List(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "private_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) privateHistory(w http.ResponseWriter, r *http.Request) {
	if s.private == nil {
		writeError(w, http.StatusServiceUnavailable, "content_unavailable", "private manager unavailable")
		return
	}
	items, err := s.private.History(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "*"))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "private_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) downloadPrivate(w http.ResponseWriter, r *http.Request) {
	if s.private == nil {
		writeError(w, http.StatusServiceUnavailable, "content_unavailable", "private manager unavailable")
		return
	}
	raw, err := s.private.Read(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "*"))
	if err != nil {
		writeError(w, http.StatusNotFound, "private_not_found", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(raw)
}

func (s *Server) deletePrivate(w http.ResponseWriter, r *http.Request) {
	if s.private == nil {
		writeError(w, http.StatusServiceUnavailable, "content_unavailable", "private manager unavailable")
		return
	}
	if r.URL.Query().Get("confirm") != "true" {
		writeError(w, http.StatusPreconditionRequired, "confirmation_required", "private file deletion requires confirmation")
		return
	}
	if err := s.private.Delete(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "*")); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "private_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) steamCredentialStatus(w http.ResponseWriter, r *http.Request) {
	if s.secrets == nil {
		writeError(w, 503, "secrets_unavailable", "secret store unavailable")
		return
	}
	_, user, _ := s.secrets.Get(r.Context(), "steam_username")
	_, password, _ := s.secrets.Get(r.Context(), "steam_password")
	writeJSON(w, 200, map[string]bool{"configured": user && password})
}
func (s *Server) setSteamCredentials(w http.ResponseWriter, r *http.Request) {
	if s.secrets == nil {
		writeError(w, 503, "secrets_unavailable", "secret store unavailable")
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if decodeJSON(w, r, &input) != nil {
		return
	}
	if err := s.secrets.Set(r.Context(), "steam_username", input.Username); err != nil {
		writeError(w, 422, "secret_error", err.Error())
		return
	}
	if err := s.secrets.Set(r.Context(), "steam_password", input.Password); err != nil {
		_ = s.secrets.Delete(r.Context(), "steam_username")
		writeError(w, 422, "secret_error", err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"configured": true})
}
func (s *Server) deleteSteamCredentials(w http.ResponseWriter, r *http.Request) {
	if s.secrets == nil {
		writeError(w, 503, "secrets_unavailable", "secret store unavailable")
		return
	}
	_ = s.secrets.Delete(r.Context(), "steam_username")
	_ = s.secrets.Delete(r.Context(), "steam_password")
	w.WriteHeader(204)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DB().PingContext(r.Context()); err != nil {
		writeError(w, 503, "database_unavailable", err.Error())
		return
	}
	result := map[string]any{"status": "ok", "database": "ok"}
	if s.system != nil {
		info, err := s.system.Info(r.Context())
		if err != nil {
			writeError(w, 503, "docker_unavailable", err.Error())
			return
		}
		result["docker_version"] = info.ServerVersion
		result["containers_running"] = info.ContainersRunning
	}
	writeJSON(w, 200, result)
}

func (s *Server) instanceResources(w http.ResponseWriter, r *http.Request) {
	if s.resources == nil {
		writeError(w, 503, "resources_unavailable", "resource provider unavailable")
		return
	}
	instance, err := s.store.Instance(r.Context(), chi.URLParam(r, "id"))
	if err != nil || instance.ContainerID == "" {
		writeError(w, 404, "instance_not_running", "instance container unavailable")
		return
	}
	stats, err := s.resources.Stats(r.Context(), instance.ContainerID)
	if err != nil {
		writeError(w, 502, "stats_failed", err.Error())
		return
	}
	writeJSON(w, 200, stats)
}

func (s *Server) updateInstance(w http.ResponseWriter, r *http.Request) {
	instance, err := s.store.Instance(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 404, "instance_not_found", err.Error())
		return
	}
	var input struct {
		Name         string `json:"name"`
		GamePort     int    `json:"game_port"`
		SourceTVPort int    `json:"sourcetv_port"`
		PluginPorts  []int  `json:"plugin_ports"`
		StartMap     string `json:"start_map"`
		GameMode     string `json:"game_mode"`
		Tickrate     int    `json:"tickrate"`
		MaxPlayers   int    `json:"max_players"`
		ExtraArgs    string `json:"extra_args"`
		RuntimeImage string `json:"runtime_image"`
	}
	if decodeJSON(w, r, &input) != nil {
		return
	}
	if input.Name == "" || input.StartMap == "" || input.GameMode == "" || input.Tickrate < 30 || input.Tickrate > 128 || input.MaxPlayers < 1 || input.MaxPlayers > 32 {
		writeError(w, 422, "invalid_instance", "invalid instance configuration")
		return
	}
	if err := validateDeclaredPorts(input.GamePort, input.SourceTVPort, input.PluginPorts); err != nil {
		writeError(w, 422, "invalid_instance", err.Error())
		return
	}
	instance.Name, instance.GamePort, instance.SourceTVPort, instance.PluginPorts, instance.StartMap, instance.GameMode, instance.Tickrate, instance.MaxPlayers, instance.ExtraArgs = input.Name, input.GamePort, input.SourceTVPort, input.PluginPorts, input.StartMap, input.GameMode, input.Tickrate, input.MaxPlayers, input.ExtraArgs
	if input.RuntimeImage != "" {
		instance.RuntimeImage = input.RuntimeImage
	}
	if err := s.store.UpdateInstance(r.Context(), instance); err != nil {
		writeError(w, 409, "instance_conflict", err.Error())
		return
	}
	if instance.ContainerID != "" && s.lifecycle != nil && s.jobs != nil {
		job := s.jobs.Start(context.WithoutCancel(r.Context()), instance.ID, "rebuild", func(ctx context.Context, _ jobs.Reporter) error { return s.lifecycle.Rebuild(ctx, instance.ID) })
		writeJSON(w, 202, job)
		return
	}
	writeJSON(w, 200, instance)
}
func (s *Server) deleteInstance(w http.ResponseWriter, r *http.Request) {
	if s.lifecycle == nil || s.jobs == nil {
		writeError(w, 503, "operations_unavailable", "lifecycle unavailable")
		return
	}
	var input struct {
		Confirm    bool `json:"confirm"`
		DeleteData bool `json:"delete_data"`
	}
	if decodeJSON(w, r, &input) != nil {
		return
	}
	if !input.Confirm {
		writeError(w, 428, "confirmation_required", "instance deletion requires confirmation")
		return
	}
	id := chi.URLParam(r, "id")
	job := s.jobs.Start(context.WithoutCancel(r.Context()), id, "delete", func(ctx context.Context, _ jobs.Reporter) error { return s.lifecycle.Delete(ctx, id, input.DeleteData) })
	writeJSON(w, 202, job)
}

func (s *Server) githubTokenStatus(w http.ResponseWriter, r *http.Request) {
	if s.secrets == nil {
		writeError(w, 503, "secrets_unavailable", "secret store unavailable")
		return
	}
	_, found, err := s.secrets.Get(r.Context(), "github_token")
	if err != nil {
		writeError(w, 500, "secret_error", err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"configured": found})
}
func (s *Server) setGithubToken(w http.ResponseWriter, r *http.Request) {
	if s.secrets == nil {
		writeError(w, 503, "secrets_unavailable", "secret store unavailable")
		return
	}
	var input struct {
		Token string `json:"token"`
	}
	if decodeJSON(w, r, &input) != nil {
		return
	}
	if err := s.secrets.Set(r.Context(), "github_token", input.Token); err != nil {
		writeError(w, 422, "secret_error", err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"configured": true})
}
func (s *Server) deleteGithubToken(w http.ResponseWriter, r *http.Request) {
	if s.secrets == nil {
		writeError(w, 503, "secrets_unavailable", "secret store unavailable")
		return
	}
	if err := s.secrets.Delete(r.Context(), "github_token"); err != nil {
		writeError(w, 500, "secret_error", err.Error())
		return
	}
	w.WriteHeader(204)
}

func (s *Server) listSchedules(w http.ResponseWriter, r *http.Request) {
	if s.schedules == nil {
		writeError(w, 503, "scheduler_unavailable", "scheduler unavailable")
		return
	}
	tasks, err := s.schedules.List(r.Context())
	if err != nil {
		writeError(w, 500, "schedule_error", err.Error())
		return
	}
	writeJSON(w, 200, tasks)
}
func (s *Server) saveSchedule(w http.ResponseWriter, r *http.Request) {
	if s.schedules == nil {
		writeError(w, 503, "scheduler_unavailable", "scheduler unavailable")
		return
	}
	var task domain.ScheduledTask
	if decodeJSON(w, r, &task) != nil {
		return
	}
	if task.ID == "" {
		task.ID = uuid.NewString()
	}
	if task.Timezone == "" {
		task.Timezone = "UTC"
	}
	if task.OnlinePolicy == "" {
		task.OnlinePolicy = "skip"
	}
	if task.Payload == "" {
		task.Payload = "{}"
	}
	if !json.Valid([]byte(task.Payload)) {
		writeError(w, 422, "invalid_payload", "payload must be JSON")
		return
	}
	if err := s.schedules.Save(r.Context(), task); err != nil {
		writeError(w, 422, "schedule_invalid", err.Error())
		return
	}
	writeJSON(w, 200, task)
}
func (s *Server) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	if s.schedules == nil {
		writeError(w, 503, "scheduler_unavailable", "scheduler unavailable")
		return
	}
	if err := s.schedules.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, 500, "schedule_error", err.Error())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) runSchedule(w http.ResponseWriter, r *http.Request) {
	if s.schedules == nil {
		writeError(w, 503, "scheduler_unavailable", "scheduler unavailable")
		return
	}
	if err := s.schedules.RunNow(context.WithoutCancel(r.Context()), chi.URLParam(r, "id")); err != nil {
		writeError(w, 422, "schedule_error", err.Error())
		return
	}
	writeJSON(w, 202, map[string]bool{"queued": true})
}

func (s *Server) updateGame(w http.ResponseWriter, r *http.Request) {
	if s.gameUpdates == nil || s.jobs == nil {
		writeError(w, 503, "updates_unavailable", "game update unavailable")
		return
	}
	var input struct {
		Confirm bool `json:"confirm"`
	}
	if decodeJSON(w, r, &input) != nil {
		return
	}
	if !input.Confirm {
		writeError(w, 428, "confirmation_required", "game update requires confirmation")
		return
	}
	id := chi.URLParam(r, "id")
	job := s.jobs.Start(context.WithoutCancel(r.Context()), id, "game_update", func(ctx context.Context, reporter jobs.Reporter) error {
		reporter.Progress("steamcmd", 10, "validating App 222860")
		return s.gameUpdates.Update(ctx, id)
	})
	writeJSON(w, 202, job)
}

func (s *Server) listPackages(w http.ResponseWriter, r *http.Request) {
	if s.packages == nil {
		writeError(w, 503, "packages_unavailable", "package manager unavailable")
		return
	}
	items, err := s.packages.List()
	if err != nil {
		writeError(w, 500, "package_error", err.Error())
		return
	}
	writeJSON(w, 200, items)
}
func (s *Server) uploadPackage(w http.ResponseWriter, r *http.Request) {
	if s.packages == nil {
		writeError(w, 503, "packages_unavailable", "package manager unavailable")
		return
	}
	if r.ContentLength < 1 || r.ContentLength > 2<<30 {
		writeError(w, 413, "invalid_size", "Content-Length is required and limited to 2 GiB")
		return
	}
	item, err := s.packages.AddUpload(r.URL.Query().Get("filename"), r.URL.Query().Get("version"), http.MaxBytesReader(w, r.Body, 2<<30), r.ContentLength)
	if err != nil {
		writeError(w, 422, "package_invalid", err.Error())
		return
	}
	writeJSON(w, 201, item)
}
func (s *Server) fetchRelease(w http.ResponseWriter, r *http.Request) {
	if s.packages == nil || s.jobs == nil {
		writeError(w, 503, "packages_unavailable", "package manager unavailable")
		return
	}
	var input struct {
		Repository   string `json:"repository"`
		AssetPattern string `json:"asset_pattern"`
	}
	if decodeJSON(w, r, &input) != nil {
		return
	}
	job := s.jobs.Start(context.WithoutCancel(r.Context()), "global", "release_fetch", func(ctx context.Context, reporter jobs.Reporter) error {
		reporter.Progress("release", 10, "checking GitHub Release")
		token := ""
		if s.secrets != nil {
			token, _, _ = s.secrets.Get(ctx, "github_token")
		}
		_, err := s.releases.FetchLatest(ctx, input.Repository, input.AssetPattern, token, s.packages)
		return err
	})
	writeJSON(w, 202, job)
}
func (s *Server) updatePackage(w http.ResponseWriter, r *http.Request) {
	if s.packages == nil || s.updateCoordinator == nil || s.jobs == nil {
		writeError(w, 503, "updates_unavailable", "update pipeline unavailable")
		return
	}
	var input struct {
		PackageID string       `json:"package_id"`
		Mode      updates.Mode `json:"mode"`
		Confirm   bool         `json:"confirm"`
	}
	if decodeJSON(w, r, &input) != nil {
		return
	}
	if input.Mode == updates.Full && !input.Confirm {
		writeError(w, 428, "confirmation_required", "full update requires confirmation")
		return
	}
	item, err := s.packages.Get(input.PackageID)
	if err != nil {
		writeError(w, 404, "package_not_found", err.Error())
		return
	}
	if input.Mode == updates.Hot && !item.HotCompatible {
		writeError(w, 422, "hot_update_forbidden", "package is not hot-update compatible")
		return
	}
	id := chi.URLParam(r, "id")
	job := s.jobs.Start(context.WithoutCancel(r.Context()), id, "package_"+string(input.Mode), func(ctx context.Context, reporter jobs.Reporter) error {
		reporter.Progress("deploy", 10, "deploying package")
		if err := s.updateCoordinator.ApplyPackage(ctx, id, item, input.Mode); err != nil {
			return err
		}
		instance, err := s.store.Instance(ctx, id)
		if err != nil {
			return err
		}
		instance.PackageVersion = item.ID
		return s.store.UpdateInstance(ctx, instance)
	})
	writeJSON(w, 202, job)
}

func (s *Server) listVPK(w http.ResponseWriter, r *http.Request) {
	if s.uploads == nil {
		writeError(w, 503, "content_unavailable", "content manager unavailable")
		return
	}
	items, err := s.uploads.List()
	if err != nil {
		writeError(w, 500, "content_error", err.Error())
		return
	}
	writeJSON(w, 200, items)
}
func (s *Server) beginVPK(w http.ResponseWriter, r *http.Request) {
	if s.uploads == nil {
		writeError(w, 503, "content_unavailable", "content manager unavailable")
		return
	}
	var input struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
		Hash string `json:"sha256"`
	}
	if decodeJSON(w, r, &input) != nil {
		return
	}
	session, err := s.uploads.Begin(input.Name, input.Size, input.Hash)
	if err != nil {
		writeError(w, 422, "invalid_upload", err.Error())
		return
	}
	writeJSON(w, 201, session)
}
func (s *Server) writeVPK(w http.ResponseWriter, r *http.Request) {
	offset, err := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	if err != nil {
		writeError(w, 422, "invalid_offset", "numeric offset required")
		return
	}
	written, err := s.uploads.Write(chi.URLParam(r, "id"), offset, http.MaxBytesReader(w, r.Body, 64<<20))
	if err != nil {
		writeError(w, 409, "upload_error", err.Error())
		return
	}
	writeJSON(w, 200, map[string]int64{"written": written, "next_offset": offset + written})
}
func (s *Server) completeVPK(w http.ResponseWriter, r *http.Request) {
	item, duplicate, err := s.uploads.Complete(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 422, "upload_incomplete", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"item": item, "duplicate": duplicate})
}
func (s *Server) renameVPK(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name    string `json:"name"`
		Confirm bool   `json:"confirm"`
	}
	if decodeJSON(w, r, &input) != nil {
		return
	}
	if !input.Confirm {
		writeError(w, 428, "confirmation_required", "renaming visible VPK requires confirmation")
		return
	}
	item, err := s.uploads.Rename(chi.URLParam(r, "name"), input.Name)
	if err != nil {
		writeError(w, 422, "rename_failed", err.Error())
		return
	}
	writeJSON(w, 200, item)
}
func (s *Server) deleteVPK(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("confirm") != "true" {
		writeError(w, 428, "confirmation_required", "deleting VPK requires confirmation")
		return
	}
	if err := s.uploads.Delete(chi.URLParam(r, "name")); err != nil {
		writeError(w, 422, "delete_failed", err.Error())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) savePrivate(w http.ResponseWriter, r *http.Request) {
	if s.private == nil {
		writeError(w, 503, "content_unavailable", "private manager unavailable")
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeError(w, 413, "file_too_large", err.Error())
		return
	}
	item, err := s.private.Save(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "*"), raw)
	if err != nil {
		writeError(w, 422, "private_file_error", err.Error())
		return
	}
	writeJSON(w, 200, item)
}
func (s *Server) applyPrivate(w http.ResponseWriter, r *http.Request) {
	if s.private == nil || s.jobs == nil {
		writeError(w, 503, "content_unavailable", "private manager unavailable")
		return
	}
	id := chi.URLParam(r, "id")
	job := s.jobs.Start(context.WithoutCancel(r.Context()), id, "apply_private", func(ctx context.Context, _ jobs.Reporter) error { return s.private.Apply(ctx, id) })
	writeJSON(w, 202, job)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (s *Server) auditMutations(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		wrapped := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(wrapped, r)
		metadata, _ := json.Marshal(map[string]string{"remote": r.RemoteAddr})
		_ = s.store.RecordAudit(context.WithoutCancel(r.Context()), domain.AuditRecord{ID: uuid.NewString(), Action: r.Method, Target: r.URL.Path, Result: strconv.Itoa(wrapped.status), Metadata: string(metadata)})
	})
}
func (s *Server) auditEvents(w http.ResponseWriter, r *http.Request) {
	events, err := s.store.AuditEvents(r.Context(), 100)
	if err != nil {
		writeError(w, 500, "audit_error", err.Error())
		return
	}
	writeJSON(w, 200, events)
}

func (s *Server) onlinePlayers(w http.ResponseWriter, r *http.Request) {
	if s.players == nil {
		writeError(w, 503, "players_unavailable", "player query unavailable")
		return
	}
	snapshot, err := s.players.Online(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 502, "player_query_failed", err.Error())
		return
	}
	writeJSON(w, 200, snapshot)
}
func (s *Server) playerAction(w http.ResponseWriter, r *http.Request) {
	if s.players == nil || s.jobs == nil {
		writeError(w, 503, "players_unavailable", "player operations unavailable")
		return
	}
	userID, err := strconv.Atoi(chi.URLParam(r, "userID"))
	if err != nil || userID < 1 {
		writeError(w, 422, "invalid_user_id", "numeric UserID required")
		return
	}
	var input struct {
		Action  string `json:"action"`
		Minutes int    `json:"minutes"`
		Confirm bool   `json:"confirm"`
	}
	if decodeJSON(w, r, &input) != nil {
		return
	}
	if !input.Confirm {
		writeError(w, http.StatusPreconditionRequired, "confirmation_required", "player action requires confirmation")
		return
	}
	id := chi.URLParam(r, "id")
	operation := func(ctx context.Context, _ jobs.Reporter) error {
		switch input.Action {
		case "kick":
			return s.players.Kick(ctx, id, userID)
		case "ban":
			return s.players.Ban(ctx, id, userID, input.Minutes)
		default:
			return errors.New("unsupported player action")
		}
	}
	if input.Action != "kick" && input.Action != "ban" {
		writeError(w, 422, "invalid_action", "supported actions: kick, ban")
		return
	}
	job := s.jobs.Start(context.WithoutCancel(r.Context()), id, "player_"+input.Action, operation)
	writeJSON(w, http.StatusAccepted, job)
}

var consoleUpgrader = websocket.Upgrader{ReadBufferSize: 4096, WriteBufferSize: 4096, CheckOrigin: func(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Host == r.Host
}}

func (s *Server) consoleSocket(w http.ResponseWriter, r *http.Request) {
	if s.console == nil {
		writeError(w, 503, "console_unavailable", "console adapter unavailable")
		return
	}
	instance, err := s.store.Instance(r.Context(), chi.URLParam(r, "id"))
	if err != nil || instance.ContainerID == "" {
		writeError(w, 404, "instance_not_running", "instance container unavailable")
		return
	}
	stream, err := s.console.AttachSupervisor(r.Context(), instance.ContainerID)
	if err != nil {
		writeError(w, 502, "console_attach_failed", err.Error())
		return
	}
	defer stream.Close()
	socket, err := consoleUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer socket.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		buffer := make([]byte, 16*1024)
		for {
			n, readErr := stream.Read(buffer)
			if n > 0 {
				if writeErr := socket.WriteMessage(websocket.BinaryMessage, buffer[:n]); writeErr != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}()
	for {
		messageType, payload, readErr := socket.ReadMessage()
		if readErr != nil {
			break
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}
		if len(payload) > 64*1024 {
			break
		}
		if _, err := stream.Write(payload); err != nil {
			break
		}
	}
	_ = stream.Close()
	<-done
}

func (s *Server) instanceAction(w http.ResponseWriter, r *http.Request) {
	if s.lifecycle == nil || s.jobs == nil {
		writeError(w, 503, "operations_unavailable", "container operations unavailable")
		return
	}
	var input struct {
		Action  string `json:"action"`
		Confirm bool   `json:"confirm"`
	}
	if decodeJSON(w, r, &input) != nil {
		return
	}
	if (input.Action == "stop" || input.Action == "restart") && !input.Confirm {
		writeError(w, http.StatusPreconditionRequired, "confirmation_required", "this action requires confirmation")
		return
	}
	id := chi.URLParam(r, "id")
	operation := func(ctx context.Context, _ jobs.Reporter) error {
		switch input.Action {
		case "start":
			return s.lifecycle.Start(ctx, id)
		case "stop":
			return s.lifecycle.Stop(ctx, id)
		case "restart":
			return s.lifecycle.Restart(ctx, id)
		default:
			return errors.New("unsupported action")
		}
	}
	if input.Action != "start" && input.Action != "stop" && input.Action != "restart" {
		writeError(w, 422, "invalid_action", "supported actions: start, stop, restart")
		return
	}
	job := s.jobs.Start(context.WithoutCancel(r.Context()), id, input.Action, operation)
	writeJSON(w, http.StatusAccepted, job)
}
func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	if s.jobs == nil {
		writeError(w, 503, "jobs_unavailable", "job manager unavailable")
		return
	}
	job, ok := s.jobs.Get(chi.URLParam(r, "id"))
	if !ok {
		writeError(w, 404, "job_not_found", "job not found")
		return
	}
	writeJSON(w, 200, job)
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
		Name         string `json:"name"`
		GamePort     int    `json:"game_port"`
		SourceTVPort int    `json:"sourcetv_port"`
		PluginPorts  []int  `json:"plugin_ports"`
		StartMap     string `json:"start_map"`
		GameMode     string `json:"game_mode"`
		Tickrate     int    `json:"tickrate"`
		MaxPlayers   int    `json:"max_players"`
	}
	if decodeJSON(w, r, &in) != nil {
		return
	}
	if in.Name == "" || in.StartMap == "" || in.GameMode == "" || in.Tickrate < 30 || in.Tickrate > 128 || in.MaxPlayers < 1 || in.MaxPlayers > 32 {
		writeError(w, 422, "invalid_instance", "name, valid port, map, mode, tickrate and player limit are required")
		return
	}
	if err := validateDeclaredPorts(in.GamePort, in.SourceTVPort, in.PluginPorts); err != nil {
		writeError(w, 422, "invalid_instance", err.Error())
		return
	}
	v := domain.Instance{ID: uuid.NewString(), NodeID: "local", Name: in.Name, GamePort: in.GamePort, SourceTVPort: in.SourceTVPort, PluginPorts: in.PluginPorts, StartMap: in.StartMap, GameMode: in.GameMode, Tickrate: in.Tickrate, MaxPlayers: in.MaxPlayers, RuntimeImage: "l4d2-server-runtime:latest", DesiredState: domain.StateStopped, ActualState: domain.StateUninstalled}
	if err := s.store.CreateInstance(r.Context(), v); err != nil {
		writeError(w, 409, "instance_conflict", err.Error())
		return
	}
	writeJSON(w, 201, v)
}

func validateDeclaredPorts(gamePort, sourceTVPort int, pluginPorts []int) error {
	ports := []int{gamePort}
	if sourceTVPort != 0 {
		ports = append(ports, sourceTVPort)
	}
	ports = append(ports, pluginPorts...)
	seen := make(map[int]struct{}, len(ports))
	for _, port := range ports {
		if port < 1024 || port > 65535 {
			return fmt.Errorf("port %d is outside the allowed range", port)
		}
		if _, exists := seen[port]; exists {
			return fmt.Errorf("port %d is declared more than once", port)
		}
		seen[port] = struct{}{}
	}
	return nil
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

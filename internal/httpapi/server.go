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
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/not0721here/l4d2-control-panel/internal/auth"
	"github.com/not0721here/l4d2-control-panel/internal/content"
	"github.com/not0721here/l4d2-control-panel/internal/docker"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
	"github.com/not0721here/l4d2-control-panel/internal/metrics"
	"github.com/not0721here/l4d2-control-panel/internal/players"
	"github.com/not0721here/l4d2-control-panel/internal/releases"
	"github.com/not0721here/l4d2-control-panel/internal/scheduler"
	"github.com/not0721here/l4d2-control-panel/internal/secrets"
	"github.com/not0721here/l4d2-control-panel/internal/srcds"
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
	privateUploads    *content.PrivateUploadManager
	updates           *updates.Pipeline
	packages          *content.PackageManager
	updateCoordinator *updates.Coordinator
	releases          releases.Client
	gameUpdates       *updates.GameCoordinator
	schedules         *scheduler.Service
	secrets           *secrets.Service
	resources         ResourceProvider
	performance       PerformanceProvider
	system            SystemProvider
	secureCookie      bool
}

func WithPrivateUploads(manager *content.PrivateUploadManager) Option {
	return func(s *Server) { s.privateUploads = manager }
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
	Summary(context.Context, string) (players.Summary, error)
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
	Running(context.Context, string) (bool, error)
	Stats(context.Context, string) (docker.ResourceStats, error)
}

func WithResources(provider ResourceProvider) Option {
	return func(s *Server) { s.resources = provider }
}

type PerformanceProvider interface {
	Latest(string) (metrics.Snapshot, bool)
	History(string) []metrics.Snapshot
}

func WithPerformance(provider PerformanceProvider) Option {
	return func(s *Server) { s.performance = provider }
}

type SystemProvider interface {
	Info(context.Context) (docker.Info, error)
}

func WithSystem(provider SystemProvider) Option { return func(s *Server) { s.system = provider } }

func WithSecureCookie(secure bool) Option { return func(s *Server) { s.secureCookie = secure } }

func New(db *store.Store, a *auth.Service, options ...Option) *Server {
	s := &Server{store: db, auth: a, secureCookie: true}
	for _, option := range options {
		option(s)
	}
	r := chi.NewRouter()
	r.Post("/api/auth/login", s.login)
	r.Get("/api/health", s.health)
	r.Group(func(r chi.Router) {
		r.Use(s.requireAuth)
		r.Use(s.auditMutations)
		r.Use(s.requireExistingPrivateInstance)
		r.Post("/api/auth/logout", s.logout)
		r.Get("/api/session", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
		})
		r.Get("/api/instances", s.listInstances)
		r.Get("/api/instances/{id}/overview", s.instanceOverview)
		r.Get("/api/instances/{id}/performance-history", s.instancePerformanceHistory)
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
		r.Get("/api/instances/{id}/private/tree", s.privateTree)
		r.Get("/api/instances/{id}/private/diff", s.privateDiff)
		r.Post("/api/instances/{id}/private/directories", s.makePrivateDirectory)
		r.Post("/api/instances/{id}/private/move", s.movePrivate)
		r.Post("/api/instances/{id}/private/uploads", s.beginPrivateUpload)
		r.Get("/api/instances/{id}/private/uploads/{uploadID}", s.recoverPrivateUpload)
		r.Patch("/api/instances/{id}/private/uploads/{uploadID}", s.writePrivateUpload)
		r.Post("/api/instances/{id}/private/uploads/{uploadID}/complete", s.completePrivateUpload)
		r.Get("/api/instances/{id}/private/snapshots", s.privateSnapshots)
		r.Post("/api/instances/{id}/private/snapshots/{snapshotID}/restore", s.restorePrivateSnapshot)
		r.Get("/api/instances/{id}/private/history/*", s.privateHistory)
		r.Get("/api/instances/{id}/private/file/*", s.downloadPrivate)
		r.Delete("/api/instances/{id}/private/file/*", s.deletePrivate)
		r.Post("/api/instances/{id}/private/apply", s.applyPrivate)
		r.Get("/api/packages", s.listPackages)
		r.Get("/api/github-sources", s.listGitHubSources)
		r.Post("/api/github-sources", s.createGitHubSource)
		r.Put("/api/github-sources/{id}", s.updateGitHubSource)
		r.Delete("/api/github-sources/{id}", s.deleteGitHubSource)
		r.Post("/api/github-sources/{id}/check", s.checkGitHubSource)
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
		r.Get("/api/settings/jobs", s.jobSettings)
		r.Put("/api/settings/jobs", s.setJobSettings)
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

type jobSettingsResponse struct {
	SuccessfulJobLimit int `json:"successful_job_limit"`
}

func (s *Server) jobSettings(w http.ResponseWriter, _ *http.Request) {
	limit, err := s.store.SuccessfulJobLimit()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "settings_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, jobSettingsResponse{SuccessfulJobLimit: limit})
}

func (s *Server) setJobSettings(w http.ResponseWriter, r *http.Request) {
	var input jobSettingsResponse
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_job_settings", "successful_job_limit must be an integer between 1 and 500")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusUnprocessableEntity, "invalid_job_settings", "request body must contain exactly one job settings object")
		return
	}
	if input.SuccessfulJobLimit < store.MinSuccessfulJobLimit || input.SuccessfulJobLimit > store.MaxSuccessfulJobLimit {
		writeError(w, http.StatusUnprocessableEntity, "invalid_job_limit", "successful_job_limit must be between 1 and 500")
		return
	}
	if err := s.store.SetSuccessfulJobLimit(input.SuccessfulJobLimit); err != nil {
		writeError(w, http.StatusInternalServerError, "settings_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, input)
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
	name, err := decodedURLParam(r, "name")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_vpk_name", err.Error())
		return
	}
	path, err := s.uploads.Path(name)
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
	name := chi.URLParam(r, "*")
	if s.privateUploads == nil {
		raw, err := s.private.Read(r.Context(), chi.URLParam(r, "id"), name)
		if err != nil {
			writeError(w, http.StatusNotFound, "private_not_found", err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(name)))
		_, _ = w.Write(raw)
		return
	}
	file, info, err := s.privateUploads.Open(chi.URLParam(r, "id"), name)
	if err != nil {
		writeError(w, http.StatusNotFound, "private_not_found", err.Error())
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(name)))
	http.ServeContent(w, r, filepath.Base(name), info.ModTime(), file)
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

type instanceOverview struct {
	ActualState           domain.InstanceState `json:"actual_state"`
	ContainerRunning      bool                 `json:"container_running"`
	ContainerRunningKnown bool                 `json:"container_running_known"`
	SampledAt             *time.Time           `json:"sampled_at"`
	RunID                 *string              `json:"run_id"`
	Map                   string               `json:"map,omitempty"`
	Players               *int                 `json:"players"`
	MaxPlayers            *int                 `json:"max_players"`
	CPUPercent            *float64             `json:"cpu_percent"`
	MemoryBytes           *uint64              `json:"memory_bytes"`
	MemoryLimitBytes      *uint64              `json:"memory_limit_bytes"`
	MemoryPercent         *float64             `json:"memory_percent"`
	NetworkRXBytesPerSec  *float64             `json:"network_rx_bytes_per_sec"`
	NetworkTXBytesPerSec  *float64             `json:"network_tx_bytes_per_sec"`
	NetworkRXBytes        *uint64              `json:"network_rx_bytes"`
	NetworkTXBytes        *uint64              `json:"network_tx_bytes"`
	BlockReadBytesPerSec  *float64             `json:"block_read_bytes_per_sec"`
	BlockWriteBytesPerSec *float64             `json:"block_write_bytes_per_sec"`
	BlockReadBytes        *uint64              `json:"block_read_bytes"`
	BlockWriteBytes       *uint64              `json:"block_write_bytes"`
	PIDs                  *uint64              `json:"pids"`
	UptimeSeconds         *uint64              `json:"uptime_seconds"`
	A2SLatencyMS          *float64             `json:"a2s_latency_ms"`
	Issues                []string             `json:"issues,omitempty"`
}

func (s *Server) instanceOverview(w http.ResponseWriter, r *http.Request) {
	instance, err := s.store.Instance(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "instance_not_found", "instance not found")
		return
	}
	result := instanceOverview{ActualState: instance.ActualState}
	if instance.ContainerID == "" {
		if instance.ActualState == domain.StateRunning || instance.DesiredState == domain.StateRunning {
			result.ActualState = domain.StateOrphaned
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	if s.performance == nil {
		writeError(w, http.StatusServiceUnavailable, "performance_unavailable", "performance provider unavailable")
		return
	}
	snapshot, ok := s.performance.Latest(instance.ID)
	if !ok {
		writeJSON(w, http.StatusOK, result)
		return
	}
	result = overviewFromSnapshot(instance.ActualState, snapshot)
	if snapshot.ContainerRunning == nil {
		result.ActualState = instance.ActualState
	} else if !*snapshot.ContainerRunning {
		result.ActualState = stoppedObservationState(instance.ActualState)
	} else if a2sAvailable(snapshot) {
		result.ActualState = domain.StateRunning
	} else {
		result.ActualState = unhealthyObservationState(instance.ActualState)
	}
	writeJSON(w, http.StatusOK, result)
}

func overviewFromSnapshot(actualState domain.InstanceState, snapshot metrics.Snapshot) instanceOverview {
	result := instanceOverview{
		ActualState: actualState, Players: snapshot.Players, MaxPlayers: snapshot.MaxPlayers,
		CPUPercent: snapshot.CPUPercent, MemoryBytes: snapshot.MemoryBytes, MemoryLimitBytes: snapshot.MemoryLimitBytes, MemoryPercent: snapshot.MemoryPercent,
		NetworkRXBytesPerSec: snapshot.NetworkRXBytesPerSecond, NetworkTXBytesPerSec: snapshot.NetworkTXBytesPerSecond, NetworkRXBytes: snapshot.NetworkRXBytes, NetworkTXBytes: snapshot.NetworkTXBytes,
		BlockReadBytesPerSec: snapshot.BlockReadBytesPerSecond, BlockWriteBytesPerSec: snapshot.BlockWriteBytesPerSecond, BlockReadBytes: snapshot.BlockReadBytes, BlockWriteBytes: snapshot.BlockWriteBytes,
		PIDs: snapshot.PIDs, UptimeSeconds: snapshot.UptimeSeconds, A2SLatencyMS: snapshot.A2SLatencyMS,
	}
	if !snapshot.Timestamp.IsZero() {
		at := snapshot.Timestamp.UTC()
		result.SampledAt = &at
	}
	if snapshot.RunID != "" {
		runID := snapshot.RunID
		result.RunID = &runID
	}
	if snapshot.ContainerRunning != nil {
		result.ContainerRunning = *snapshot.ContainerRunning
		result.ContainerRunningKnown = true
	}
	if snapshot.Map != nil {
		result.Map = *snapshot.Map
	}
	for _, issue := range snapshot.Issues {
		result.Issues = append(result.Issues, issue.Source+": "+issue.Message)
	}
	return result
}

func a2sAvailable(snapshot metrics.Snapshot) bool {
	for _, issue := range snapshot.Issues {
		if issue.Source == "a2s" {
			return false
		}
	}
	return snapshot.Map != nil && snapshot.Players != nil && snapshot.MaxPlayers != nil
}

type performanceHistoryPoint struct {
	At                    time.Time `json:"at"`
	RunID                 string    `json:"run_id"`
	CPUPercent            *float64  `json:"cpu_percent"`
	MemoryPercent         *float64  `json:"memory_percent"`
	NetworkRXBytesPerSec  *float64  `json:"network_rx_bytes_per_sec"`
	NetworkTXBytesPerSec  *float64  `json:"network_tx_bytes_per_sec"`
	BlockReadBytesPerSec  *float64  `json:"block_read_bytes_per_sec"`
	BlockWriteBytesPerSec *float64  `json:"block_write_bytes_per_sec"`
}

func (s *Server) instancePerformanceHistory(w http.ResponseWriter, r *http.Request) {
	if _, err := s.store.Instance(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusNotFound, "instance_not_found", "instance not found")
		return
	}
	if s.performance == nil {
		writeError(w, http.StatusServiceUnavailable, "performance_unavailable", "performance provider unavailable")
		return
	}
	snapshots := slices.Clone(s.performance.History(chi.URLParam(r, "id")))
	sort.SliceStable(snapshots, func(i, j int) bool {
		return snapshots[i].Timestamp.Before(snapshots[j].Timestamp)
	})
	if len(snapshots) > 720 {
		snapshots = snapshots[len(snapshots)-720:]
	}
	points := make([]performanceHistoryPoint, len(snapshots))
	for i, snapshot := range snapshots {
		points[i] = performanceHistoryPoint{
			At: snapshot.Timestamp.UTC(), RunID: snapshot.RunID, CPUPercent: snapshot.CPUPercent, MemoryPercent: snapshot.MemoryPercent,
			NetworkRXBytesPerSec: snapshot.NetworkRXBytesPerSecond, NetworkTXBytesPerSec: snapshot.NetworkTXBytesPerSecond,
			BlockReadBytesPerSec: snapshot.BlockReadBytesPerSecond, BlockWriteBytesPerSec: snapshot.BlockWriteBytesPerSecond,
		}
	}
	writeJSON(w, http.StatusOK, points)
}

func stoppedObservationState(current domain.InstanceState) domain.InstanceState {
	switch current {
	case domain.StateUninstalled, domain.StateInstalling, domain.StateUpdating, domain.StateRollingBack, domain.StateFaulted, domain.StateOrphaned:
		return current
	default:
		return domain.StateStopped
	}
}

func unhealthyObservationState(current domain.InstanceState) domain.InstanceState {
	switch current {
	case domain.StateInstalling, domain.StateStarting, domain.StateUpdating, domain.StateRollingBack:
		return current
	default:
		return domain.StateFaulted
	}
}

type instanceInput struct {
	Name         string `json:"name"`
	GamePort     int    `json:"game_port"`
	SourceTVPort int    `json:"sourcetv_port"`
	PluginPorts  []int  `json:"plugin_ports"`
	StartMap     string `json:"start_map"`
	GameMode     string `json:"game_mode"`
	Tickrate     int    `json:"tickrate"`
	MaxPlayers   int    `json:"max_players"`
	ExtraArgs    string `json:"extra_args"`
	PackageID    string `json:"package_id"`
}

func (s *Server) validateInstanceInput(input *instanceInput) (content.PackageVersion, error) {
	if input.Name == "" || input.StartMap == "" || input.GameMode == "" || input.Tickrate < 30 || input.Tickrate > 128 || input.MaxPlayers < 1 || input.MaxPlayers > 32 {
		return content.PackageVersion{}, errors.New("invalid instance configuration")
	}
	if err := validateDeclaredPorts(input.GamePort, input.SourceTVPort, input.PluginPorts); err != nil {
		return content.PackageVersion{}, err
	}
	if _, err := srcds.ParseExtraArgs(input.ExtraArgs); err != nil {
		return content.PackageVersion{}, err
	}
	item, err := s.packages.Get(input.PackageID)
	if err != nil {
		return content.PackageVersion{}, fmt.Errorf("invalid package: %w", err)
	}
	slices.Sort(input.PluginPorts)
	return item, nil
}

func (input instanceInput) apply(instance domain.Instance) domain.Instance {
	instance.Name = input.Name
	instance.GamePort = input.GamePort
	instance.SourceTVPort = input.SourceTVPort
	instance.PluginPorts = append([]int(nil), input.PluginPorts...)
	instance.StartMap = input.StartMap
	instance.GameMode = input.GameMode
	instance.Tickrate = input.Tickrate
	instance.MaxPlayers = input.MaxPlayers
	instance.ExtraArgs = input.ExtraArgs
	instance.SelectedPackageID = input.PackageID
	return instance
}

func runtimeConfigurationChanged(before, after domain.Instance) bool {
	return before.GamePort != after.GamePort ||
		before.SourceTVPort != after.SourceTVPort ||
		!slices.Equal(before.PluginPorts, after.PluginPorts) ||
		before.StartMap != after.StartMap ||
		before.GameMode != after.GameMode ||
		before.Tickrate != after.Tickrate ||
		before.MaxPlayers != after.MaxPlayers ||
		before.ExtraArgs != after.ExtraArgs
}

func (s *Server) updateInstance(w http.ResponseWriter, r *http.Request) {
	instance, err := s.store.Instance(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 404, "instance_not_found", err.Error())
		return
	}
	if s.packages == nil {
		writeError(w, 503, "packages_unavailable", "package manager unavailable")
		return
	}
	var input instanceInput
	if decodeJSON(w, r, &input) != nil {
		return
	}
	item, err := s.validateInstanceInput(&input)
	if err != nil {
		writeError(w, 422, "invalid_instance", err.Error())
		return
	}
	next := input.apply(instance)
	runtimeChanged := runtimeConfigurationChanged(instance, next)
	packageNeedsApply := instance.SelectedPackageID != input.PackageID || instance.PackageVersion != input.PackageID
	requiresJob := instance.ContainerID != "" && (runtimeChanged || packageNeedsApply)
	if requiresJob && s.jobs == nil {
		writeError(w, 503, "operations_unavailable", "job manager unavailable")
		return
	}
	if instance.ContainerID != "" && runtimeChanged && s.lifecycle == nil {
		writeError(w, 503, "operations_unavailable", "lifecycle unavailable")
		return
	}
	if instance.ContainerID != "" && packageNeedsApply && s.updateCoordinator == nil {
		writeError(w, 503, "updates_unavailable", "update pipeline unavailable")
		return
	}
	if err := s.store.UpdateInstance(r.Context(), next); err != nil {
		writeError(w, 409, "instance_conflict", err.Error())
		return
	}
	if !requiresJob {
		writeJSON(w, 200, next)
		return
	}
	job, ok := s.startJob(w, r, instance.ID, "reconfigure", func(ctx context.Context, reporter jobs.Reporter) error {
		if packageNeedsApply {
			reporter.Progress("package", 20, "deploying selected package")
			if err := s.updateCoordinator.ApplyPackage(ctx, instance.ID, item, updates.Full); err != nil {
				return err
			}
		}
		if runtimeChanged {
			reporter.Progress("container", 70, "rebuilding game container")
			return s.lifecycle.Rebuild(ctx, instance.ID)
		}
		return nil
	})
	if !ok {
		return
	}
	writeJSON(w, 202, job)
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
	job, ok := s.startJob(w, r, id, "delete", func(ctx context.Context, _ jobs.Reporter) error { return s.lifecycle.Delete(ctx, id, input.DeleteData) })
	if !ok {
		return
	}
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
		Confirm          bool  `json:"confirm"`
		ReinstallGame    *bool `json:"reinstall_game"`
		ReinstallPackage *bool `json:"reinstall_package"`
	}
	if decodeJSON(w, r, &input) != nil {
		return
	}
	if !input.Confirm {
		writeError(w, 428, "confirmation_required", "game update requires confirmation")
		return
	}
	options := updates.ReinstallOptions{}
	if input.ReinstallGame == nil && input.ReinstallPackage == nil {
		options.Game = true
	} else {
		options.Game = input.ReinstallGame != nil && *input.ReinstallGame
		options.Package = input.ReinstallPackage != nil && *input.ReinstallPackage
	}
	if !options.Game && !options.Package {
		writeError(w, 422, "reinstall_target_required", "at least one reinstall target is required")
		return
	}
	id := chi.URLParam(r, "id")
	job, ok := s.startJob(w, r, id, "game_update", func(ctx context.Context, reporter jobs.Reporter) error {
		reporter.Progress("reinstall", 10, "reinstalling selected instance components")
		return s.gameUpdates.Reinstall(ctx, id, options)
	})
	if !ok {
		return
	}
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
	job, ok := s.startJob(w, r, "global", "release_fetch", func(ctx context.Context, reporter jobs.Reporter) error {
		reporter.Progress("release", 10, "checking GitHub Release")
		token := ""
		if s.secrets != nil {
			token, _, _ = s.secrets.Get(ctx, "github_token")
		}
		_, err := s.releases.FetchLatest(ctx, input.Repository, input.AssetPattern, token, s.packages)
		return err
	})
	if !ok {
		return
	}
	writeJSON(w, 202, job)
}

func (s *Server) listGitHubSources(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.GitHubSources(r.Context())
	if err != nil {
		writeError(w, 500, "source_error", err.Error())
		return
	}
	writeJSON(w, 200, items)
}
func decodeGitHubSource(w http.ResponseWriter, r *http.Request) (domain.GitHubSource, bool) {
	var input domain.GitHubSource
	if decodeJSON(w, r, &input) != nil {
		return input, false
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Repository = strings.TrimSpace(input.Repository)
	input.AssetPattern = strings.TrimSpace(input.AssetPattern)
	if input.Name == "" || !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(input.Repository) {
		writeError(w, 422, "source_invalid", "name and valid owner/repository are required")
		return input, false
	}
	if _, err := regexp.Compile(input.AssetPattern); err != nil || input.AssetPattern == "" {
		writeError(w, 422, "source_invalid", "valid asset pattern is required")
		return input, false
	}
	return input, true
}
func (s *Server) createGitHubSource(w http.ResponseWriter, r *http.Request) {
	input, ok := decodeGitHubSource(w, r)
	if !ok {
		return
	}
	input.ID = uuid.NewString()
	if err := s.store.SaveGitHubSource(r.Context(), input); err != nil {
		writeError(w, 500, "source_error", err.Error())
		return
	}
	created, _ := s.store.GitHubSource(r.Context(), input.ID)
	writeJSON(w, 201, created)
}
func (s *Server) updateGitHubSource(w http.ResponseWriter, r *http.Request) {
	input, ok := decodeGitHubSource(w, r)
	if !ok {
		return
	}
	input.ID = chi.URLParam(r, "id")
	if _, err := s.store.GitHubSource(r.Context(), input.ID); err != nil {
		writeError(w, 404, "source_not_found", "GitHub source not found")
		return
	}
	if err := s.store.SaveGitHubSource(r.Context(), input); err != nil {
		writeError(w, 500, "source_error", err.Error())
		return
	}
	updated, _ := s.store.GitHubSource(r.Context(), input.ID)
	writeJSON(w, 200, updated)
}
func (s *Server) deleteGitHubSource(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteGitHubSource(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeError(w, 404, "source_not_found", "GitHub source not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) checkGitHubSource(w http.ResponseWriter, r *http.Request) {
	if s.packages == nil || s.jobs == nil {
		writeError(w, 503, "packages_unavailable", "package manager unavailable")
		return
	}
	source, err := s.store.GitHubSource(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 404, "source_not_found", "GitHub source not found")
		return
	}
	job, ok := s.startJob(w, r, "global", "release_fetch", func(ctx context.Context, reporter jobs.Reporter) error {
		reporter.Progress("release", 10, "checking GitHub Release")
		token := ""
		if s.secrets != nil {
			token, _, _ = s.secrets.Get(ctx, "github_token")
		}
		_, err := s.releases.FetchLatest(ctx, source.Repository, source.AssetPattern, token, s.packages)
		return err
	})
	if !ok {
		return
	}
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
	job, ok := s.startJob(w, r, id, "package_"+string(input.Mode), func(ctx context.Context, reporter jobs.Reporter) error {
		reporter.Progress("deploy", 10, "deploying package")
		return s.updateCoordinator.ApplyPackage(ctx, id, item, input.Mode)
	})
	if !ok {
		return
	}
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
	name, err := decodedURLParam(r, "name")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_vpk_name", err.Error())
		return
	}
	item, err := s.uploads.Rename(name, input.Name)
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
	name, err := decodedURLParam(r, "name")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_vpk_name", err.Error())
		return
	}
	if err := s.uploads.Delete(name); err != nil {
		writeError(w, 422, "delete_failed", err.Error())
		return
	}
	w.WriteHeader(204)
}

func decodedURLParam(r *http.Request, name string) (string, error) {
	return url.PathUnescape(chi.URLParam(r, name))
}
func (s *Server) savePrivate(w http.ResponseWriter, r *http.Request) {
	if s.private == nil {
		writeError(w, 503, "content_unavailable", "private manager unavailable")
		return
	}
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	if mediaType != "text/plain" && mediaType != "application/json" {
		writeError(w, 415, "unsupported_media_type", "private editor accepts UTF-8 text/plain")
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeError(w, 413, "file_too_large", err.Error())
		return
	}
	if !utf8.Valid(raw) {
		writeError(w, 422, "invalid_utf8", "private editor requires UTF-8 text")
		return
	}
	item, err := s.private.Save(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "*"), raw)
	if err != nil {
		writeError(w, 422, "private_file_error", err.Error())
		return
	}
	writeJSON(w, 200, item)
}

type privatePathRequest struct {
	Path    string `json:"path"`
	Confirm bool   `json:"confirm,omitempty"`
}
type privateMoveRequest struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Overwrite bool   `json:"overwrite"`
	Confirm   bool   `json:"confirm,omitempty"`
}
type privateUploadRequest struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func (s *Server) privateTree(w http.ResponseWriter, r *http.Request) {
	if s.private == nil {
		writeError(w, 503, "content_unavailable", "private manager unavailable")
		return
	}
	items, err := s.private.Tree(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 422, "private_path_error", err.Error())
		return
	}
	writeJSON(w, 200, items)
}
func (s *Server) privateDiff(w http.ResponseWriter, r *http.Request) {
	if s.private == nil {
		writeError(w, 503, "content_unavailable", "private manager unavailable")
		return
	}
	item, err := s.private.Diff(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 422, "private_path_error", err.Error())
		return
	}
	writeJSON(w, 200, item)
}
func (s *Server) makePrivateDirectory(w http.ResponseWriter, r *http.Request) {
	if s.private == nil {
		writeError(w, 503, "content_unavailable", "private manager unavailable")
		return
	}
	var input privatePathRequest
	if decodeJSON(w, r, &input) != nil {
		return
	}
	if err := s.private.MakeDir(r.Context(), chi.URLParam(r, "id"), input.Path); err != nil {
		writeError(w, 422, "private_path_error", err.Error())
		return
	}
	writeJSON(w, 201, map[string]string{"path": filepath.ToSlash(input.Path)})
}
func (s *Server) movePrivate(w http.ResponseWriter, r *http.Request) {
	if s.private == nil {
		writeError(w, 503, "content_unavailable", "private manager unavailable")
		return
	}
	var input privateMoveRequest
	if decodeJSON(w, r, &input) != nil {
		return
	}
	if input.Overwrite && !input.Confirm {
		writeError(w, 428, "confirmation_required", "overwriting a private path requires confirmation")
		return
	}
	if err := s.private.Move(r.Context(), chi.URLParam(r, "id"), input.From, input.To, input.Overwrite); err != nil {
		writeError(w, 409, "private_move_conflict", err.Error())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) beginPrivateUpload(w http.ResponseWriter, r *http.Request) {
	if s.privateUploads == nil {
		writeError(w, 503, "content_unavailable", "private upload manager unavailable")
		return
	}
	var input privateUploadRequest
	if decodeJSON(w, r, &input) != nil {
		return
	}
	session, err := s.privateUploads.Begin(chi.URLParam(r, "id"), input.Path, input.Size, input.SHA256)
	if err != nil {
		writeError(w, 422, "invalid_upload", err.Error())
		return
	}
	writeJSON(w, 201, session)
}
func (s *Server) recoverPrivateUpload(w http.ResponseWriter, r *http.Request) {
	if s.privateUploads == nil {
		writeError(w, 503, "content_unavailable", "private upload manager unavailable")
		return
	}
	session, err := s.privateUploads.Recover(chi.URLParam(r, "uploadID"))
	if err != nil || session.InstanceID != chi.URLParam(r, "id") {
		writeError(w, 404, "upload_not_found", "upload session not found")
		return
	}
	writeJSON(w, 200, session)
}
func (s *Server) writePrivateUpload(w http.ResponseWriter, r *http.Request) {
	if s.privateUploads == nil {
		writeError(w, 503, "content_unavailable", "private upload manager unavailable")
		return
	}
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	if mediaType != "application/offset+octet-stream" && mediaType != "application/octet-stream" {
		writeError(w, 415, "unsupported_media_type", "upload chunks require application/offset+octet-stream")
		return
	}
	session, err := s.privateUploads.Recover(chi.URLParam(r, "uploadID"))
	if err != nil || session.InstanceID != chi.URLParam(r, "id") {
		writeError(w, 404, "upload_not_found", "upload session not found")
		return
	}
	offset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil {
		writeError(w, 422, "invalid_offset", "numeric Upload-Offset required")
		return
	}
	written, err := s.privateUploads.Write(session.ID, offset, http.MaxBytesReader(w, r.Body, session.Size-session.Offset+1))
	if err != nil {
		writeError(w, 409, "upload_offset_error", err.Error())
		return
	}
	w.Header().Set("Upload-Offset", strconv.FormatInt(offset+written, 10))
	w.WriteHeader(204)
}
func (s *Server) completePrivateUpload(w http.ResponseWriter, r *http.Request) {
	if s.privateUploads == nil {
		writeError(w, 503, "content_unavailable", "private upload manager unavailable")
		return
	}
	session, err := s.privateUploads.Recover(chi.URLParam(r, "uploadID"))
	if err != nil || session.InstanceID != chi.URLParam(r, "id") {
		writeError(w, 404, "upload_not_found", "upload session not found")
		return
	}
	if err = s.privateUploads.Complete(session.ID); err != nil {
		writeError(w, 422, "upload_incomplete", err.Error())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) privateSnapshots(w http.ResponseWriter, r *http.Request) {
	if s.private == nil {
		writeError(w, 503, "content_unavailable", "private manager unavailable")
		return
	}
	items, err := s.private.Snapshots(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, 422, "snapshot_error", err.Error())
		return
	}
	writeJSON(w, 200, items)
}
func (s *Server) restorePrivateSnapshot(w http.ResponseWriter, r *http.Request) {
	if s.private == nil {
		writeError(w, 503, "content_unavailable", "private manager unavailable")
		return
	}
	var input struct {
		Confirm bool `json:"confirm"`
	}
	if decodeJSON(w, r, &input) != nil {
		return
	}
	if !input.Confirm {
		writeError(w, 428, "confirmation_required", "snapshot restore requires confirmation")
		return
	}
	if err := s.private.RestoreSnapshot(r.Context(), chi.URLParam(r, "id"), chi.URLParam(r, "snapshotID")); err != nil {
		writeError(w, 422, "snapshot_error", err.Error())
		return
	}
	w.WriteHeader(204)
}
func (s *Server) applyPrivate(w http.ResponseWriter, r *http.Request) {
	if s.private == nil || s.jobs == nil {
		writeError(w, 503, "content_unavailable", "private manager unavailable")
		return
	}
	id := chi.URLParam(r, "id")
	job, ok := s.startJob(w, r, id, "apply_private", func(ctx context.Context, reporter jobs.Reporter) error {
		percent := map[string]int{"snapshot": 10, "restore-lower": 35, "apply-private": 65, "commit": 90}
		return s.private.ApplyChangesWithProgress(ctx, id, func(stage string) { reporter.Progress(stage, percent[stage], stage) })
	})
	if !ok {
		return
	}
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
	job, ok := s.startJob(w, r, id, "player_"+input.Action, operation)
	if !ok {
		return
	}
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
	job, ok := s.startJob(w, r, id, input.Action, operation)
	if !ok {
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) startJob(w http.ResponseWriter, r *http.Request, instanceID, kind string, operation func(context.Context, jobs.Reporter) error) (jobs.Job, bool) {
	job, err := s.jobs.Start(context.WithoutCancel(r.Context()), instanceID, kind, operation)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "job_persistence_failed", err.Error())
		return jobs.Job{}, false
	}
	return job, true
}
func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	if s.jobs == nil {
		writeError(w, 503, "jobs_unavailable", "job manager unavailable")
		return
	}
	job, events, ok, err := s.jobs.Details(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "jobs_error", err.Error())
		return
	}
	if !ok {
		writeError(w, 404, "job_not_found", "job not found")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		jobs.Job
		Events []domain.JobEvent
	}{Job: job, Events: events})
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
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: s.secureCookie, SameSite: http.SameSiteStrictMode, MaxAge: 86400})
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.auth.Logout(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", HttpOnly: true, Secure: s.secureCookie, SameSite: http.SameSiteStrictMode, MaxAge: -1})
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
func (s *Server) requireExistingPrivateInstance(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/instances/") && strings.Contains(r.URL.Path, "/private") {
			id := chi.URLParam(r, "id")
			if id == "" { // URL params are populated after route matching; derive only for this middleware.
				parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/instances/"), "/")
				if len(parts) > 0 {
					id = parts[0]
				}
			}
			if _, err := s.store.Instance(r.Context(), id); errors.Is(err, store.ErrNotFound) {
				writeError(w, 404, "instance_not_found", "instance not found")
				return
			} else if err != nil {
				writeError(w, 500, "store_error", "instance lookup failed")
				return
			}
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
	if s.packages == nil {
		writeError(w, 503, "packages_unavailable", "package manager unavailable")
		return
	}
	var in instanceInput
	if decodeJSON(w, r, &in) != nil {
		return
	}
	if _, err := s.validateInstanceInput(&in); err != nil {
		writeError(w, 422, "invalid_instance", err.Error())
		return
	}
	v := in.apply(domain.Instance{ID: uuid.NewString(), NodeID: "local", RuntimeImage: "l4d2-server-runtime:latest", DesiredState: domain.StateStopped, ActualState: domain.StateUninstalled})
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

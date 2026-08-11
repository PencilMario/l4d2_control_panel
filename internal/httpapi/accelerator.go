package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/not0721here/l4d2-control-panel/internal/crashreports"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
	"github.com/not0721here/l4d2-control-panel/internal/store"
)

type crashReportDetails struct {
	crashreports.Report
	Metadata string `json:"metadata"`
}

type acceleratorSettingsResponse struct {
	DownloadURL    string `json:"download_url"`
	UseGitHubProxy bool   `json:"use_github_proxy"`
}

type crashAnalysisSettingsResponse struct {
	Endpoint  string `json:"endpoint"`
	Model     string `json:"model"`
	APIKeySet bool   `json:"api_key_set"`
}

type crashAnalysisSettingsInput struct {
	Endpoint    string  `json:"endpoint"`
	Model       string  `json:"model"`
	APIKey      *string `json:"api_key"`
	ClearAPIKey bool    `json:"clear_api_key"`
}

func (s *Server) getAcceleratorSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.AcceleratorSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "settings_error", "Accelerator settings unavailable")
		return
	}
	writeJSON(w, http.StatusOK, acceleratorSettingsResponse{
		DownloadURL: settings.DownloadURL, UseGitHubProxy: settings.UseGitHubProxy,
	})
}

func (s *Server) putAcceleratorSettings(w http.ResponseWriter, r *http.Request) {
	var input acceleratorSettingsResponse
	if decodeJSON(w, r, &input) != nil {
		return
	}
	if err := s.store.SaveAcceleratorSettings(r.Context(), store.AcceleratorSettings{
		DownloadURL: input.DownloadURL, UseGitHubProxy: input.UseGitHubProxy,
	}); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_accelerator_settings", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, input)
}

func (s *Server) getCrashAnalysisSettings(w http.ResponseWriter, r *http.Request) {
	if s.secrets == nil {
		writeError(w, http.StatusServiceUnavailable, "secrets_unavailable", "secret store unavailable")
		return
	}
	settings, err := s.store.CrashAnalysisSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "settings_error", "crash analysis settings unavailable")
		return
	}
	_, configured, err := s.secrets.Get(r.Context(), "accelerator_ai_api_key")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "secret_error", "crash analysis key status unavailable")
		return
	}
	writeJSON(w, http.StatusOK, crashAnalysisSettingsResponse{
		Endpoint: settings.Endpoint, Model: settings.Model, APIKeySet: configured,
	})
}

func (s *Server) putCrashAnalysisSettings(w http.ResponseWriter, r *http.Request) {
	if s.secrets == nil {
		writeError(w, http.StatusServiceUnavailable, "secrets_unavailable", "secret store unavailable")
		return
	}
	var input crashAnalysisSettingsInput
	if decodeJSON(w, r, &input) != nil {
		return
	}
	if input.APIKey != nil && input.ClearAPIKey {
		writeError(w, http.StatusUnprocessableEntity, "invalid_crash_analysis_settings", "api_key and clear_api_key cannot be used together")
		return
	}
	settings := store.CrashAnalysisSettings{Endpoint: input.Endpoint, Model: input.Model}
	if err := store.ValidateCrashAnalysisSettings(settings); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_crash_analysis_settings", err.Error())
		return
	}
	oldKey, oldKeySet, err := s.secrets.Get(r.Context(), "accelerator_ai_api_key")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "secret_error", "crash analysis key status unavailable")
		return
	}
	secretChanged := false
	restoreSecret := func() {
		if !secretChanged {
			return
		}
		if oldKeySet {
			_ = s.secrets.Set(context.Background(), "accelerator_ai_api_key", oldKey)
		} else {
			_ = s.secrets.Delete(context.Background(), "accelerator_ai_api_key")
		}
	}
	if input.APIKey != nil {
		if strings.TrimSpace(*input.APIKey) == "" {
			writeError(w, http.StatusUnprocessableEntity, "invalid_crash_analysis_key", "api_key cannot be empty")
			return
		}
		if err := s.secrets.Set(r.Context(), "accelerator_ai_api_key", *input.APIKey); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "invalid_crash_analysis_key", err.Error())
			return
		}
		secretChanged = true
	} else if input.ClearAPIKey {
		if err := s.secrets.Delete(r.Context(), "accelerator_ai_api_key"); err != nil {
			writeError(w, http.StatusInternalServerError, "secret_error", "failed to clear crash analysis key")
			return
		}
		secretChanged = true
	}
	if err := s.store.SaveCrashAnalysisSettings(r.Context(), settings); err != nil {
		restoreSecret()
		writeError(w, http.StatusUnprocessableEntity, "invalid_crash_analysis_settings", err.Error())
		return
	}
	settings, err = s.store.CrashAnalysisSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "settings_error", "crash analysis settings unavailable")
		return
	}
	_, configured, err := s.secrets.Get(r.Context(), "accelerator_ai_api_key")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "secret_error", "crash analysis key status unavailable")
		return
	}
	writeJSON(w, http.StatusOK, crashAnalysisSettingsResponse{Endpoint: settings.Endpoint, Model: settings.Model, APIKeySet: configured})
}

func matchesCrashReportQuery(report crashreports.Report, r *http.Request) bool {
	query := r.URL.Query()
	instanceID := query.Get("instance_id")
	if instanceID == "" {
		instanceID = query.Get("instance")
	}
	if instanceID != "" && report.InstanceID != instanceID {
		return false
	}
	if signature := query.Get("signature"); signature != "" && !strings.Contains(report.CrashSignature, signature) {
		return false
	}
	if status := query.Get("status"); status != "" && report.StackwalkStatus != crashreports.AnalysisStatus(status) && report.AIStatus != crashreports.AnalysisStatus(status) {
		return false
	}
	if status := query.Get("stackwalk_status"); status != "" && report.StackwalkStatus != crashreports.AnalysisStatus(status) {
		return false
	}
	if status := query.Get("ai_status"); status != "" && report.AIStatus != crashreports.AnalysisStatus(status) {
		return false
	}
	return true
}

func crashReportReferencesBinary(report crashreports.Report, artifactID string) bool {
	for _, module := range report.Modules {
		if module.BinaryArtifact == artifactID {
			return true
		}
	}
	return false
}

func (s *Server) analyzeCrashReport(w http.ResponseWriter, r *http.Request) {
	if s.crashReports == nil {
		writeError(w, http.StatusServiceUnavailable, "crash_reports_unavailable", "crash report manager unavailable")
		return
	}
	if s.crashAnalysis == nil || s.jobs == nil {
		writeError(w, http.StatusServiceUnavailable, "analysis_unavailable", "crash analysis worker unavailable")
		return
	}
	report, err := s.crashReports.Get(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, crashreports.ErrNotFound) {
		writeError(w, http.StatusNotFound, "crash_report_not_found", "crash report not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "crash_reports_error", "crash report details unavailable")
		return
	}
	var input struct {
		AI *bool `json:"ai"`
	}
	if r.Body != nil && r.Body != http.NoBody {
		if decodeOptionalJSON(w, r, &input) != nil {
			return
		}
	}
	requestAI := true
	if input.AI != nil {
		requestAI = *input.AI
	}
	job, ok := s.startJob(w, r, report.InstanceID, "crash_analysis", func(ctx context.Context, reporter jobs.Reporter) error {
		reporter.Progress("analyzing", 5, "正在分析崩溃转储")
		if err := s.crashAnalysis.Analyze(ctx, report.ID, requestAI); err != nil {
			return err
		}
		reporter.Progress("persisted", 100, "崩溃转储分析结果已写入")
		return nil
	})
	if !ok {
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/crashreports"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"github.com/not0721here/l4d2-control-panel/internal/jobs"
	"github.com/not0721here/l4d2-control-panel/internal/secrets"
)

func testSecretService(t *testing.T, db interface {
	SaveSecret(context.Context, string, []byte) error
	LoadSecret(context.Context, string) ([]byte, bool, error)
	DeleteSecret(context.Context, string) error
}) *secrets.Service {
	t.Helper()
	service, err := secrets.New(db, []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	return service
}

type failingSecretRepository struct{}

func (failingSecretRepository) SaveSecret(context.Context, string, []byte) error {
	return errors.New("secret write failed")
}
func (failingSecretRepository) LoadSecret(context.Context, string) ([]byte, bool, error) {
	return nil, false, nil
}
func (failingSecretRepository) DeleteSecret(context.Context, string) error { return nil }

func TestCrashAnalysisSettingsDoesNotPersistWhenAPIKeyWriteFails(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	service, err := secrets.New(failingSecretRepository{}, []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	s = New(db, s.auth, WithSecrets(service))
	cookie := loginCookie(t, s)
	response := authenticatedJSON(t, s, cookie, http.MethodPut, "/api/settings/crash-analysis", `{"endpoint":"http://127.0.0.1:11434/v1","model":"local","api_key":"secret"}`)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("failed key write=%d %s", response.Code, response.Body.String())
	}
	settings, err := db.CrashAnalysisSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if settings.Endpoint != "" || settings.Model != "" {
		t.Fatalf("settings persisted after key failure: %+v", settings)
	}
}

func TestAcceleratorSettingsAPIValidatesAndPersistsHTTPSSource(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	s = New(db, s.auth, WithSecrets(testSecretService(t, db)))
	cookie := loginCookie(t, s)

	initial := authenticatedJSON(t, s, cookie, http.MethodGet, "/api/settings/accelerator", "")
	if initial.Code != http.StatusOK || initial.Body.String() != `{"download_url":"http://sp2.0721play.icu/d/Discord%E5%85%B1%E4%BA%AB%E6%96%87%E4%BB%B6%E5%A4%B9/accelerator-2.6.0-static-x86-linux.zip","use_github_proxy":false}`+"\n" {
		t.Fatalf("initial settings=%d %q", initial.Code, initial.Body.String())
	}

	for _, raw := range []string{
		`{"download_url":"https://example.test/accelerator.zip?token=secret"}`,
	} {
		response := authenticatedJSON(t, s, cookie, http.MethodPut, "/api/settings/accelerator", raw)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid URL %s accepted: %d %s", raw, response.Code, response.Body.String())
		}
	}

	saved := authenticatedJSON(t, s, cookie, http.MethodPut, "/api/settings/accelerator", `{"download_url":"https://github.com/example/accelerator.zip","use_github_proxy":true}`)
	if saved.Code != http.StatusOK || !strings.Contains(saved.Body.String(), `"use_github_proxy":true`) || !strings.Contains(saved.Body.String(), `https://github.com/example/accelerator.zip`) {
		t.Fatalf("save settings=%d %q", saved.Code, saved.Body.String())
	}
	loaded := authenticatedJSON(t, s, cookie, http.MethodGet, "/api/settings/accelerator", "")
	if loaded.Code != http.StatusOK || !strings.Contains(loaded.Body.String(), `"use_github_proxy":true`) {
		t.Fatalf("loaded settings=%d %q", loaded.Code, loaded.Body.String())
	}
}

func TestCrashAnalysisSettingsAPIStoresOnlyAPIKeyPresence(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	s = New(db, s.auth, WithSecrets(testSecretService(t, db)))
	cookie := loginCookie(t, s)

	initial := authenticatedJSON(t, s, cookie, http.MethodGet, "/api/settings/crash-analysis", "")
	if initial.Code != http.StatusOK || !strings.Contains(initial.Body.String(), `"api_key_set":false`) {
		t.Fatalf("initial analysis settings=%d %q", initial.Code, initial.Body.String())
	}

	invalid := authenticatedJSON(t, s, cookie, http.MethodPut, "/api/settings/crash-analysis", `{"endpoint":"http://example.test/v1","model":"local"}`)
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("remote HTTP endpoint accepted: %d %s", invalid.Code, invalid.Body.String())
	}

	saved := authenticatedJSON(t, s, cookie, http.MethodPut, "/api/settings/crash-analysis", `{"endpoint":"http://127.0.0.1:11434/v1","model":"local","api_key":"dont-return-this"}`)
	if saved.Code != http.StatusOK || !strings.Contains(saved.Body.String(), `"api_key_set":true`) || strings.Contains(saved.Body.String(), "dont-return-this") {
		t.Fatalf("save analysis settings=%d %q", saved.Code, saved.Body.String())
	}
	loaded := authenticatedJSON(t, s, cookie, http.MethodGet, "/api/settings/crash-analysis", "")
	if loaded.Code != http.StatusOK || !strings.Contains(loaded.Body.String(), `"api_key_set":true`) || strings.Contains(loaded.Body.String(), "dont-return-this") {
		t.Fatalf("loaded analysis settings=%d %q", loaded.Code, loaded.Body.String())
	}

	cleared := authenticatedJSON(t, s, cookie, http.MethodPut, "/api/settings/crash-analysis", `{"clear_api_key":true}`)
	if cleared.Code != http.StatusOK || !strings.Contains(cleared.Body.String(), `"api_key_set":false`) {
		t.Fatalf("clear analysis key=%d %q", cleared.Code, cleared.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(cleared.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if _, exists := response["api_key"]; exists {
		t.Fatal("API response exposed api_key field")
	}
}

type acceleratorTrackingLifecycle struct {
	action string
}

type recordingAnalysisQueue struct {
	reportID  string
	requestAI bool
	started   chan struct{}
	release   <-chan struct{}
}

func (q *recordingAnalysisQueue) Enqueue(_ context.Context, reportID string, requestAI bool) error {
	q.reportID = reportID
	q.requestAI = requestAI
	return nil
}

func (q *recordingAnalysisQueue) Analyze(ctx context.Context, reportID string, requestAI bool) error {
	q.reportID = reportID
	q.requestAI = requestAI
	if q.started != nil {
		select {
		case q.started <- struct{}{}:
		default:
		}
	}
	if q.release != nil {
		select {
		case <-q.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (l *acceleratorTrackingLifecycle) Start(context.Context, string) error {
	l.action = "start"
	return nil
}

func (l *acceleratorTrackingLifecycle) Stop(context.Context, string) error {
	l.action = "stop"
	return nil
}

func (l *acceleratorTrackingLifecycle) Restart(context.Context, string) error {
	l.action = "restart"
	return nil
}

func (l *acceleratorTrackingLifecycle) Rebuild(context.Context, string) error {
	l.action = "rebuild"
	return nil
}

func (l *acceleratorTrackingLifecycle) Delete(context.Context, string, bool) error {
	l.action = "delete"
	return nil
}

func TestInstanceAcceleratorFlagRebuildsExistingContainerButDoesNotStartUninstalledInstance(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	packageID := defaultPackageID(t, s)
	lifecycle := &acceleratorTrackingLifecycle{}
	jobManager := jobs.NewManager()
	s = New(db, s.auth,
		WithOperations(lifecycle, jobManager),
		WithContent(nil, nil, s.packages, s.updates, nil),
	)
	cookie := loginCookie(t, s)
	base := domain.Instance{
		ID: "configured", NodeID: "local", Name: "configured", ContainerID: "container",
		GamePort: 27015, StartMap: "c2m1_highway", GameMode: "coop", Tickrate: 100, MaxPlayers: 8,
		SelectedPackageID: packageID, PackageVersion: packageID, DesiredState: domain.StateStopped, ActualState: domain.StateStopped,
	}
	if err := db.CreateInstance(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	response := authenticatedJSON(t, s, cookie, http.MethodPut, "/api/instances/configured", instanceJSON(base, true, false))
	if response.Code != http.StatusAccepted {
		t.Fatalf("existing container update=%d %s", response.Code, response.Body.String())
	}
	var job jobs.Job
	if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	waitForJob(t, jobManager, job.ID)
	if lifecycle.action != "rebuild" {
		t.Fatalf("lifecycle action=%q", lifecycle.action)
	}
	updated, err := db.Instance(context.Background(), base.ID)
	if err != nil || !updated.AcceleratorEnabled {
		t.Fatalf("updated instance=%+v err=%v", updated, err)
	}

	uninstalled := base
	uninstalled.ID = "uninstalled"
	uninstalled.Name = "uninstalled"
	uninstalled.ContainerID = ""
	uninstalled.GamePort = 27016
	uninstalled.ActualState = domain.StateUninstalled
	if err := db.CreateInstance(context.Background(), uninstalled); err != nil {
		t.Fatal(err)
	}
	lifecycle.action = ""
	response = authenticatedJSON(t, s, cookie, http.MethodPut, "/api/instances/uninstalled", instanceJSON(uninstalled, true, true))
	if response.Code != http.StatusOK || lifecycle.action != "" {
		t.Fatalf("uninstalled update=%d action=%q body=%s", response.Code, lifecycle.action, response.Body.String())
	}
	updated, err = db.Instance(context.Background(), uninstalled.ID)
	if err != nil || !updated.AcceleratorEnabled || !updated.AutoCrashAnalysis {
		t.Fatalf("uninstalled instance=%+v err=%v", updated, err)
	}
}

func TestCrashReportManagementAPIFiltersDetailsAndDownloadsArtifacts(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	root := t.TempDir()
	manager, err := crashreports.New(crashreports.Config{Root: root, Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := manager.SaveBinary(context.Background(), crashreports.BinaryInput{
		InstanceID: "instance-a", Platform: "windows", Architecture: "x86", DebugIdentifier: "DEBUG-ID",
		CodeFileName: `C:\game\server.dll`, CodeFile: bytes.NewReader([]byte("binary-bytes")),
	})
	if err != nil {
		t.Fatal(err)
	}
	signature := "2|0|Windows|x86|1|EXCEPTION|0|0|M|server.dll|DEBUG-ID"
	pre, err := manager.PreSubmit(crashreports.PreSubmitInput{InstanceID: "instance-a", ServerID: "server-a", CrashSignature: signature})
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(pre, "|")
	if len(parts) != 3 {
		t.Fatalf("pre-submit=%q", pre)
	}
	report, err := manager.Receive(context.Background(), crashreports.UploadInput{
		InstanceID: "instance-a", ServerID: "server-a", PresubmitToken: parts[2],
		Minidump: bytes.NewReader([]byte("MDMPreport-a")), Metadata: strings.NewReader("metadata for report-a"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Modules) != 1 || report.Modules[0].BinaryArtifact != artifact.ID {
		t.Fatalf("report artifact reference=%#v artifact=%#v", report.Modules, artifact)
	}
	if err := manager.SaveStackwalk(context.Background(), report.ID, crashreports.StackwalkUpdate{Status: crashreports.AnalysisStatusSucceeded, Text: "#0 server", Tool: "stackwalk"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.SaveAIAnalysis(context.Background(), report.ID, crashreports.AIAnalysisUpdate{Status: crashreports.AnalysisStatusSucceeded, Model: "local", Text: "analysis"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Receive(context.Background(), crashreports.UploadInput{
		InstanceID: "instance-b", Minidump: bytes.NewReader([]byte("MDMPreport-b")), Metadata: strings.NewReader("metadata for report-b"),
	}); err != nil {
		t.Fatal(err)
	}
	queue := &recordingAnalysisQueue{}
	jobManager := jobs.NewManager()
	s = New(db, s.auth, WithCrashReports(manager), WithCrashAnalysis(queue), WithOperations(nil, jobManager))
	cookie := loginCookie(t, s)

	list := authenticatedJSON(t, s, cookie, http.MethodGet, "/api/crash-reports?instance_id=instance-a&signature=EXCEPTION&status=succeeded", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), report.ID) || strings.Count(list.Body.String(), `"id"`) != 1 {
		t.Fatalf("filtered reports=%d %s", list.Code, list.Body.String())
	}
	detail := authenticatedJSON(t, s, cookie, http.MethodGet, "/api/crash-reports/"+report.ID, "")
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "metadata for report-a") || !strings.Contains(detail.Body.String(), "DEBUG-ID") || strings.Contains(detail.Body.String(), root) {
		t.Fatalf("detail=%d %s", detail.Code, detail.Body.String())
	}
	stackwalk := authenticatedJSON(t, s, cookie, http.MethodGet, "/api/crash-reports/"+report.ID+"/download?file=stackwalk", "")
	if stackwalk.Code != http.StatusOK || stackwalk.Body.String() != "#0 server" {
		t.Fatalf("stackwalk=%d %q", stackwalk.Code, stackwalk.Body.String())
	}
	ai := authenticatedJSON(t, s, cookie, http.MethodGet, "/api/crash-reports/"+report.ID+"/download?file=ai", "")
	if ai.Code != http.StatusOK || ai.Body.String() != "analysis" {
		t.Fatalf("AI=%d %q", ai.Code, ai.Body.String())
	}
	missingArtifact := authenticatedJSON(t, s, cookie, http.MethodGet, "/api/crash-reports/"+report.ID+"/download?file=binary", "")
	if missingArtifact.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing artifact=%d %s", missingArtifact.Code, missingArtifact.Body.String())
	}
	binary := authenticatedJSON(t, s, cookie, http.MethodGet, "/api/crash-reports/"+report.ID+"/download?file=binary&artifact="+artifact.ID, "")
	if binary.Code != http.StatusOK || !bytes.Equal(binary.Body.Bytes(), []byte("binary-bytes")) {
		t.Fatalf("binary=%d %q", binary.Code, binary.Body.Bytes())
	}
	forbidden := authenticatedJSON(t, s, cookie, http.MethodGet, "/api/crash-reports/"+report.ID+"/download?file=binary&artifact="+strings.Repeat("0", 64), "")
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("unrelated artifact=%d %s", forbidden.Code, forbidden.Body.String())
	}
	analyze := authenticatedJSON(t, s, cookie, http.MethodPost, "/api/crash-reports/"+report.ID+"/analyze", `{"ai":true}`)
	if analyze.Code != http.StatusAccepted {
		t.Fatalf("analyze=%d %s", analyze.Code, analyze.Body.String())
	}
	var job jobs.Job
	if err := json.Unmarshal(analyze.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	waitForJob(t, jobManager, job.ID)
	if queue.reportID != report.ID || !queue.requestAI {
		t.Fatalf("analysis queue=%+v", queue)
	}
}

func TestCrashReportAnalyzeDefaultsToStackwalkOnly(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	manager, err := crashreports.New(crashreports.Config{Root: t.TempDir(), Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	report, err := manager.Receive(context.Background(), crashreports.UploadInput{
		Minidump: bytes.NewReader([]byte("MDMPempty-body")),
		Metadata: strings.NewReader("metadata"),
	})
	if err != nil {
		t.Fatal(err)
	}
	queue := &recordingAnalysisQueue{}
	jobManager := jobs.NewManager()
	s = New(db, s.auth, WithCrashReports(manager), WithCrashAnalysis(queue), WithOperations(nil, jobManager))
	cookie := loginCookie(t, s)
	response := authenticatedJSON(t, s, cookie, http.MethodPost, "/api/crash-reports/"+report.ID+"/analyze", "")
	if response.Code != http.StatusAccepted {
		t.Fatalf("empty-body analyze=%d %s", response.Code, response.Body.String())
	}
	var job jobs.Job
	if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	waitForJob(t, jobManager, job.ID)
	if queue.reportID != report.ID || queue.requestAI {
		t.Fatalf("analysis queue=%+v", queue)
	}
}

func TestCrashReportAnalyzeTaskWaitsForAnalysisPersistence(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	manager, err := crashreports.New(crashreports.Config{Root: t.TempDir(), Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	report, err := manager.Receive(context.Background(), crashreports.UploadInput{
		Minidump: bytes.NewReader([]byte("MDMPwait-for-ai")), Metadata: strings.NewReader("metadata"),
	})
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	queue := &recordingAnalysisQueue{started: make(chan struct{}, 1), release: release}
	jobManager := jobs.NewManager()
	s = New(db, s.auth, WithCrashReports(manager), WithCrashAnalysis(queue), WithOperations(nil, jobManager))
	cookie := loginCookie(t, s)
	response := authenticatedJSON(t, s, cookie, http.MethodPost, "/api/crash-reports/"+report.ID+"/analyze", `{"ai":true}`)
	if response.Code != http.StatusAccepted {
		t.Fatalf("analyze=%d %s", response.Code, response.Body.String())
	}
	var job jobs.Job
	if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	select {
	case <-queue.started:
	case <-time.After(time.Second):
		t.Fatal("analysis task did not wait for persistence")
	}
	current, found := jobManager.Get(job.ID)
	if !found || current.Status != jobs.Running {
		t.Fatalf("job ended before analysis persisted: %+v found=%t", current, found)
	}
	close(release)
	waitForJob(t, jobManager, job.ID)
	if err := jobManager.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if queue.reportID != report.ID || !queue.requestAI {
		t.Fatalf("analysis runner=%+v", queue)
	}
}

func instanceJSON(instance domain.Instance, acceleratorEnabled, autoCrashAnalysis bool) string {
	raw, err := json.Marshal(map[string]any{
		"name":                instance.Name,
		"game_port":           instance.GamePort,
		"sourcetv_port":       instance.SourceTVPort,
		"plugin_ports":        instance.PluginPorts,
		"start_map":           instance.StartMap,
		"game_mode":           instance.GameMode,
		"tickrate":            instance.Tickrate,
		"max_players":         instance.MaxPlayers,
		"extra_args":          instance.ExtraArgs,
		"package_id":          instance.SelectedPackageID,
		"source_id":           instance.PackageSourceID,
		"accelerator_enabled": acceleratorEnabled,
		"auto_crash_analysis": autoCrashAnalysis,
	})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

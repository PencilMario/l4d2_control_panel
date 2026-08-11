package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/not0721here/l4d2-control-panel/internal/crashreports"
)

func TestCrashReportPublicSubmitRequiresLoopbackAndTokenWithoutPanelSession(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	manager, err := crashreports.New(crashreports.Config{
		Root:  t.TempDir(),
		Token: "secret",
		AuthorizeInstance: func(_ context.Context, serverID, gameDirectory string) error {
			if serverID != "server-id" || (gameDirectory != "" && gameDirectory != "left4dead2") {
				return crashreports.ErrInstanceNotAllowed
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	s = New(db, s.auth, WithCrashReports(manager))

	form := url.Values{
		"UserID":         {"account"},
		"ServerID":       {"server-id"},
		"CrashSignature": {"1|0|Linux|x86_64|0|SIGSEGV"},
	}
	request := httptest.NewRequest(http.MethodPost, "/submit?token=secret", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.RemoteAddr = "127.0.0.1:1234"
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.HasPrefix(response.Body.String(), "Y||") {
		t.Fatalf("public submit=%d %q", response.Code, response.Body.String())
	}

	wrongToken := httptest.NewRequest(http.MethodPost, "/submit?token=wrong", strings.NewReader(form.Encode()))
	wrongToken.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	wrongToken.RemoteAddr = "127.0.0.1:1234"
	wrongResponse := httptest.NewRecorder()
	s.Handler().ServeHTTP(wrongResponse, wrongToken)
	if wrongResponse.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token=%d %q", wrongResponse.Code, wrongResponse.Body.String())
	}

	nonLocal := httptest.NewRequest(http.MethodPost, "/submit?token=secret", strings.NewReader(form.Encode()))
	nonLocal.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	nonLocal.RemoteAddr = "192.168.1.10:1234"
	nonLocalResponse := httptest.NewRecorder()
	s.Handler().ServeHTTP(nonLocalResponse, nonLocal)
	if nonLocalResponse.Code != http.StatusForbidden || nonLocalResponse.Body.String() != "local_source_required" {
		t.Fatalf("non-local=%d %q", nonLocalResponse.Code, nonLocalResponse.Body.String())
	}
}

func TestCrashReportManagementAPINeedsSessionAndDoesNotExposeStorageRoot(t *testing.T) {
	s, db := testServer(t)
	defer db.Close()
	root := t.TempDir()
	manager, err := crashreports.New(crashreports.Config{Root: root, Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	report, err := manager.Receive(context.Background(), crashreports.UploadInput{
		UserID:        "account",
		GameDirectory: "left4dead2",
		Minidump:      bytes.NewReader([]byte("MDMPpanel report")),
		Metadata:      strings.NewReader("metadata"),
	})
	if err != nil {
		t.Fatal(err)
	}
	s = New(db, s.auth, WithCrashReports(manager))

	unauthorized := httptest.NewRecorder()
	s.Handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/crash-reports", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized list=%d %q", unauthorized.Code, unauthorized.Body.String())
	}

	cookie := loginCookie(t, s)
	list := authenticatedJSON(t, s, cookie, http.MethodGet, "/api/crash-reports", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), report.ID) || strings.Contains(list.Body.String(), root) {
		t.Fatalf("list=%d %q", list.Code, list.Body.String())
	}
	detail := authenticatedJSON(t, s, cookie, http.MethodGet, "/api/crash-reports/"+report.ID, "")
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"sha256":"`+report.ID+`"`) || strings.Contains(detail.Body.String(), root) {
		t.Fatalf("detail=%d %q", detail.Code, detail.Body.String())
	}
	dump := authenticatedJSON(t, s, cookie, http.MethodGet, "/api/crash-reports/"+report.ID+"/download?file=minidump", "")
	if dump.Code != http.StatusOK || !bytes.Equal(dump.Body.Bytes(), []byte("MDMPpanel report")) || !strings.Contains(dump.Header().Get("Content-Disposition"), report.ID) {
		t.Fatalf("dump=%d headers=%v body=%q", dump.Code, dump.Header(), dump.Body.Bytes())
	}
	metadata := authenticatedJSON(t, s, cookie, http.MethodGet, "/api/crash-reports/"+report.ID+"/download?file=metadata", "")
	if metadata.Code != http.StatusOK || metadata.Body.String() != "metadata" {
		t.Fatalf("metadata=%d %q", metadata.Code, metadata.Body.String())
	}
	invalid := authenticatedJSON(t, s, cookie, http.MethodGet, "/api/crash-reports/"+report.ID+"/download?file=secret", "")
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid file=%d %q", invalid.Code, invalid.Body.String())
	}
	missing := authenticatedJSON(t, s, cookie, http.MethodGet, "/api/crash-reports/"+strings.Repeat("0", 64), "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing detail=%d %q", missing.Code, missing.Body.String())
	}
	if err := os.RemoveAll(filepath.Join(root, "reports")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "reports"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	storageFailure := authenticatedJSON(t, s, cookie, http.MethodGet, "/api/crash-reports", "")
	escapedRoot := strings.ReplaceAll(root, `\`, `\\`)
	if storageFailure.Code != http.StatusInternalServerError || strings.Contains(storageFailure.Body.String(), root) || strings.Contains(storageFailure.Body.String(), escapedRoot) {
		t.Fatalf("storage failure=%d %q", storageFailure.Code, storageFailure.Body.String())
	}
}

package crashreports

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPreSubmitReturnsOneDecisionPerModule(t *testing.T) {
	manager := newTestManager(t, testProtocolNow())
	signature := "2|0|Linux|x86_64|3|SIGSEGV|abc|0|M|server-a|M|server-b|M|server-c"
	request := newFormRequest(t, "/submit?token=secret", map[string]string{
		"UserID":           "account",
		"ExtensionVersion": "1.2.3",
		"ServerID":         "server-id",
		"CrashSignature":   signature,
	})
	response := httptest.NewRecorder()
	manager.SubmitHandler(response, request)

	parts := strings.Split(response.Body.String(), "|")
	if response.Code != http.StatusOK || len(parts) != 3 || parts[0] != "Y" || parts[1] != "NNN" || parts[2] == "" {
		t.Fatalf("response=%d %q", response.Code, response.Body.String())
	}
	pending, err := manager.readPending(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	if pending.Input.CrashSignature != signature || pending.Input.UserID != "account" {
		t.Fatalf("pending=%#v", pending)
	}
}

func TestMultipartUploadRejectsPresubmitTokenFromAnotherInstance(t *testing.T) {
	manager, err := New(Config{
		Root:  t.TempDir(),
		Token: "secret",
		ResolveInstance: func(_ context.Context, serverID, _ string) (string, error) {
			switch serverID {
			case "server-a":
				return "instance-a", nil
			case "server-b":
				return "instance-b", nil
			default:
				return "", ErrInstanceNotAllowed
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	pre := newFormRequest(t, "/submit?token=secret", map[string]string{
		"ServerID":       "server-a",
		"CrashSignature": "1|0|Linux|x86_64|0|SIGSEGV",
	})
	preResponse := httptest.NewRecorder()
	manager.SubmitHandler(preResponse, pre)
	if preResponse.Code != http.StatusOK {
		t.Fatalf("pre-submit=%d %q", preResponse.Code, preResponse.Body.String())
	}
	parts := strings.Split(preResponse.Body.String(), "|")
	if len(parts) != 3 || parts[2] == "" {
		t.Fatalf("pre-submit=%q", preResponse.Body.String())
	}

	upload := newMultipartRequest(t, "/submit?token=secret", map[string]string{
		"ServerID":       "server-b",
		"GameDirectory":  "left4dead2",
		"PresubmitToken": parts[2],
	}, map[string][]byte{
		"upload_file_minidump": []byte("MDMPcross-instance"),
		"upload_file_metadata": []byte("metadata"),
	})
	uploadResponse := httptest.NewRecorder()
	manager.SubmitHandler(uploadResponse, upload)
	if uploadResponse.Code != http.StatusForbidden || uploadResponse.Body.String() != "instance_not_allowed" {
		t.Fatalf("cross-instance upload=%d %q", uploadResponse.Code, uploadResponse.Body.String())
	}
	if reports, listErr := manager.List(context.Background()); listErr != nil || len(reports) != 0 {
		t.Fatalf("reports=%#v err=%v", reports, listErr)
	}
}

func TestArtifactUploadsRejectPresubmitTokenFromAnotherInstance(t *testing.T) {
	manager, err := New(Config{
		Root:  t.TempDir(),
		Token: "secret",
		ResolveInstance: func(_ context.Context, serverID, _ string) (string, error) {
			switch serverID {
			case "server-a":
				return "instance-a", nil
			case "server-b":
				return "instance-b", nil
			default:
				return "", ErrInstanceNotAllowed
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	pre := newFormRequest(t, "/submit?token=secret", map[string]string{
		"ServerID":       "server-a",
		"CrashSignature": "2|0|Linux|x86_64|1|SIGSEGV|0|0|M|server.so|DEBUG",
	})
	preResponse := httptest.NewRecorder()
	manager.SubmitHandler(preResponse, pre)
	if preResponse.Code != http.StatusOK {
		t.Fatalf("pre-submit=%d %q", preResponse.Code, preResponse.Body.String())
	}
	parts := strings.Split(preResponse.Body.String(), "|")
	if len(parts) != 3 || parts[2] == "" {
		t.Fatalf("pre-submit=%q", preResponse.Body.String())
	}
	for _, test := range []struct {
		name     string
		endpoint string
		fields   map[string]string
		files    map[string][]byte
	}{
		{
			name:     "symbol",
			endpoint: "/symbols/submit?token=secret",
			fields: map[string]string{
				"ServerID": "server-b", "PresubmitToken": parts[2], "debug_identifier": "DEBUG",
			},
			files: map[string][]byte{"symbol_file": []byte("MODULE Linux x86_64 DEBUG server.so\n")},
		},
		{
			name:     "binary",
			endpoint: "/binary/submit?token=secret",
			fields: map[string]string{
				"ServerID": "server-b", "PresubmitToken": parts[2], "debug_identifier": "DEBUG",
			},
			files: map[string][]byte{"code_file": []byte("ELF bytes")},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := newMultipartRequest(t, test.endpoint, test.fields, test.files)
			response := httptest.NewRecorder()
			switch test.name {
			case "symbol":
				manager.SymbolHandler(response, request)
			case "binary":
				manager.BinaryHandler(response, request)
			}
			if response.Code != http.StatusForbidden || response.Body.String() != "instance_not_allowed" {
				t.Fatalf("cross-instance %s=%d %q", test.name, response.Code, response.Body.String())
			}
		})
	}
}

func TestProtocolRejectsNonLocalSourceBeforeReadingBody(t *testing.T) {
	manager := newTestManager(t, testProtocolNow())
	request := httptest.NewRequest(http.MethodPost, "/submit?token=secret", panicReader{})
	request.RemoteAddr = "203.0.113.10:1234"
	response := httptest.NewRecorder()
	manager.SubmitHandler(response, request)
	if response.Code != http.StatusForbidden || response.Body.String() != "local_source_required" {
		t.Fatalf("response=%d %q", response.Code, response.Body.String())
	}
}

func TestProtocolRejectsUnknownProjectInstance(t *testing.T) {
	manager, err := New(Config{
		Root:  t.TempDir(),
		Token: "secret",
		AuthorizeInstance: func(_ context.Context, serverID, _ string) error {
			if serverID != "known-server" {
				return ErrInstanceNotAllowed
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := newFormRequest(t, "/submit?token=secret", map[string]string{
		"ServerID":       "unknown-server",
		"CrashSignature": "1|0|Linux|x86_64|0|SIGSEGV",
	})
	response := httptest.NewRecorder()
	manager.SubmitHandler(response, request)
	if response.Code != http.StatusForbidden || response.Body.String() != "instance_not_allowed" {
		t.Fatalf("response=%d %q", response.Code, response.Body.String())
	}
	if entries, readErr := os.ReadDir(filepath.Join(manager.root, "pending")); readErr != nil || len(entries) != 0 {
		t.Fatalf("pending=%v err=%v", entries, readErr)
	}
}

func TestProtocolRequiresProjectInstanceAuthorizer(t *testing.T) {
	manager, err := New(Config{Root: t.TempDir(), Token: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	request := newFormRequest(t, "/submit?token=secret", map[string]string{
		"ServerID":       "server-id",
		"CrashSignature": "1|0|Linux|x86_64|0|SIGSEGV",
	})
	response := httptest.NewRecorder()
	manager.SubmitHandler(response, request)
	if response.Code != http.StatusForbidden || response.Body.String() != "instance_not_allowed" {
		t.Fatalf("response=%d %q", response.Code, response.Body.String())
	}
}

func TestPreSubmitAllowsZeroModulesAndRejectsUnauthorizedInput(t *testing.T) {
	for _, test := range []struct {
		name       string
		token      string
		signature  string
		configured bool
		status     int
		response   string
	}{
		{name: "zero modules", token: "secret", signature: "2|0|Linux|x86_64|0|SIGSEGV", configured: true, status: http.StatusOK, response: "Y||"},
		{name: "missing token", token: "", signature: "", configured: true, status: http.StatusUnauthorized, response: "unauthorized"},
		{name: "wrong token", token: "wrong", signature: "", configured: true, status: http.StatusUnauthorized, response: "unauthorized"},
		{name: "unconfigured", token: "secret", signature: "", configured: false, status: http.StatusServiceUnavailable, response: "crash report receiver disabled"},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := Config{Root: t.TempDir()}
			config.AuthorizeInstance = allowTestInstance
			if test.configured {
				config.Token = "secret"
			}
			manager, err := New(config)
			if err != nil {
				t.Fatal(err)
			}
			request := newFormRequest(t, "/submit?token="+test.token, map[string]string{"CrashSignature": test.signature})
			response := httptest.NewRecorder()
			manager.SubmitHandler(response, request)
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.response) {
				t.Fatalf("response=%d %q", response.Code, response.Body.String())
			}
			if !test.configured || test.token != "secret" {
				entries, readErr := os.ReadDir(filepath.Join(manager.root, "pending"))
				if readErr != nil || len(entries) != 0 {
					t.Fatalf("pending=%v err=%v", entries, readErr)
				}
			}
		})
	}
}

func TestPreSubmitRejectsInvalidAndOversizedCrashSignature(t *testing.T) {
	for _, signature := range []string{"bad\x00signature", strings.Repeat("x", MaxCrashSignatureBytes+1)} {
		t.Run(fmt.Sprintf("length-%d", len(signature)), func(t *testing.T) {
			manager := newTestManager(t, testProtocolNow())
			request := newFormRequest(t, "/submit?token=secret", map[string]string{"CrashSignature": signature})
			response := httptest.NewRecorder()
			manager.SubmitHandler(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
			}
		})
	}
}

func TestMultipartUploadStoresReportAndCorrelatesPreSubmit(t *testing.T) {
	manager := newTestManager(t, testProtocolNow())
	preSubmit := newFormRequest(t, "/submit?token=secret", map[string]string{"CrashSignature": "1|0|Linux|x86_64|1|SIGSEGV|M|server"})
	preResponse := httptest.NewRecorder()
	manager.SubmitHandler(preResponse, preSubmit)
	parts := strings.Split(preResponse.Body.String(), "|")
	if preResponse.Code != http.StatusOK || len(parts) != 3 {
		t.Fatalf("pre-submit=%d %q", preResponse.Code, preResponse.Body.String())
	}

	request := newMultipartRequest(t, "/submit?token=secret", map[string]string{
		"UserID":           "account",
		"GameDirectory":    "left4dead2",
		"ExtensionVersion": "1.2.3",
		"ServerID":         "server-id",
		"PresubmitToken":   parts[2],
	}, map[string][]byte{
		"upload_file_minidump": []byte("MDMPreal dump"),
		"upload_file_metadata": []byte("GameDirectory=left4dead2\n"),
	})
	response := httptest.NewRecorder()
	manager.SubmitHandler(response, request)
	if response.Code != http.StatusOK || !strings.HasPrefix(response.Body.String(), "OK|") {
		t.Fatalf("upload=%d %q", response.Code, response.Body.String())
	}
	id := strings.TrimPrefix(response.Body.String(), "OK|")
	report, err := manager.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if report.CrashSignature == "" || report.UserID != "account" || report.GameDirectory != "left4dead2" {
		t.Fatalf("report=%#v", report)
	}
	if _, err := os.Stat(filepath.Join(manager.root, "pending", parts[2]+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pending token remains: %v", err)
	}
}

func TestMultipartUploadRejectsInvalidInputAndLeavesNoReport(t *testing.T) {
	for _, test := range []struct {
		name  string
		files map[string][]byte
	}{
		{name: "missing dump", files: map[string][]byte{"upload_file_metadata": []byte("metadata")}},
		{name: "missing metadata", files: map[string][]byte{"upload_file_minidump": []byte("MDMPdump")}},
		{name: "invalid dump", files: map[string][]byte{"upload_file_minidump": []byte("NOPE"), "upload_file_metadata": []byte("metadata")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager := newTestManager(t, testProtocolNow())
			request := newMultipartRequest(t, "/submit?token=secret", nil, test.files)
			response := httptest.NewRecorder()
			manager.SubmitHandler(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
			}
			reports, err := manager.List(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(reports) != 0 {
				t.Fatalf("reports=%#v", reports)
			}
			entries, err := os.ReadDir(filepath.Join(manager.root, "incoming"))
			if err != nil || len(entries) != 0 {
				t.Fatalf("incoming=%v err=%v", entries, err)
			}
		})
	}
}

func TestMultipartUploadRejectsOversizedMinidump(t *testing.T) {
	manager := newTestManager(t, testProtocolNow())
	boundary := "crash-test-boundary"
	prefix := []byte("--" + boundary + "\r\nContent-Disposition: form-data; name=\"upload_file_minidump\"; filename=\"crash.dmp\"\r\nContent-Type: application/octet-stream\r\n\r\nMDMP")
	metadata := []byte("\r\n--" + boundary + "\r\nContent-Disposition: form-data; name=\"upload_file_metadata\"; filename=\"metadata.txt\"\r\nContent-Type: text/plain\r\n\r\nmetadata")
	suffix := []byte("\r\n--" + boundary + "--\r\n")
	request := httptest.NewRequest(http.MethodPost, "/submit?token=secret", io.MultiReader(bytes.NewReader(prefix), &repeatingReader{remaining: MaxMinidumpBytes - 3}, bytes.NewReader(metadata), bytes.NewReader(suffix)))
	request.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	request.RemoteAddr = "127.0.0.1:1234"
	response := httptest.NewRecorder()
	manager.SubmitHandler(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if reports, err := manager.List(context.Background()); err != nil || len(reports) != 0 {
		t.Fatalf("reports=%#v err=%v", reports, err)
	}
}

func TestSymbolUploadStoresGeneratedTextSymbol(t *testing.T) {
	manager := newTestManager(t, testProtocolNow())
	request := newMultipartRequest(t, "/symbols/submit?token=secret", map[string]string{
		"UserID":           "account",
		"ExtensionVersion": "1.2.3",
		"ServerID":         "server-id",
		"PresubmitToken":   "pending",
		"debug_identifier": "ABC/../../escape",
		"code_identifier":  "code",
	}, map[string][]byte{"symbol_file": []byte("MODULE Linux x86_64 ABC\nFUNC 1 2 symbol\n")})
	response := httptest.NewRecorder()
	manager.SymbolHandler(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "OK" {
		t.Fatalf("response=%d %q", response.Code, response.Body.String())
	}
	entries, err := os.ReadDir(filepath.Join(manager.root, "symbols", "uploaded"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("symbols=%v err=%v", entries, err)
	}
	if strings.Contains(entries[0].Name(), "escape") || strings.Contains(entries[0].Name(), "..") {
		t.Fatalf("caller data became path: %s", entries[0].Name())
	}
}

func TestSymbolUploadAcceptsUpstreamStringField(t *testing.T) {
	manager := newTestManager(t, testProtocolNow())
	want := "MODULE Linux x86_64 ABC\nFUNC 1 2 symbol\n"
	request := newMultipartRequest(t, "/symbols/submit?token=secret", map[string]string{
		"ServerID":         "server-id",
		"debug_identifier": "ABC",
		"code_identifier":  "code",
		"symbol_file":      want,
	}, nil)
	response := httptest.NewRecorder()
	manager.SymbolHandler(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "OK" {
		t.Fatalf("response=%d %q", response.Code, response.Body.String())
	}
	entries, err := os.ReadDir(filepath.Join(manager.root, "symbols", "uploaded"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("symbols=%v err=%v", entries, err)
	}
	assertFileBytes(t, filepath.Join(manager.root, "symbols", "uploaded", entries[0].Name()), []byte(want))
}

func TestBinaryUploadStoresUpstreamCodeFile(t *testing.T) {
	manager, err := New(Config{
		Root:  t.TempDir(),
		Token: "secret",
		Now:   func() time.Time { return testProtocolNow() },
		ResolveInstance: func(context.Context, string, string) (string, error) {
			return "instance-1", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := newMultipartRequest(t, "/binary/submit?token=secret", map[string]string{
		"ServerID":         "server-id",
		"debug_identifier": "SERVERDEBUG",
		"code_identifier":  "SERVERCODE",
	}, map[string][]byte{"code_file": []byte("ELF module bytes")})
	response := httptest.NewRecorder()
	manager.BinaryHandler(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "OK" {
		t.Fatalf("response=%d %q", response.Code, response.Body.String())
	}
	entries, err := os.ReadDir(filepath.Join(manager.root, "binaries"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("binaries=%v err=%v", entries, err)
	}
	digest := sha256.Sum256([]byte("ELF module bytes"))
	file, artifact, err := manager.OpenArtifact(context.Background(), ArtifactKindBinary, hex.EncodeToString(digest[:]))
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if artifact.InstanceID != "instance-1" {
		t.Fatalf("artifact=%#v", artifact)
	}
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("request body was read") }

func newFormRequest(t *testing.T, target string, fields map[string]string) *http.Request {
	t.Helper()
	values := make(url.Values, len(fields))
	for key, value := range fields {
		values.Set(key, value)
	}
	form := values.Encode()
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.RemoteAddr = "127.0.0.1:1234"
	return request
}

func newMultipartRequest(t *testing.T, target string, fields map[string]string, files map[string][]byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	for field, content := range files {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, field, field))
		header.Set("Content-Type", "application/octet-stream")
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, target, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.RemoteAddr = "127.0.0.1:1234"
	return request
}

func testProtocolNow() time.Time {
	return time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
}

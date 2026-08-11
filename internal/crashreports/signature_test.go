package crashreports

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseCrashSignatureV2ReadsModulesAndFrames(t *testing.T) {
	value, err := ParseCrashSignature(strings.Join([]string{
		"2", "1710000000", "Linux", "x86_64", "1", "SIGSEGV", "deadbeef", "3",
		"M", "server.so", "SERVERDEBUG",
		"M", "addons/sourcemod/extensions/accelerator.ext.so", "ACCELDEBUG",
		"F", "0", "40", "F", "1", "80",
	}, "|"))
	if err != nil {
		t.Fatal(err)
	}
	if value.Version != 2 || value.Platform != "linux" || value.Architecture != "x86_64" || value.RequestingThread != 3 {
		t.Fatalf("header=%#v", value)
	}
	if len(value.Modules) != 2 || value.Modules[0].DebugFile != "server.so" || value.Modules[0].DebugIdentifier != "SERVERDEBUG" || value.Modules[1].DebugIdentifier != "ACCELDEBUG" {
		t.Fatalf("modules=%#v", value.Modules)
	}
	if len(value.Frames) != 2 || value.Frames[1].ModuleIndex != 1 || value.Frames[1].Offset != "80" {
		t.Fatalf("frames=%#v", value.Frames)
	}
}

func TestParseCrashSignatureNormalizesWindowsAndAcceptsLegacyModuleNames(t *testing.T) {
	value, err := ParseCrashSignature("2|0|Windows|x86|0|EXCEPTION|0|0|M|kernel32.dll|M|user32.dll")
	if err != nil {
		t.Fatal(err)
	}
	if value.Platform != "windows" || len(value.Modules) != 2 || value.Modules[0].DebugIdentifier != "" {
		t.Fatalf("signature=%#v", value)
	}
}

func TestParseCrashSignatureRejectsMalformedAndOversizedValues(t *testing.T) {
	for _, input := range []string{
		"1|0|Linux|x86_64|0|SIGSEGV",
		"2|0|Linux|x86_64|0|SIGSEGV|0|0|X|bad",
		"2|0|Linux|x86_64|0|SIGSEGV|0|0|" + strings.Repeat("M|module|debug|", MaxModuleCount+1),
	} {
		if _, err := ParseCrashSignature(input); err == nil {
			t.Fatalf("accepted malformed signature %q", input[:min(len(input), 80)])
		}
	}
}

func TestPreSubmitUsesPlatformSpecificArtifactDecisions(t *testing.T) {
	manager := newTestManager(t, testProtocolNow())
	linux := "2|0|Linux|x86_64|1|SIGSEGV|0|0|M|server.so|SERVERDEBUG"
	response, err := manager.PreSubmit(PreSubmitInput{CrashSignature: linux})
	if err != nil || !strings.HasPrefix(response, "Y|Y|") {
		t.Fatalf("linux response=%q err=%v", response, err)
	}
	windows := "2|0|Windows|x86|1|EXCEPTION|0|0|M|server.dll|SERVERDEBUG"
	response, err = manager.PreSubmit(PreSubmitInput{CrashSignature: windows})
	if err != nil || !strings.HasPrefix(response, "Y|U|") {
		t.Fatalf("windows response=%q err=%v", response, err)
	}
	if err := manager.SaveSymbol(context.Background(), SymbolInput{DebugIdentifier: "SERVERDEBUG", CodeIdentifier: "", Symbol: strings.NewReader("MODULE Linux x86_64 SERVERDEBUG\n")}); err != nil {
		t.Fatal(err)
	}
	response, err = manager.PreSubmit(PreSubmitInput{CrashSignature: linux})
	if err != nil || !strings.HasPrefix(response, "Y|N|") {
		t.Fatalf("symbol hit response=%q err=%v", response, err)
	}
}

func TestReceivePersistsResolvedInstanceAndParsedModules(t *testing.T) {
	manager, err := New(Config{
		Root:              t.TempDir(),
		Token:             "secret",
		AuthorizeInstance: allowTestInstance,
		ResolveInstance: func(context.Context, string, string) (string, error) {
			return "instance-1", nil
		},
		Now: func() time.Time { return testProtocolNow() },
	})
	if err != nil {
		t.Fatal(err)
	}
	signature := "2|0|Linux|x86_64|1|SIGSEGV|0|0|M|server.so|SERVERDEBUG"
	preSubmit := newFormRequest(t, "/submit?token=secret", map[string]string{
		"ServerID":       "server-id",
		"CrashSignature": signature,
	})
	preResponse := httptest.NewRecorder()
	manager.SubmitHandler(preResponse, preSubmit)
	parts := strings.Split(preResponse.Body.String(), "|")
	if preResponse.Code != http.StatusOK || len(parts) != 3 {
		t.Fatalf("pre-submit=%d %q", preResponse.Code, preResponse.Body.String())
	}
	upload := newMultipartRequest(t, "/submit?token=secret", map[string]string{
		"ServerID":       "server-id",
		"PresubmitToken": parts[2],
	}, map[string][]byte{
		"upload_file_minidump": []byte("MDMPmodule"),
		"upload_file_metadata": []byte("metadata"),
	})
	uploadResponse := httptest.NewRecorder()
	manager.SubmitHandler(uploadResponse, upload)
	if uploadResponse.Code != http.StatusOK {
		t.Fatalf("upload=%d %q", uploadResponse.Code, uploadResponse.Body.String())
	}
	report, err := manager.Get(context.Background(), strings.TrimPrefix(uploadResponse.Body.String(), "OK|"))
	if err != nil {
		t.Fatal(err)
	}
	if report.InstanceID != "instance-1" || len(report.Modules) != 1 || report.Modules[0].DebugIdentifier != "SERVERDEBUG" {
		t.Fatalf("report=%#v", report)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

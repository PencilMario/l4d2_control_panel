package crashanalysis

import (
	"strings"
	"testing"

	"github.com/not0721here/l4d2-control-panel/internal/crashreports"
)

func TestRedactRemovesSecretsPathsIPsAndCommandLine(t *testing.T) {
	metadata := `GameDirectory=left4dead2
ServerID=550e8400-e29b-41d4-a716-446655440000
UserID=STEAM_1:0:123456
Token=https://127.0.0.1:8080/submit?token=secret-token
CommandLine=srcds_run -game /opt/l4d2 -ip 192.0.2.44 +map c1m1
LogPath=C:\\servers\\l4d2\\console.log
`
	stackwalk := `#0 0x1234 in /opt/l4d2/addons/sourcemod/extensions/accelerator.ext.so
remote 2001:db8::1:27015 token=secret-token
`
	redacted := Redact(metadata + stackwalk)
	for _, secret := range []string{
		"550e8400-e29b-41d4-a716-446655440000",
		"STEAM_1:0:123456",
		"secret-token",
		"/opt/l4d2",
		`C:\\servers\\l4d2\\console.log`,
		"192.0.2.44",
		"2001:db8::1",
	} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("redaction left %q in %q", secret, redacted)
		}
	}
	for _, marker := range []string{"<path>", "<ip>", "<id>", "<redacted-command>"} {
		if !strings.Contains(redacted, marker) {
			t.Fatalf("redaction missing %q in %q", marker, redacted)
		}
	}
}

func TestBuildAIInputContainsStructuredReportWithoutRawArtifacts(t *testing.T) {
	report := crashreports.Report{
		ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ParsedSignature: &crashreports.CrashSignature{
			Platform: "linux", Architecture: "x86_64", CrashReason: "SIGSEGV",
			Modules: []crashreports.Module{{DebugFile: "accelerator.ext.so", DebugIdentifier: "debug-id"}},
		},
	}
	input, err := BuildAIInput(report, "ServerID=server-secret\nCommandLine=srcds -ip 192.0.2.44", "#0 in /opt/l4d2/accelerator.ext.so")
	if err != nil {
		t.Fatal(err)
	}
	text := string(input.Body)
	if input.SHA256 == "" || strings.Contains(text, "server-secret") || strings.Contains(text, "/opt/l4d2") || strings.Contains(text, "192.0.2.44") {
		t.Fatalf("input=%+v", input)
	}
	if !strings.Contains(text, "SIGSEGV") || !strings.Contains(text, "accelerator.ext.so") {
		t.Fatalf("structured input=%q", text)
	}
}

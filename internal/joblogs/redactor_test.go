package joblogs

import (
	"os"
	"strings"
	"testing"
)

func TestRedactorRemovesKnownSecretsAndSensitiveFields(t *testing.T) {
	r := NewRedactor(func() []string { return []string{"steam-pass", "github-token"} })
	input := "STEAM_PASSWORD=steam-pass Authorization: Bearer github-token Cookie: session=abc API_TOKEN=token-value"
	got := r.Redact(input)
	for _, secret := range []string{"steam-pass", "github-token", "session=abc", "token-value"} {
		if strings.Contains(got, secret) {
			t.Fatalf("secret %q remained in %q", secret, got)
		}
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("redaction marker missing: %q", got)
	}
}

func TestManagerRedactsBeforeWritingDisk(t *testing.T) {
	root := t.TempDir()
	m, err := Open(root, Options{Redactor: NewRedactor(func() []string { return []string{"disk-secret"} })})
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Append(t.Context(), "job-1", "task", Info, "password=disk-secret")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := readFile(root + "/job-1.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "disk-secret") {
		t.Fatalf("secret written to disk: %q", raw)
	}
}

func readFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	return string(raw), err
}

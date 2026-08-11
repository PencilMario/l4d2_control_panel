package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestAcceleratorSettingsDefaultAndRoundTrip(t *testing.T) {
	const expectedDefaultDownloadURL = "http://sp2.0721play.icu/d/Discord%E5%85%B1%E4%BA%AB%E6%96%87%E4%BB%B6%E5%A4%B9/accelerator-2.6.0-static-x86-linux.zip"
	s, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	got, err := s.AcceleratorSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.DownloadURL != expectedDefaultDownloadURL || got.UseGitHubProxy {
		t.Fatalf("defaults=%+v", got)
	}
	want := AcceleratorSettings{DownloadURL: "https://downloads.example.test/accelerator.zip", UseGitHubProxy: true}
	if err := s.SaveAcceleratorSettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	httpSource := AcceleratorSettings{DownloadURL: "http://downloads.example.test/accelerator.zip"}
	if err := s.SaveAcceleratorSettings(ctx, httpSource); err != nil {
		t.Fatalf("accepted HTTP download source: %v", err)
	}
	got, err = s.AcceleratorSettings(ctx)
	if err != nil || got != httpSource {
		t.Fatalf("settings=%+v err=%v want=%+v", got, err, httpSource)
	}
	for _, value := range []string{"https://", "https://downloads.example.test/a.zip?token=secret", "https://downloads.example.test/a.zip#fragment"} {
		invalid := httpSource
		invalid.DownloadURL = value
		if err := s.SaveAcceleratorSettings(ctx, invalid); err == nil {
			t.Fatalf("accepted invalid download URL %q", value)
		}
	}
	if err := s.SaveAcceleratorSettings(ctx, AcceleratorSettings{}); err != nil {
		t.Fatal(err)
	}
	got, err = s.AcceleratorSettings(ctx)
	if err != nil || got.DownloadURL != "" || got.UseGitHubProxy {
		t.Fatalf("explicitly cleared settings=%+v err=%v", got, err)
	}
}

func TestCrashAnalysisSettingsDefaultAndValidatesEndpoint(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	got, err := s.CrashAnalysisSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Endpoint != "" || got.Model != "" {
		t.Fatalf("defaults=%+v", got)
	}
	want := CrashAnalysisSettings{Endpoint: "http://127.0.0.1:11434/v1", Model: "qwen2.5-coder"}
	if err := s.SaveCrashAnalysisSettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err = s.CrashAnalysisSettings(ctx)
	if err != nil || got != want {
		t.Fatalf("settings=%+v err=%v want=%+v", got, err, want)
	}
	for _, endpoint := range []string{"http://192.0.2.10:11434/v1", "http://example.test/v1", "ftp://example.test/v1", "https://"} {
		invalid := want
		invalid.Endpoint = endpoint
		if err := s.SaveCrashAnalysisSettings(ctx, invalid); err == nil {
			t.Fatalf("accepted invalid endpoint %q", endpoint)
		}
	}
}

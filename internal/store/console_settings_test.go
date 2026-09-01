package store

import (
	"path/filepath"
	"testing"
)

func TestConsoleHistoryLinesDefaultsPersistsAndRejectsInvalidValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.ConsoleHistoryLines()
	if err != nil || got != DefaultConsoleHistoryLines {
		t.Fatalf("default=%d err=%v", got, err)
	}
	if err := s.SetConsoleHistoryLines(MinConsoleHistoryLines); err != nil {
		t.Fatal(err)
	}
	if got, err = s.ConsoleHistoryLines(); err != nil || got != MinConsoleHistoryLines {
		t.Fatalf("min=%d err=%v", got, err)
	}
	if err := s.SetConsoleHistoryLines(MaxConsoleHistoryLines); err != nil {
		t.Fatal(err)
	}
	if got, err = s.ConsoleHistoryLines(); err != nil || got != MaxConsoleHistoryLines {
		t.Fatalf("max=%d err=%v", got, err)
	}
	for _, invalid := range []int{0, -1, MaxConsoleHistoryLines + 1} {
		if err := s.SetConsoleHistoryLines(invalid); err == nil {
			t.Fatalf("expected %d rejected", invalid)
		}
		if got, err := s.ConsoleHistoryLines(); err != nil || got != MaxConsoleHistoryLines {
			t.Fatalf("invalid %d changed value=%d err=%v", invalid, got, err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got, err := s.ConsoleHistoryLines(); err != nil || got != MaxConsoleHistoryLines {
		t.Fatalf("reopen=%d err=%v", got, err)
	}
}

package scheduler

import (
	"testing"
	"time"
)

func TestNextUsesConfiguredTimezoneAndSkipsMissedRuns(t *testing.T) {
	s, err := Parse("0 4 * * *", "Asia/Hong_Kong")
	if err != nil {
		t.Fatal(err)
	}
	after := time.Date(2026, 7, 14, 20, 0, 0, 0, time.FixedZone("UTC", 0))
	next := s.Next(after)
	if next.Before(after) || next.Location().String() != "Asia/Hong_Kong" {
		t.Fatalf("next=%v", next)
	}
}
func TestParseRejectsInvalidCronAndTimezone(t *testing.T) {
	if _, err := Parse("bad", "UTC"); err == nil {
		t.Fatal("accepted bad cron")
	}
	if _, err := Parse("0 4 * * *", "Mars/Base"); err == nil {
		t.Fatal("accepted bad timezone")
	}
}

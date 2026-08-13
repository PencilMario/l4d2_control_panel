package gamelogs

import (
	"context"
	"errors"
	"testing"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

type cleanerStore struct {
	instances []domain.Instance
	sizeMB    int
	err       error
}

func (s cleanerStore) Instances(context.Context) ([]domain.Instance, error) {
	return s.instances, s.err
}

func (s cleanerStore) GameLogMaxFileSizeMB() (int, error) {
	return s.sizeMB, s.err
}

type recordingCleanerManager struct {
	retentionDays []int
	maxBytes      []int64
	errors        map[string]error
}

func (m *recordingCleanerManager) Maintain(_ context.Context, instanceID string, retentionDays int, maxFileSizeBytes int64) (CleanupResult, error) {
	m.retentionDays = append(m.retentionDays, retentionDays)
	m.maxBytes = append(m.maxBytes, maxFileSizeBytes)
	if err := m.errors[instanceID]; err != nil {
		return CleanupResult{}, err
	}
	return CleanupResult{}, nil
}

func TestCleanerUsesExplicitRetentionAndContinuesAfterInstanceFailure(t *testing.T) {
	manager := &recordingCleanerManager{errors: map[string]error{"first": errors.New("first failed")}}
	cleaner := NewCleaner(cleanerStore{
		instances: []domain.Instance{{ID: "first"}, {ID: "second"}},
		sizeMB:    12,
	}, manager)

	err := cleaner.Cleanup(context.Background(), 45)
	if err == nil || !errors.Is(err, manager.errors["first"]) {
		t.Fatalf("error=%v, want first instance error", err)
	}
	if len(manager.retentionDays) != 2 || manager.retentionDays[0] != 45 || manager.retentionDays[1] != 45 {
		t.Fatalf("retention calls=%v", manager.retentionDays)
	}
	if len(manager.maxBytes) != 2 || manager.maxBytes[0] != 12<<20 || manager.maxBytes[1] != 12<<20 {
		t.Fatalf("max size calls=%v", manager.maxBytes)
	}
}

func TestCleanerDoesNotReadLegacyRetentionSetting(t *testing.T) {
	manager := &recordingCleanerManager{errors: map[string]error{}}
	cleaner := NewCleaner(cleanerStore{instances: []domain.Instance{{ID: "instance"}}, sizeMB: 8}, manager)

	if err := cleaner.Cleanup(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	if got := manager.retentionDays[0]; got != 7 {
		t.Fatalf("retention=%d, want explicit value", got)
	}
}

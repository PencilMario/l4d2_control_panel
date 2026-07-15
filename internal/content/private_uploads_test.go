package content

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func uploadHash(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func TestPrivateUploadResumesAndCompletesAtomically(t *testing.T) {
	root := t.TempDir()
	manager := NewPrivateUploadManager(root, 8<<20)
	session, err := manager.Begin("abc", "addons/file.bin", 6, uploadHash([]byte("abcdef")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Write(session.ID, 0, bytes.NewBufferString("abc")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "instances", "abc", "private", "addons", "file.bin")); !os.IsNotExist(err) {
		t.Fatalf("partial target visible: %v", err)
	}
	recovered, err := manager.Recover(session.ID)
	if err != nil || recovered.Offset != 3 {
		t.Fatalf("recover = %+v, %v", recovered, err)
	}
	if _, err = manager.Write(session.ID, 3, bytes.NewBufferString("def")); err != nil {
		t.Fatal(err)
	}
	if err = manager.Complete(session.ID); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "instances", "abc", "private", "addons", "file.bin"))
	if err != nil || string(raw) != "abcdef" {
		t.Fatalf("published = %q, %v", raw, err)
	}
	if _, err = manager.Recover(session.ID); err == nil {
		t.Fatal("completed metadata remains")
	}
}

func TestPrivateUploadRejectsInvalidWritesAndMetadata(t *testing.T) {
	root := t.TempDir()
	manager := NewPrivateUploadManager(root, 6)
	if _, err := manager.Begin("../abc", "file", 1, uploadHash([]byte("x"))); err == nil {
		t.Fatal("unsafe instance accepted")
	}
	if _, err := manager.Begin("abc", "../file", 1, uploadHash([]byte("x"))); err == nil {
		t.Fatal("unsafe path accepted")
	}
	if _, err := manager.Begin("abc", "file", 7, uploadHash([]byte("1234567"))); err == nil {
		t.Fatal("oversize accepted")
	}
	s, err := manager.Begin("abc", "file", 3, uploadHash([]byte("abc")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Write(s.ID, 1, bytes.NewBufferString("a")); err == nil {
		t.Fatal("wrong offset accepted")
	}
	if _, err = manager.Write(s.ID, 0, bytes.NewBufferString("abcd")); err == nil {
		t.Fatal("oversize chunk accepted")
	}
	if _, err = manager.Write(s.ID, 0, bytes.NewBufferString("abd")); err != nil {
		t.Fatal(err)
	}
	if err = manager.Complete(s.ID); err == nil {
		t.Fatal("hash mismatch accepted")
	}
}

func TestPrivateUploadCompleteNeverOverwritesAndCleanupSurvivesRestart(t *testing.T) {
	root := t.TempDir()
	manager := NewPrivateUploadManager(root, 1024)
	s, err := manager.Begin("abc", "file.bin", 3, uploadHash([]byte("new")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Write(s.ID, 0, bytes.NewBufferString("new")); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "instances", "abc", "private", "file.bin")
	if err = os.MkdirAll(filepath.Dir(target), 0750); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(target, []byte("old"), 0640); err != nil {
		t.Fatal(err)
	}
	if err = manager.Complete(s.ID); err == nil {
		t.Fatal("complete overwrote destination")
	}
	if raw, _ := os.ReadFile(target); string(raw) != "old" {
		t.Fatalf("destination = %q", raw)
	}
	if recovered, err := NewPrivateUploadManager(root, 1024).Recover(s.ID); err != nil || recovered.Offset != 3 {
		t.Fatalf("recover = %+v, %v", recovered, err)
	}

	expired, err := manager.Begin("abc", "expired.bin", 1, uploadHash([]byte("x")))
	if err != nil {
		t.Fatal(err)
	}
	_, meta := manager.sessionPaths(filepath.Join(root, "instances", "abc", "backups", "private", "uploads"), expired.ID)
	raw, _ := os.ReadFile(meta)
	var stored PrivateUploadSession
	if err = json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	stored.ExpiresAt = time.Now().Add(-time.Hour)
	if err = writeUploadMetadata(meta, stored); err != nil {
		t.Fatal(err)
	}
	if err = NewPrivateUploadManager(root, 1024).RecoverAll(); err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Recover(expired.ID); err == nil {
		t.Fatal("expired session remains")
	}
}

func TestPrivateApplyReportsTruthfulStages(t *testing.T) {
	root := t.TempDir()
	manager := NewPrivateManager(root, 1024)
	if _, err := manager.Save(context.Background(), "abc", "cfg/a.cfg", []byte("x")); err != nil {
		t.Fatal(err)
	}
	var stages []string
	if err := manager.ApplyChangesWithProgress(context.Background(), "abc", func(stage string) { stages = append(stages, stage) }); err != nil {
		t.Fatal(err)
	}
	want := []string{"snapshot", "restore-lower", "apply-private", "commit"}
	if !slices.Equal(stages, want) {
		t.Fatalf("stages = %v, want %v", stages, want)
	}
}

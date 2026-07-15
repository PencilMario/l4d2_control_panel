package content

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
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

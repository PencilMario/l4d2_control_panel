package content

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"
)

func TestChunkedUploadResumesVerifiesAndMovesAtomically(t *testing.T) {
	root := t.TempDir()
	data := []byte("large-vpk-content")
	sum := sha256.Sum256(data)
	manager, err := NewUploadManager(root)
	if err != nil {
		t.Fatal(err)
	}
	session, err := manager.Begin("maps.vpk", int64(len(data)), hex.EncodeToString(sum[:]))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Write(session.ID, 0, bytes.NewReader(data[:5])); err != nil {
		t.Fatal(err)
	}
	manager, err = NewUploadManager(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Write(session.ID, 5, bytes.NewReader(data[5:])); err != nil {
		t.Fatal(err)
	}
	item, duplicate, err := manager.Complete(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate || item.Name != "maps.vpk" || item.Size != int64(len(data)) {
		t.Fatalf("item=%#v duplicate=%v", item, duplicate)
	}
	if filepath.Dir(item.Path) != filepath.Join(root, "shared-vpk") {
		t.Fatalf("path=%s", item.Path)
	}
	second, err := manager.Begin("copy.vpk", int64(len(data)), hex.EncodeToString(sum[:]))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = manager.Write(second.ID, 0, bytes.NewReader(data))
	existing, duplicate, err := manager.Complete(second.ID)
	if err != nil || !duplicate || existing.Hash != item.Hash {
		t.Fatalf("existing=%#v duplicate=%v err=%v", existing, duplicate, err)
	}
	items, err := manager.List()
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	renamed, err := manager.Rename("maps.vpk", "campaign.vpk")
	if err != nil || renamed.Name != "campaign.vpk" {
		t.Fatalf("renamed=%#v err=%v", renamed, err)
	}
	if err := manager.Delete("campaign.vpk"); err != nil {
		t.Fatal(err)
	}
	items, _ = manager.List()
	if len(items) != 0 {
		t.Fatalf("items after delete=%#v", items)
	}
}
func TestUploadRejectsUnsafeNameOffsetAndHash(t *testing.T) {
	manager, _ := NewUploadManager(t.TempDir())
	if _, err := manager.Begin("../bad.vpk", 1, "00"); err == nil {
		t.Fatal("unsafe name accepted")
	}
	session, err := manager.Begin("safe.vpk", 3, "0000000000000000000000000000000000000000000000000000000000000000")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Write(session.ID, 1, bytes.NewReader([]byte("x"))); err == nil {
		t.Fatal("noncontiguous chunk accepted")
	}
	_, _ = manager.Write(session.ID, 0, bytes.NewReader([]byte("abc")))
	if _, _, err := manager.Complete(session.ID); err == nil {
		t.Fatal("bad hash accepted")
	}
}

package content

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/BenLubar/vpk"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type testVPKEntry struct {
	name string
	data []byte
}

func (e testVPKEntry) Rel() string { return e.name }
func (e testVPKEntry) Open() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(e.data)), nil
}

func TestCleanVPKRemovesClientResourcesAndUpdatesMetadata(t *testing.T) {
	m, err := NewUploadManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	name := "map.vpk"
	target := filepath.Join(m.shared, name)
	entries := []vpk.Entry{testVPKEntry{"maps/test.bsp", []byte("bsp")}, testVPKEntry{"materials/test.vtf", []byte("texture")}, testVPKEntry{"sound/test.wav", []byte("sound")}, testVPKEntry{"cfg/server.cfg", []byte("cfg")}}
	if err := vpk.Create(vpk.SingleVPKCreator(target), entries, -1); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(target)
	raw, _ := os.ReadFile(target)
	sum := sha256.Sum256(raw)
	meta, _ := json.Marshal(SharedVPK{Name: name, Hash: hex.EncodeToString(sum[:]), Size: before.Size()})
	if err := os.WriteFile(target+".json", meta, 0640); err != nil {
		t.Fatal(err)
	}
	result, err := m.Clean(name)
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 2 || result.BeforeSize != before.Size() {
		t.Fatalf("result=%+v", result)
	}
	cleaned, err := vpk.Open(vpk.SingleVPK(target))
	if err != nil {
		t.Fatal(err)
	}
	if got := cleaned.Paths(); len(got) != 2 || cleaned.Entry("maps/test.bsp") == nil || cleaned.Entry("cfg/server.cfg") == nil {
		t.Fatalf("paths=%v", got)
	}
	items, err := m.List()
	if err != nil || len(items) != 1 || items[0].Size != result.AfterSize || items[0].Hash == hex.EncodeToString(sum[:]) {
		t.Fatalf("items=%+v err=%v", items, err)
	}
}

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

func TestChunkedUploadRecoversOffsetFromPartFile(t *testing.T) {
	root := t.TempDir()
	data := []byte("recoverable-vpk")
	sum := sha256.Sum256(data)
	manager, _ := NewUploadManager(root)
	session, err := manager.Begin("recover.vpk", int64(len(data)), hex.EncodeToString(sum[:]))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manager.part(session.ID), data[:5], 0640); err != nil {
		t.Fatal(err)
	}

	manager, _ = NewUploadManager(root)
	if _, err := manager.Write(session.ID, 5, bytes.NewReader(data[5:8])); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(manager.meta(session.ID))
	if err != nil {
		t.Fatal(err)
	}
	var recovered UploadSession
	if err := json.Unmarshal(raw, &recovered); err != nil {
		t.Fatal(err)
	}
	if recovered.Offset != 8 {
		t.Fatalf("offset=%d want=8", recovered.Offset)
	}
	if _, err := manager.Write(session.ID, 8, bytes.NewReader(data[8:])); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Complete(session.ID); err != nil {
		t.Fatal(err)
	}
}

func TestChunkedUploadCompletesWhenPartFileOutranMetadata(t *testing.T) {
	root := t.TempDir()
	data := []byte("complete-after-crash")
	sum := sha256.Sum256(data)
	manager, _ := NewUploadManager(root)
	session, err := manager.Begin("complete.vpk", int64(len(data)), hex.EncodeToString(sum[:]))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manager.part(session.ID), data, 0640); err != nil {
		t.Fatal(err)
	}
	manager, _ = NewUploadManager(root)
	item, duplicate, err := manager.Complete(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate || item.Name != "complete.vpk" || item.Size != int64(len(data)) {
		t.Fatalf("item=%#v duplicate=%v", item, duplicate)
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

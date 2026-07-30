package content

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPackageUploadStoresValidatedVersionAndManifest(t *testing.T) {
	root := t.TempDir()
	raw := packageZip(t, map[string]string{"cfg/plugin.cfg": "x"})
	manager, _ := NewPackageManager(root)
	version, err := manager.AddUpload("plugins.zip", "v1.2.3", bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if version.Version != "v1.2.3" || version.Hash == "" || !version.HotCompatible || len(version.Files) != 1 {
		t.Fatalf("version=%#v", version)
	}
	loaded, err := manager.Get(version.ID)
	if err != nil || loaded.ArchivePath == "" {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
}
func TestPackageUploadRejectsTraversal(t *testing.T) {
	raw := packageZip(t, map[string]string{"../escape": "x"})
	manager, _ := NewPackageManager(t.TempDir())
	if _, err := manager.AddUpload("bad.zip", "v1", bytes.NewReader(raw), int64(len(raw))); err == nil {
		t.Fatal("malicious package accepted")
	}
}
func packageZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "p.zip")
	file, _ := os.Create(path)
	writer := zip.NewWriter(file)
	for name, value := range files {
		entry, _ := writer.Create(name)
		_, _ = entry.Write([]byte(value))
	}
	_ = writer.Close()
	_ = file.Close()
	raw, _ := os.ReadFile(path)
	return raw
}

func TestCleanupUnreferencedSourceVersionsProtectsReferencesAndLatest(t *testing.T) {
	manager, _ := NewPackageManager(t.TempDir())
	raw := packageZip(t, map[string]string{"cfg/plugin.cfg": "x"})
	created := time.Date(2026, 7, 30, 1, 0, 0, 0, time.UTC)
	add := func(filename, version, repository string, at time.Time) PackageVersion {
		t.Helper()
		item, err := manager.AddUpload(filename, version, bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			t.Fatal(err)
		}
		item.SourceRepository = repository
		item.CreatedAt = at
		if err := manager.UpdateMetadata(item); err != nil {
			t.Fatal(err)
		}
		return item
	}
	old := add("plugins.zip", "v1", "owner/repo", created)
	selected := add("plugins.zip", "v2", "owner/repo", created.Add(time.Hour))
	applied := add("plugins.zip", "v3", "owner/repo", created.Add(2*time.Hour))
	latest := add("plugins.zip", "v4", "owner/repo", created.Add(3*time.Hour))
	otherOld := add("other.zip", "v1", "owner/other", created)
	otherLatest := add("other.zip", "v2", "owner/other", created.Add(time.Hour))
	regular := add("manual.zip", "manual", "", created)

	result, err := manager.CleanupUnreferencedSourceVersions(context.Background(), map[string]bool{
		selected.ID: true,
		applied.ID:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 2 || result.ReleasedBytes != old.Size+otherOld.Size {
		t.Fatalf("result=%+v", result)
	}
	for _, removed := range []PackageVersion{old, otherOld} {
		if _, err := manager.Get(removed.ID); !os.IsNotExist(err) {
			t.Fatalf("removed package %s still available: %v", removed.ID, err)
		}
		if _, err := os.Stat(removed.ArchivePath); !os.IsNotExist(err) {
			t.Fatalf("removed archive %s still available: %v", removed.ID, err)
		}
	}
	for _, kept := range []PackageVersion{selected, applied, latest, otherLatest, regular} {
		if _, err := manager.Get(kept.ID); err != nil {
			t.Fatalf("protected package %s removed: %v", kept.ID, err)
		}
	}
}

func TestCleanupUnreferencedSourceVersionsUsesStableTieBreak(t *testing.T) {
	manager, _ := NewPackageManager(t.TempDir())
	raw := packageZip(t, map[string]string{"cfg/plugin.cfg": "x"})
	items := make([]PackageVersion, 2)
	for index := range items {
		item, err := manager.AddUpload("plugins.zip", "same", bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			t.Fatal(err)
		}
		item.SourceRepository = "owner/repo"
		item.CreatedAt = time.Date(2026, 7, 30, 1, 0, 0, 0, time.UTC)
		if err := manager.UpdateMetadata(item); err != nil {
			t.Fatal(err)
		}
		items[index] = item
	}
	kept, removed := items[0], items[1]
	if removed.ID > kept.ID {
		kept, removed = removed, kept
	}
	if _, err := manager.CleanupUnreferencedSourceVersions(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Get(kept.ID); err != nil {
		t.Fatalf("stable latest removed: %v", err)
	}
	if _, err := manager.Get(removed.ID); !os.IsNotExist(err) {
		t.Fatalf("stable old package retained: %v", err)
	}
}

func TestCleanupUnreferencedSourceVersionsContinuesAfterDeleteFailure(t *testing.T) {
	manager, _ := NewPackageManager(t.TempDir())
	raw := packageZip(t, map[string]string{"cfg/plugin.cfg": "x"})
	addPair := func(repository string) (PackageVersion, PackageVersion) {
		t.Helper()
		var pair [2]PackageVersion
		for index := range pair {
			item, err := manager.AddUpload("plugins.zip", string(rune('1'+index)), bytes.NewReader(raw), int64(len(raw)))
			if err != nil {
				t.Fatal(err)
			}
			item.SourceRepository = repository
			item.CreatedAt = time.Date(2026, 7, 30, index+1, 0, 0, 0, time.UTC)
			if err := manager.UpdateMetadata(item); err != nil {
				t.Fatal(err)
			}
			pair[index] = item
		}
		return pair[0], pair[1]
	}
	failed, _ := addPair("owner/failed")
	removed, _ := addPair("owner/removed")
	originalRemove := manager.remove
	manager.remove = func(path string) error {
		if path == failed.ArchivePath {
			return errors.New("remove " + failed.ArchivePath + ": injected failure")
		}
		return originalRemove(path)
	}
	result, err := manager.CleanupUnreferencedSourceVersions(context.Background(), nil)
	if err == nil || result.Failed != 1 || result.Deleted != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if strings.Contains(err.Error(), manager.directory) || strings.Contains(err.Error(), failed.ArchivePath) {
		t.Fatalf("cleanup error leaked managed path: %q", err)
	}
	if _, err := manager.Get(failed.ID); err != nil {
		t.Fatalf("failed candidate metadata removed: %v", err)
	}
	if _, err := manager.Get(removed.ID); !os.IsNotExist(err) {
		t.Fatalf("independent candidate retained: %v", err)
	}
}

func TestCleanupUnreferencedSourceVersionsCountsArchiveRemovedBeforeMetadataFailure(t *testing.T) {
	manager, _ := NewPackageManager(t.TempDir())
	raw := packageZip(t, map[string]string{"cfg/plugin.cfg": "x"})
	var old PackageVersion
	for index := 0; index < 2; index++ {
		item, err := manager.AddUpload("plugins.zip", string(rune('1'+index)), bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			t.Fatal(err)
		}
		item.SourceRepository = "owner/repo"
		item.CreatedAt = time.Date(2026, 7, 30, index+1, 0, 0, 0, time.UTC)
		if err := manager.UpdateMetadata(item); err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			old = item
		}
	}
	metadata := filepath.Join(manager.directory, old.ID+".json")
	originalRemove := manager.remove
	manager.remove = func(path string) error {
		if path == metadata {
			return errors.New("injected metadata failure")
		}
		return originalRemove(path)
	}
	result, err := manager.CleanupUnreferencedSourceVersions(context.Background(), nil)
	if err == nil || result.Failed != 1 || result.Deleted != 0 || result.ReleasedBytes != old.Size {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if _, err := os.Stat(old.ArchivePath); !os.IsNotExist(err) {
		t.Fatalf("archive retained after successful removal: %v", err)
	}
	if _, err := os.Stat(metadata); err != nil {
		t.Fatalf("metadata not retained for retry: %v", err)
	}
}

func TestCleanupUnreferencedSourceVersionsHonorsCanceledContext(t *testing.T) {
	manager, _ := NewPackageManager(t.TempDir())
	raw := packageZip(t, map[string]string{"cfg/plugin.cfg": "x"})
	var old PackageVersion
	for index := 0; index < 2; index++ {
		item, err := manager.AddUpload("plugins.zip", string(rune('1'+index)), bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			t.Fatal(err)
		}
		item.SourceRepository = "owner/repo"
		item.CreatedAt = time.Date(2026, 7, 30, index+1, 0, 0, 0, time.UTC)
		if err := manager.UpdateMetadata(item); err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			old = item
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.CleanupUnreferencedSourceVersions(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if _, err := manager.Get(old.ID); err != nil {
		t.Fatalf("canceled cleanup removed package: %v", err)
	}
}

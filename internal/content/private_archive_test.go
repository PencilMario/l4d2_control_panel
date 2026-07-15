package content

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPrivateZIPImportReplacesWorkspaceAndPreservesMetadata(t *testing.T) {
	root := t.TempDir()
	m := NewPrivateManager(root, 1<<20)
	ctx := context.Background()
	if _, err := m.Save(ctx, "abc", "old/keep.cfg", []byte("old")); err != nil {
		t.Fatal(err)
	}
	if err := m.MakeDir(ctx, "abc", "old-empty"); err != nil {
		t.Fatal(err)
	}
	if err := m.ApplyChanges(ctx, "abc"); err != nil {
		t.Fatal(err)
	}
	snapshotsBefore, err := m.Snapshots(ctx, "abc")
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "instances", "abc", "private-applied.json")
	manifestBefore, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}

	raw := privateZIP(t, []privateZIPEntry{
		{name: "bundle/cfg/new.cfg", body: []byte("new")},
		{name: "bundle/empty/", mode: fs.ModeDir | 0750},
	})
	if err := m.ImportZIP(ctx, "abc", bytes.NewReader(raw), DefaultPrivateArchiveLimits); err != nil {
		t.Fatal(err)
	}

	entries, err := m.Tree(ctx, "abc")
	if err != nil {
		t.Fatal(err)
	}
	assertPrivatePaths(t, entries, "bundle", "bundle/cfg", "bundle/cfg/new.cfg", "bundle/empty")
	if _, err := m.Read(ctx, "abc", "old/keep.cfg"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old workspace file retained: %v", err)
	}
	snapshotsAfter, err := m.Snapshots(ctx, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshotsBefore, snapshotsAfter) {
		t.Fatalf("snapshots changed: before=%v after=%v", snapshotsBefore, snapshotsAfter)
	}
	manifestAfter, err := os.ReadFile(manifestPath)
	if err != nil || !bytes.Equal(manifestBefore, manifestAfter) {
		t.Fatalf("applied manifest changed: equal=%t err=%v", bytes.Equal(manifestBefore, manifestAfter), err)
	}
	diff, err := m.Diff(ctx, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if diff.Summary.Added != 2 || diff.Summary.Modified != 0 || diff.Summary.Deleted != 2 {
		t.Fatalf("diff=%+v", diff)
	}
	gameRoot := filepath.Join(root, "instances", "abc", "game", "left4dead2")
	if game, err := os.ReadFile(filepath.Join(gameRoot, "old", "keep.cfg")); err != nil || string(game) != "old" {
		t.Fatalf("applied game file changed: %q err=%v", game, err)
	}
	if _, err := os.Stat(filepath.Join(gameRoot, "bundle", "cfg", "new.cfg")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("import applied to game directory: %v", err)
	}
}

func TestPrivateZIPExportRoundTripsFilesAndEmptyDirectories(t *testing.T) {
	root := t.TempDir()
	m := NewPrivateManager(root, 1<<20)
	ctx := context.Background()
	if _, err := m.Save(ctx, "abc", "top/data.bin", []byte{0, 1, 2}); err != nil {
		t.Fatal(err)
	}
	if err := m.MakeDir(ctx, "abc", "top/empty"); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if err := m.ExportZIP(ctx, "abc", &archive); err != nil {
		t.Fatal(err)
	}
	if err := m.ImportZIP(ctx, "def", bytes.NewReader(archive.Bytes()), DefaultPrivateArchiveLimits); err != nil {
		t.Fatal(err)
	}
	raw, err := m.Read(ctx, "def", "top/data.bin")
	if err != nil || !bytes.Equal(raw, []byte{0, 1, 2}) {
		t.Fatalf("raw=%v err=%v", raw, err)
	}
	entries, err := m.Tree(ctx, "def")
	if err != nil {
		t.Fatal(err)
	}
	assertPrivatePaths(t, entries, "top", "top/data.bin", "top/empty")
}

func TestPrivateZIPExportEmptyWorkspace(t *testing.T) {
	m := NewPrivateManager(t.TempDir(), 1<<20)
	var archive bytes.Buffer
	if err := m.ExportZIP(context.Background(), "abc", &archive); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.File) != 0 {
		t.Fatalf("entries=%d", len(reader.File))
	}
}

func TestPrivateZIPImportRejectsUnsafeOrConflictingEntriesWithoutChangingWorkspace(t *testing.T) {
	defaults := DefaultPrivateArchiveLimits
	limit := func(change func(*PrivateArchiveLimits)) PrivateArchiveLimits {
		value := defaults
		change(&value)
		return value
	}
	tests := []struct {
		name   string
		raw    []byte
		limits PrivateArchiveLimits
		want   error
	}{
		{name: "malformed", raw: []byte("not a zip"), limits: defaults, want: ErrPrivateArchiveInvalid},
		{name: "parent escape", raw: privateZIP(t, []privateZIPEntry{{name: "../escape.cfg", body: []byte("x")}}), limits: defaults, want: ErrPrivateArchivePath},
		{name: "absolute", raw: privateZIP(t, []privateZIPEntry{{name: "/escape.cfg", body: []byte("x")}}), limits: defaults, want: ErrPrivateArchivePath},
		{name: "drive", raw: privateZIP(t, []privateZIPEntry{{name: "C:/escape.cfg", body: []byte("x")}}), limits: defaults, want: ErrPrivateArchivePath},
		{name: "backslash", raw: privateZIP(t, []privateZIPEntry{{name: `cfg\escape.cfg`, body: []byte("x")}}), limits: defaults, want: ErrPrivateArchivePath},
		{name: "duplicate", raw: privateZIP(t, []privateZIPEntry{{name: "cfg/a.cfg", body: []byte("a")}, {name: "cfg/a.cfg", body: []byte("b")}}), limits: defaults, want: ErrPrivateArchiveConflict},
		{name: "case collision", raw: privateZIP(t, []privateZIPEntry{{name: "Cfg/a.cfg", body: []byte("a")}, {name: "cfg/b.cfg", body: []byte("b")}}), limits: defaults, want: ErrPrivateArchiveConflict},
		{name: "file parent", raw: privateZIP(t, []privateZIPEntry{{name: "cfg", body: []byte("file")}, {name: "cfg/a.cfg", body: []byte("child")}}), limits: defaults, want: ErrPrivateArchiveConflict},
		{name: "symlink", raw: privateZIP(t, []privateZIPEntry{{name: "link", body: []byte("target"), mode: os.ModeSymlink | 0777}}), limits: defaults, want: ErrPrivateArchiveUnsupported},
		{name: "special file", raw: privateZIP(t, []privateZIPEntry{{name: "pipe", mode: os.ModeNamedPipe | 0600}}), limits: defaults, want: ErrPrivateArchiveUnsupported},
		{name: "file count", raw: privateZIP(t, []privateZIPEntry{{name: "a", body: []byte("a")}, {name: "b", body: []byte("b")}}), limits: limit(func(value *PrivateArchiveLimits) { value.MaxFiles = 1 }), want: ErrPrivateArchiveTooLarge},
		{name: "single file size", raw: privateZIP(t, []privateZIPEntry{{name: "a", body: []byte("abcd")}}), limits: limit(func(value *PrivateArchiveLimits) { value.MaxFileBytes = 3 }), want: ErrPrivateArchiveTooLarge},
		{name: "expanded size", raw: privateZIP(t, []privateZIPEntry{{name: "a", body: []byte("abc")}, {name: "b", body: []byte("def")}}), limits: limit(func(value *PrivateArchiveLimits) { value.MaxExpandedBytes = 5 }), want: ErrPrivateArchiveTooLarge},
		{name: "compression ratio", raw: privateZIP(t, []privateZIPEntry{{name: "a", body: bytes.Repeat([]byte("a"), 4096), method: zip.Deflate}}), limits: limit(func(value *PrivateArchiveLimits) { value.MaxCompressionRatio = 2 }), want: ErrPrivateArchiveTooLarge},
		{name: "compressed size", raw: privateZIP(t, []privateZIPEntry{{name: "a", body: []byte("abcd")}}), limits: limit(func(value *PrivateArchiveLimits) { value.MaxCompressedBytes = 8 }), want: ErrPrivateArchiveTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			m := NewPrivateManager(root, 1<<20)
			if _, err := m.Save(context.Background(), "abc", "sentinel.cfg", []byte("before")); err != nil {
				t.Fatal(err)
			}
			err := m.ImportZIP(context.Background(), "abc", bytes.NewReader(test.raw), test.limits)
			if !errors.Is(err, test.want) {
				t.Fatalf("err=%v want=%v", err, test.want)
			}
			raw, readErr := m.Read(context.Background(), "abc", "sentinel.cfg")
			if readErr != nil || string(raw) != "before" {
				t.Fatalf("sentinel=%q err=%v", raw, readErr)
			}
		})
	}
}

func TestPrivateZIPImportRollsBackWhenWorkspacePublicationFails(t *testing.T) {
	root := t.TempDir()
	m := NewPrivateManager(root, 1<<20)
	ctx := context.Background()
	if _, err := m.Save(ctx, "abc", "sentinel.cfg", []byte("before")); err != nil {
		t.Fatal(err)
	}
	setPrivateRestoreFailureHook(func(stage string) error {
		if stage == "journal" {
			return errors.New("injected journal failure")
		}
		return nil
	})
	t.Cleanup(func() { setPrivateRestoreFailureHook(nil) })
	raw := privateZIP(t, []privateZIPEntry{{name: "new.cfg", body: []byte("after")}})
	if err := m.ImportZIP(ctx, "abc", bytes.NewReader(raw), DefaultPrivateArchiveLimits); err == nil {
		t.Fatal("import succeeded")
	}
	current, err := m.Read(ctx, "abc", "sentinel.cfg")
	if err != nil || string(current) != "before" {
		t.Fatalf("sentinel=%q err=%v", current, err)
	}
	works, err := filepath.Glob(filepath.Join(root, "instances", "abc", "backups", "private", "restore-*"))
	if err != nil || len(works) != 0 {
		t.Fatalf("restore work=%v err=%v", works, err)
	}
}

func TestPrivateZIPRecoveryCleansPreJournalWorkspace(t *testing.T) {
	root := t.TempDir()
	m := NewPrivateManager(root, 1<<20)
	ctx := context.Background()
	if _, err := m.Save(ctx, "abc", "sentinel.cfg", []byte("before")); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(root, "instances", "abc", "backups", "private", "restore-"+uuid.NewString())
	if err := os.MkdirAll(filepath.Join(work, "staged"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "archive.zip"), []byte("partial"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := m.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(work); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pre-journal work remains: %v", err)
	}
	raw, err := m.Read(ctx, "abc", "sentinel.cfg")
	if err != nil || string(raw) != "before" {
		t.Fatalf("sentinel=%q err=%v", raw, err)
	}
}

func TestPrivateZIPRecoverySkipsCleanInstanceLease(t *testing.T) {
	root := t.TempDir()
	m := NewPrivateManager(root, 1<<20)
	if _, err := m.Save(context.Background(), "abc", "sentinel.cfg", []byte("before")); err != nil {
		t.Fatal(err)
	}
	lock := m.instanceLock("abc")
	lock.Lock()
	done := make(chan error, 1)
	go func() { done <- m.Recover(context.Background()) }()
	select {
	case err := <-done:
		lock.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		lock.Unlock()
		<-done
		t.Fatal("recovery waited for a clean instance lease")
	}
}

type privateZIPEntry struct {
	name   string
	body   []byte
	mode   fs.FileMode
	method uint16
}

func privateZIP(t *testing.T, entries []privateZIPEntry) []byte {
	t.Helper()
	var raw bytes.Buffer
	w := zip.NewWriter(&raw)
	for _, item := range entries {
		header := &zip.FileHeader{Name: item.name, Method: item.method}
		if item.mode != 0 {
			header.SetMode(item.mode)
		}
		entry, err := w.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write(item.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return raw.Bytes()
}

func assertPrivatePaths(t *testing.T, entries []PrivateEntry, want ...string) {
	t.Helper()
	got := make([]string, len(entries))
	for index, entry := range entries {
		got[index] = entry.Path
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("paths=%v want=%v", got, want)
	}
}

func TestPrivateZIPImportRejectsEncryptedEntry(t *testing.T) {
	raw := privateZIP(t, []privateZIPEntry{{name: "secret.cfg", body: []byte("secret")}})
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.File) != 1 {
		t.Fatalf("entries=%d", len(reader.File))
	}
	for offset := 0; offset+10 <= len(raw); offset++ {
		switch binary.LittleEndian.Uint32(raw[offset : offset+4]) {
		case 0x04034b50:
			raw[offset+6] |= 1
		case 0x02014b50:
			raw[offset+8] |= 1
		}
	}
	m := NewPrivateManager(t.TempDir(), 1<<20)
	err = m.ImportZIP(context.Background(), "abc", bytes.NewReader(raw), DefaultPrivateArchiveLimits)
	if !errors.Is(err, ErrPrivateArchiveUnsupported) && !strings.Contains(strings.ToLower(err.Error()), "encrypted") {
		t.Fatalf("err=%v", err)
	}
}

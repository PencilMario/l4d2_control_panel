//go:build unix

package gamelogs

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestReadAPIsRejectFIFO(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "instances", "i", "logs", "game", "special.fifo")
	if err := secureMkdirAll(root, filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(root, Options{})
	if _, err := manager.Tree(context.Background(), "i"); err == nil {
		t.Fatal("Tree accepted FIFO")
	}
	if _, err := manager.Preview(context.Background(), "i", "game", "special.fifo", 10); err == nil {
		t.Fatal("Preview accepted FIFO")
	}
	if _, _, err := manager.ResolveDownload("i", "game", "special.fifo"); err == nil {
		t.Fatal("ResolveDownload accepted FIFO")
	}
}

func TestPreviewOpenFileReadsOpenedInodeAfterRenameReplacement(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "server.log")
	writeFile(t, path, "old-log")
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	initial, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(directory, "server.log.1")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, "replacement")

	preview, err := previewOpenFile(file, initial, 20)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Text != "old-log" || preview.Size != 7 || preview.Truncated {
		t.Fatalf("preview read replacement instead of opened inode: %+v", preview)
	}
}

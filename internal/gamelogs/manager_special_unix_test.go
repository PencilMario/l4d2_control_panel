//go:build unix

package gamelogs

import (
	"context"
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

package safepath

import (
	"path/filepath"
	"testing"
)

func TestJoinRejectsEscapeAndAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"../secret", "/etc/passwd", `C:\Windows\system.ini`} {
		if _, err := Join(root, name); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	got, err := Join(root, "cfg/server.cfg")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(root, "cfg", "server.cfg") {
		t.Fatalf("got %q", got)
	}
}

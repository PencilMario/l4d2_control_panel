package safepath

import (
	"path/filepath"
	"testing"
)

func TestJoinRejectsEscapeAndAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	tests := []string{
		"../secret",
		"/etc/passwd",
		`C:\Windows\system.ini`,
		"C:/Windows/system.ini",
		"C:relative",
		`\\server\share`,
		"//server/share",
		`..\secret`,
		`nested\..\..\secret`,
	}
	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Join(root, name); err == nil {
				t.Fatalf("accepted %q", name)
			}
		})
	}

	for _, test := range []struct {
		name string
		want string
	}{
		{name: "cfg/server.cfg", want: filepath.Join(root, "cfg", "server.cfg")},
		{name: `nested\file`, want: filepath.Join(root, "nested", "file")},
		{name: "ordinary.txt", want: filepath.Join(root, "ordinary.txt")},
	} {
		t.Run("valid_"+test.name, func(t *testing.T) {
			got, err := Join(root, test.name)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

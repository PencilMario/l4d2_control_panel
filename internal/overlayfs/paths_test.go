package overlayfs

import (
	"path/filepath"
	"testing"
)

func TestPathsResolveConfinedMount(t *testing.T) {
	root := t.TempDir()
	paths := Paths{Root: root}
	got, err := paths.Mount("instance-1", "release-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Lower != filepath.Join(root, "game", "releases", "release-1") || got.Upper != filepath.Join(root, "instances", "instance-1", "overlay", "upper") || got.Work != filepath.Join(root, "instances", "instance-1", "overlay", "work") || got.Merged != filepath.Join(root, "instances", "instance-1", "overlay", "merged") {
		t.Fatalf("mount paths = %#v", got)
	}
}

func TestPathsRejectUnsafeIdentifiers(t *testing.T) {
	paths := Paths{Root: t.TempDir()}
	for _, value := range []string{"", ".", "..", "../escape", "a/b", `a\\b`} {
		if _, err := paths.Mount(value, "release"); err == nil {
			t.Fatalf("instance %q accepted", value)
		}
		if _, err := paths.Mount("instance", value); err == nil {
			t.Fatalf("release %q accepted", value)
		}
	}
}

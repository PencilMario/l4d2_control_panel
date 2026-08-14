package systemlibs

import (
	"reflect"
	"testing"
)

func TestIsAllowedLibraryNameRestrictsSystemRuntimeNames(t *testing.T) {
	for _, test := range []struct {
		name  string
		allow bool
	}{
		{name: "libc.so.6", allow: true},
		{name: "libc-2.31.so", allow: true},
		{name: "libstdc++.so.6.0.30", allow: true},
		{name: "libgame.so", allow: false},
		{name: "libc.so", allow: false},
		{name: "", allow: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := IsAllowedLibraryName(test.name); got != test.allow {
				t.Fatalf("allowed=%v want=%v", got, test.allow)
			}
		})
	}
}

func TestIsAllowedContainerPathRejectsTraversalAndUnknownDirectories(t *testing.T) {
	for _, value := range []string{
		"/lib/x86_64-linux-gnu/libc.so.6",
		"/usr/lib/x86_64-linux-gnu/libc.so.6",
		"/tmp/libc.so.6",
		"/lib/x86_64-linux-gnu/../libc.so.6",
		"/lib/x86_64-linux-gnu/libgame.so",
		"libc.so.6",
	} {
		want := value == "/lib/x86_64-linux-gnu/libc.so.6" || value == "/usr/lib/x86_64-linux-gnu/libc.so.6"
		if got := IsAllowedContainerPath(value); got != want {
			t.Fatalf("path=%q allowed=%v want=%v", value, got, want)
		}
	}
}

func TestCandidatePathsUseArchitectureSpecificDirectories(t *testing.T) {
	want := []string{"/lib/x86_64-linux-gnu/libc.so.6", "/usr/lib/x86_64-linux-gnu/libc.so.6", "/lib64/libc.so.6", "/usr/lib64/libc.so.6", "/lib/libc.so.6"}
	if got := CandidatePaths("/usr/lib/libc.so.6", "x86_64"); !reflect.DeepEqual(got, want) {
		t.Fatalf("paths=%v want=%v", got, want)
	}
	if got := CandidatePaths("libgame.so", "x86_64"); got != nil {
		t.Fatalf("unknown library paths=%v", got)
	}
}

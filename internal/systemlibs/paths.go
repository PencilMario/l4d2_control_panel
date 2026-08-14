package systemlibs

import (
	"path"
	"strings"
)

var knownNames = map[string]struct{}{
	"ld-linux.so.2":        {},
	"ld-linux-x86-64.so.2": {},
	"libc.so.6":            {},
	"libdl.so.2":           {},
	"libgcc_s.so.1":        {},
	"libm.so.6":            {},
	"libpthread.so.0":      {},
	"libresolv.so.2":       {},
	"librt.so.1":           {},
	"libstdc++.so.6":       {},
	"libutil.so.1":         {},
	"libz.so.1":            {},
}

var libraryDirectories = map[string][]string{
	"x86": {
		"/lib/i386-linux-gnu", "/usr/lib/i386-linux-gnu", "/lib32", "/usr/lib32", "/lib",
	},
	"x86_64": {
		"/lib/x86_64-linux-gnu", "/usr/lib/x86_64-linux-gnu", "/lib64", "/usr/lib64", "/lib",
	},
	"arm": {
		"/lib/arm-linux-gnueabihf", "/usr/lib/arm-linux-gnueabihf", "/lib", "/usr/lib",
	},
	"arm64": {
		"/lib/aarch64-linux-gnu", "/usr/lib/aarch64-linux-gnu", "/lib64", "/usr/lib64", "/lib",
	},
}

func IsAllowedContainerPath(value string) bool {
	if value == "" || strings.ContainsAny(value, "\\\x00\r\n") || !path.IsAbs(value) || path.Clean(value) != value {
		return false
	}
	if !IsAllowedLibraryName(path.Base(value)) {
		return false
	}
	for _, directory := range allLibraryDirectories() {
		if value == directory+"/"+path.Base(value) {
			return true
		}
	}
	return false
}

func IsAllowedLibraryName(name string) bool {
	if _, ok := knownNames[name]; ok {
		return true
	}
	if strings.HasPrefix(name, "libc-") && strings.HasSuffix(name, ".so") {
		return true
	}
	for known := range knownNames {
		if strings.HasPrefix(name, known+".") {
			return true
		}
	}
	return false
}

func CandidatePaths(debugFile, architecture string) []string {
	name := path.Base(strings.ReplaceAll(debugFile, "\\", "/"))
	if !IsAllowedLibraryName(name) {
		return nil
	}
	directories := libraryDirectories[strings.ToLower(strings.TrimSpace(architecture))]
	if len(directories) == 0 {
		directories = []string{"/lib", "/usr/lib"}
	}
	paths := make([]string, 0, len(directories))
	for _, directory := range directories {
		paths = append(paths, directory+"/"+name)
	}
	return paths
}

func allLibraryDirectories() []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, directories := range libraryDirectories {
		for _, directory := range directories {
			if _, ok := seen[directory]; ok {
				continue
			}
			seen[directory] = struct{}{}
			result = append(result, directory)
		}
	}
	return result
}

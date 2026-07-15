package archive

import (
	"archive/zip"
	"errors"
	"github.com/not0721here/l4d2-control-panel/internal/safepath"
	"io/fs"
	"strings"
)

type Limits struct {
	MaxFiles            int
	MaxBytes            uint64
	MaxFileBytes        uint64
	MaxCompressionRatio float64
}
type Entry struct {
	Path string
	Size uint64
}
type Manifest struct {
	Entries       []Entry
	TotalBytes    uint64
	HotCompatible bool
}

var hotPrefixes = []string{"addons/sourcemod/configs/", "addons/sourcemod/data/", "addons/sourcemod/gamedata/", "addons/sourcemod/plugins/", "addons/sourcemod/translations/", "scripts/", "cfg/"}

func InspectZip(path string, limits Limits) (Manifest, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return Manifest{}, err
	}
	defer r.Close()
	m := Manifest{}
	if limits.MaxFileBytes == 0 {
		limits.MaxFileBytes = 2 << 30
	}
	if limits.MaxCompressionRatio == 0 {
		limits.MaxCompressionRatio = 400
	}
	if len(r.File) > limits.MaxFiles {
		return m, errors.New("archive file count limit exceeded")
	}
	root := CommonRoot(r.File)
	for _, f := range r.File {
		if f.Flags&1 != 0 {
			return m, errors.New("encrypted archive entries are forbidden")
		}
		if f.Mode()&fs.ModeSymlink != 0 {
			return m, errors.New("archive symlinks are forbidden")
		}
		if _, err := safepath.Join("root", f.Name); err != nil {
			return m, err
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if f.UncompressedSize64 > limits.MaxFileBytes {
			return m, errors.New("archive single-file size limit exceeded")
		}
		compressed := f.CompressedSize64
		if compressed == 0 {
			compressed = 1
		}
		if float64(f.UncompressedSize64)/float64(compressed) > limits.MaxCompressionRatio {
			return m, errors.New("archive compression ratio limit exceeded")
		}
		m.TotalBytes += f.UncompressedSize64
		if m.TotalBytes > limits.MaxBytes {
			return m, errors.New("archive expanded size limit exceeded")
		}
		name := NormalizePath(f.Name, root)
		m.HotCompatible = m.HotCompatible || IsHotPath(name)
		m.Entries = append(m.Entries, Entry{Path: name, Size: f.UncompressedSize64})
	}
	return m, nil
}

func IsHotPath(name string) bool {
	for _, prefix := range hotPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func CommonRoot(files []*zip.File) string {
	root := ""
	gameRoots := map[string]bool{
		"addons": true, "bin": true, "cfg": true, "download": true,
		"maps": true, "materials": true, "media": true, "models": true,
		"resource": true, "scripts": true, "sound": true,
	}
	for _, file := range files {
		if file.FileInfo().IsDir() {
			continue
		}
		name := strings.TrimPrefix(strings.ReplaceAll(file.Name, "\\", "/"), "./")
		parts := strings.SplitN(name, "/", 2)
		if len(parts) != 2 || parts[0] == "" {
			return ""
		}
		if root == "" {
			root = parts[0]
			if gameRoots[strings.ToLower(root)] {
				return ""
			}
		} else if root != parts[0] {
			return ""
		}
	}
	return root
}

func NormalizePath(name, root string) string {
	name = strings.TrimPrefix(strings.ReplaceAll(name, "\\", "/"), "./")
	if root != "" {
		name = strings.TrimPrefix(name, root+"/")
	}
	return name
}

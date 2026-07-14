package archive

import (
	"archive/zip"
	"errors"
	"github.com/not0721here/l4d2-control-panel/internal/safepath"
	"io/fs"
	"strings"
)

type Limits struct {
	MaxFiles int
	MaxBytes uint64
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
	m := Manifest{HotCompatible: true}
	if len(r.File) > limits.MaxFiles {
		return m, errors.New("archive file count limit exceeded")
	}
	for _, f := range r.File {
		if f.Mode()&fs.ModeSymlink != 0 {
			return m, errors.New("archive symlinks are forbidden")
		}
		if _, err := safepath.Join("root", f.Name); err != nil {
			return m, err
		}
		if f.FileInfo().IsDir() {
			continue
		}
		m.TotalBytes += f.UncompressedSize64
		if m.TotalBytes > limits.MaxBytes {
			return m, errors.New("archive expanded size limit exceeded")
		}
		name := strings.TrimPrefix(strings.ReplaceAll(f.Name, "\\", "/"), "./")
		hot := false
		for _, prefix := range hotPrefixes {
			if strings.HasPrefix(name, prefix) {
				hot = true
				break
			}
		}
		m.HotCompatible = m.HotCompatible && hot
		m.Entries = append(m.Entries, Entry{Path: name, Size: f.UncompressedSize64})
	}
	return m, nil
}

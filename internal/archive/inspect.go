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
	m := Manifest{HotCompatible: true}
	if limits.MaxFileBytes == 0 {
		limits.MaxFileBytes = 2 << 30
	}
	if limits.MaxCompressionRatio == 0 {
		limits.MaxCompressionRatio = 200
	}
	if len(r.File) > limits.MaxFiles {
		return m, errors.New("archive file count limit exceeded")
	}
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

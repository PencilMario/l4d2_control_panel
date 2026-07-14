package content

import (
	"github.com/not0721here/l4d2-control-panel/internal/safepath"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func ApplyTree(source, target string) error {
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil || rel == "." {
			return err
		}
		dst, err := safepath.Join(target, filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fs.ErrInvalid
		}
		if d.IsDir() {
			return os.MkdirAll(dst, 0750)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0750); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		tmp := dst + ".tmp"
		out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0640)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		return os.Rename(tmp, dst)
	})
}

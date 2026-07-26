package vpkcleaner

import (
	"bytes"
	"errors"
	"io"
	"os"
	"time"

	"github.com/BenLubar/vpk"
	"github.com/not0721here/l4d2-control-panel/internal/vpkpolicy"
)

type Result struct {
	Data    []byte
	Removed int
}

func CleanBytes(data []byte) (Result, error) {
	pak, err := vpk.Open(bytesOpener{data: data})
	if err != nil {
		return Result{}, err
	}
	kept := make([]vpk.Entry, 0, len(pak.Paths()))
	removed := 0
	for _, rel := range pak.Paths() {
		if vpkpolicy.ShouldRemove(rel) {
			removed++
			continue
		}
		kept = append(kept, pak.Entry(rel))
	}
	var output bytes.Buffer
	if err := vpk.Create(bufferCreator{buffer: &output}, kept, -1); err != nil {
		return Result{}, err
	}
	if _, err := vpk.Open(bytesOpener{data: output.Bytes()}); err != nil {
		return Result{}, err
	}
	return Result{Data: append([]byte(nil), output.Bytes()...), Removed: removed}, nil
}

type bytesOpener struct{ data []byte }

func (o bytesOpener) Main() (vpk.File, error) {
	return &memoryFile{Reader: bytes.NewReader(o.data), size: int64(len(o.data))}, nil
}
func (o bytesOpener) Archive(int16) (vpk.File, error) { return nil, os.ErrNotExist }

type memoryFile struct {
	*bytes.Reader
	size int64
}

func (f *memoryFile) Close() error               { return nil }
func (f *memoryFile) Stat() (os.FileInfo, error) { return fileInfo{size: f.size}, nil }

type fileInfo struct{ size int64 }

func (f fileInfo) Name() string     { return "memory.vpk" }
func (f fileInfo) Size() int64      { return f.size }
func (fileInfo) Mode() os.FileMode  { return 0 }
func (fileInfo) ModTime() time.Time { return time.Time{} }
func (fileInfo) IsDir() bool        { return false }
func (fileInfo) Sys() any           { return nil }

type bufferCreator struct{ buffer *bytes.Buffer }

func (c bufferCreator) Main() (io.WriteCloser, error) { return nopWriteCloser{Writer: c.buffer}, nil }
func (bufferCreator) Archive(int16) (io.WriteCloser, error) {
	return nil, errors.New("single VPK only")
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

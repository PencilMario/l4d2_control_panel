package crashartifacts

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"path"
	"strings"
)

var (
	errArchiveEntryNotRegular = errors.New("archive entry is not a regular file")
	errArchiveEntryUnexpected = errors.New("archive entry name is unexpected")
	errArchiveFileTooLarge    = errors.New("archive file exceeds size limit")
)

type archiveEntry struct {
	Content  []byte
	Linkname string
	Regular  bool
}

func readArchiveEntry(ctx context.Context, source io.Reader, expectedName string, maxBytes int64) (archiveEntry, error) {
	if err := ctx.Err(); err != nil {
		return archiveEntry{}, err
	}
	reader := tar.NewReader(source)
	header, err := reader.Next()
	if err != nil {
		return archiveEntry{}, err
	}
	if path.Base(strings.ReplaceAll(header.Name, "\\", "/")) != expectedName {
		return archiveEntry{}, errArchiveEntryUnexpected
	}
	if header.Typeflag == tar.TypeSymlink {
		return archiveEntry{Linkname: header.Linkname}, nil
	}
	if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
		return archiveEntry{}, errArchiveEntryNotRegular
	}
	if header.Size < 0 || header.Size > maxBytes {
		return archiveEntry{}, errArchiveFileTooLarge
	}
	var output bytes.Buffer
	if _, err := io.Copy(&output, contextReader{ctx: ctx, reader: io.LimitReader(reader, header.Size)}); err != nil {
		return archiveEntry{}, err
	}
	if int64(output.Len()) != header.Size {
		return archiveEntry{}, io.ErrUnexpectedEOF
	}
	return archiveEntry{Content: output.Bytes(), Regular: true}, nil
}

func copyArchiveFile(ctx context.Context, source io.Reader, expectedName string, destination io.Writer, maxBytes int64) (int64, error) {
	entry, err := readArchiveEntry(ctx, source, expectedName, maxBytes)
	if err != nil {
		return 0, err
	}
	if !entry.Regular {
		return 0, errArchiveEntryNotRegular
	}
	return io.Copy(destination, bytes.NewReader(entry.Content))
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}

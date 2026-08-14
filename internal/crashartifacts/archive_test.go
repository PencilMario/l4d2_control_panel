package crashartifacts

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

func tarBytes(t *testing.T, header *tar.Header, content []byte) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := tar.NewWriter(&output)
	header.Size = int64(len(content))
	if err := writer.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestCopyArchiveFileCopiesOnlyRegularExpectedFile(t *testing.T) {
	archive := tarBytes(t, &tar.Header{Name: "libc.so.6", Mode: 0o755, Typeflag: tar.TypeReg}, []byte("ELF-bytes"))
	var destination bytes.Buffer
	size, err := copyArchiveFile(context.Background(), bytes.NewReader(archive), "libc.so.6", &destination, 1024)
	if err != nil || size != int64(len("ELF-bytes")) || destination.String() != "ELF-bytes" {
		t.Fatalf("size=%d content=%q err=%v", size, destination.String(), err)
	}
}

func TestCopyArchiveFileRejectsSymlinkAndDirectory(t *testing.T) {
	for _, header := range []*tar.Header{{Name: "libc.so.6", Typeflag: tar.TypeDir}} {
		archive := tarBytes(t, header, nil)
		var destination bytes.Buffer
		if _, err := copyArchiveFile(context.Background(), bytes.NewReader(archive), "libc.so.6", &destination, 1024); !errors.Is(err, errArchiveEntryNotRegular) {
			t.Fatalf("header=%#v err=%v", header, err)
		}
	}
}

func TestReadArchiveEntryReturnsRelativeSymlinkTarget(t *testing.T) {
	archive := tarBytes(t, &tar.Header{Name: "libc.so.6", Typeflag: tar.TypeSymlink, Linkname: "libc-2.35.so"}, nil)
	entry, err := readArchiveEntry(context.Background(), bytes.NewReader(archive), "libc.so.6", 1024)
	if err != nil || entry.Regular || entry.Linkname != "libc-2.35.so" {
		t.Fatalf("entry=%#v err=%v", entry, err)
	}
}

func TestCopyArchiveFileRejectsUnexpectedNameAndOversize(t *testing.T) {
	archive := tarBytes(t, &tar.Header{Name: "other.so", Typeflag: tar.TypeReg}, []byte("ELF"))
	var destination bytes.Buffer
	if _, err := copyArchiveFile(context.Background(), bytes.NewReader(archive), "libc.so.6", &destination, 1024); !errors.Is(err, errArchiveEntryUnexpected) {
		t.Fatalf("unexpected name err=%v", err)
	}

	archive = tarBytes(t, &tar.Header{Name: "libc.so.6", Typeflag: tar.TypeReg}, []byte("too-large"))
	destination.Reset()
	if _, err := copyArchiveFile(context.Background(), bytes.NewReader(archive), "libc.so.6", &destination, 3); !errors.Is(err, errArchiveFileTooLarge) {
		t.Fatalf("oversize err=%v", err)
	}
}

func TestCopyArchiveFileHonorsCanceledContext(t *testing.T) {
	archive := tarBytes(t, &tar.Header{Name: "libc.so.6", Typeflag: tar.TypeReg}, []byte("ELF"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := copyArchiveFile(ctx, bytes.NewReader(archive), "libc.so.6", io.Discard, 1024); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

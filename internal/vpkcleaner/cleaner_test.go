package vpkcleaner

import (
	"bytes"
	"io"
	"testing"

	"github.com/BenLubar/vpk"
)

type testEntry struct {
	name string
	data []byte
}

func (e testEntry) Rel() string                  { return e.name }
func (e testEntry) Open() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(e.data)), nil }

func TestCleanBytesUsesSharedCleanupPolicy(t *testing.T) {
	bsp := bytes.Repeat([]byte("bsp-data-"), 256*1024)
	var source bytes.Buffer
	if err := vpk.Create(bufferCreator{buffer: &source}, []vpk.Entry{
		testEntry{"maps/test.bsp", bsp},
		testEntry{"materials/test.vtf", []byte("vtf")},
		testEntry{"cfg/server.cfg", []byte("cfg")},
	}, -1); err != nil {
		t.Fatal(err)
	}
	result, err := CleanBytes(source.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 1 {
		t.Fatalf("removed=%d", result.Removed)
	}
	pak, err := vpk.Open(bytesOpener{data: result.Data})
	if err != nil {
		t.Fatal(err)
	}
	if pak.Entry("materials/test.vtf") != nil || pak.Entry("maps/test.bsp") == nil || pak.Entry("cfg/server.cfg") == nil {
		t.Fatalf("paths=%v", pak.Paths())
	}
	reader, err := pak.Entry("maps/test.bsp").Open()
	if err != nil {
		t.Fatal(err)
	}
	cleanedBSP, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || !bytes.Equal(cleanedBSP, bsp) {
		t.Fatalf("cleaned BSP size=%d want=%d err=%v", len(cleanedBSP), len(bsp), err)
	}
}

package disk

import "testing"

func TestAvailableReportsWritableFilesystemSpace(t *testing.T) {
	available, err := Available(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if available == 0 {
		t.Fatal("expected available disk space")
	}
}

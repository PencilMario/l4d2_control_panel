package httpapi

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

func TestAppendConsoleHistoryKeepsNewestLinesAcrossFrames(t *testing.T) {
	first := strings.Builder{}
	for index := 1; index <= 750; index++ {
		_, _ = first.WriteString("old-" + strconv.Itoa(index) + "\n")
	}
	second := strings.Builder{}
	for index := 1; index <= 400; index++ {
		_, _ = second.WriteString("new-" + strconv.Itoa(index) + "\n")
	}

	history := appendConsoleHistory(nil, []byte(first.String()))
	history = appendConsoleHistory(history, []byte(second.String()))

	lines := strings.Split(strings.TrimSuffix(string(history), "\n"), "\n")
	if len(lines) != maxConsoleHistoryLines {
		t.Fatalf("lines=%d, want %d", len(lines), maxConsoleHistoryLines)
	}
	if lines[0] != "old-151" || lines[len(lines)-1] != "new-400" {
		t.Fatalf("history endpoints=%q...%q", lines[0], lines[len(lines)-1])
	}
}

func TestAppendConsoleHistoryCapsUnterminatedOutputByBytes(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), maxConsoleHistoryBytes+1)
	history := appendConsoleHistory(nil, payload)

	if len(history) != maxConsoleHistoryBytes {
		t.Fatalf("history bytes=%d, want %d", len(history), maxConsoleHistoryBytes)
	}
	if !bytes.Equal(history, payload[1:]) {
		t.Fatal("history did not retain the newest bytes")
	}
}

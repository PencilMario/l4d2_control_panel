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

func TestConsoleOutputNormalizerDropsBlankRefreshLinesAndPreservesPartialLines(t *testing.T) {
	if output := normalizeConsolePayload([]byte("\r\r\n\r\r\nsta")); string(output) != "sta" {
		t.Fatalf("partial output=%q, want %q", output, "sta")
	}
	output := normalizeConsolePayload([]byte("tus\r\r\n\r\r\n"))
	if string(output) != "tus\n" {
		t.Fatalf("normalized output=%q, want %q", output, "tus\n")
	}
	if output = normalizeConsolePayload([]byte("\r\r\n\r\r\n")); len(output) != 0 {
		t.Fatalf("blank refresh output=%q, want no frame", output)
	}
}

func TestConsoleOutputNormalizerPreservesPromptAndControlCharacters(t *testing.T) {
	output := normalizeConsolePayload([]byte("\x1b[2J\x1b[Hhello\r\n"))
	if string(output) != "\x1b[2J\x1b[Hhello\n" {
		t.Fatalf("normalized control output=%q", output)
	}
}

func TestConsoleHubRetainsBurstWithinSubscriberFrameBuffer(t *testing.T) {
	hub := &consoleHub{sessions: make(map[string]*consoleSession)}
	session := &consoleSession{
		instanceID:  "instance",
		subscribers: make(map[*consoleSubscriber]struct{}),
	}
	hub.sessions[session.instanceID] = session
	updates := newConsoleSubscriber()
	session.subscribers[updates] = struct{}{}
	payload := bytes.Repeat([]byte("x"), 4*1024)

	for index := 0; index < 128; index++ {
		hub.publish(session, payload)
	}

	for index := 0; index < 128; index++ {
		frame, open := updates.next()
		if !open {
			t.Fatalf("subscriber closed after %d frames", index)
		}
		if !bytes.Equal(frame, payload) {
			t.Fatalf("frame %d differed from payload", index)
		}
	}
}

func TestConsoleHubDoesNotDisconnectSubscriberWhenBurstExceedsBuffer(t *testing.T) {
	hub := &consoleHub{sessions: make(map[string]*consoleSession)}
	session := &consoleSession{
		instanceID:  "instance",
		subscribers: make(map[*consoleSubscriber]struct{}),
	}
	hub.sessions[session.instanceID] = session
	updates := newConsoleSubscriber()
	session.subscribers[updates] = struct{}{}
	payload := []byte("console burst\n")

	for index := 0; index <= 256; index++ {
		hub.publish(session, payload)
	}

	for index := 0; index <= 256; index++ {
		if _, open := updates.next(); !open {
			t.Fatalf("subscriber closed after %d frames", index)
		}
	}
}

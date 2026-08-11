package crashanalysis

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type recordingCommand struct {
	path string
	args []string
	out  []byte
	err  error
}

func (r *recordingCommand) Run(_ context.Context, path string, args ...string) ([]byte, error) {
	r.path = path
	r.args = append([]string(nil), args...)
	return r.out, r.err
}

func TestStackwalkerPassesOnlyDumpAndSymbolRootAndBoundsOutput(t *testing.T) {
	command := &recordingCommand{out: []byte("thread 0\nframe 0\n")}
	stackwalker, err := NewStackwalker(StackwalkConfig{
		Path:           "/usr/local/bin/minidump_stackwalk",
		SymbolRoot:     "/panel/symbols",
		Timeout:        time.Second,
		MaxOutputBytes: 1024,
		Command:        command,
		ToolVersion:    "test-stackwalk",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := stackwalker.Run(context.Background(), "/panel/incoming/report.dmp")
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != string(command.out) || result.Tool != "test-stackwalk" || command.path != stackwalker.Path || len(command.args) != 2 || command.args[0] != "/panel/incoming/report.dmp" || command.args[1] != "/panel/symbols" {
		t.Fatalf("result=%+v command=%+v", result, command)
	}
	command.out = []byte(strings.Repeat("x", 1025))
	if _, err := stackwalker.Run(context.Background(), "/panel/incoming/report.dmp"); !errors.Is(err, ErrStackwalkOutputTooLarge) {
		t.Fatalf("oversized output error=%v", err)
	}
}

func TestStackwalkerClassifiesTimeoutAndCommandFailure(t *testing.T) {
	command := &recordingCommand{err: context.DeadlineExceeded}
	stackwalker, err := NewStackwalker(StackwalkConfig{Path: "stackwalk", SymbolRoot: "symbols", Timeout: time.Second, Command: command})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stackwalker.Run(context.Background(), "dump"); !errors.Is(err, ErrStackwalkTimeout) {
		t.Fatalf("timeout error=%v", err)
	}
	want := errors.New("stackwalk exited")
	command.err = want
	if _, err := stackwalker.Run(context.Background(), "dump"); !errors.Is(err, want) {
		t.Fatalf("command error=%v", err)
	}
}

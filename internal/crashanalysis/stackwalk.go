package crashanalysis

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const DefaultMaxStackwalkOutputBytes int64 = 8 << 20

var (
	ErrStackwalkOutputTooLarge = errors.New("stackwalk output exceeds size limit")
	ErrStackwalkTimeout        = errors.New("stackwalk timed out")
)

type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type execCommandRunner struct {
	limit int64
}

func (r execCommandRunner) Run(ctx context.Context, path string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, path, args...)
	output := limitedOutput{limit: r.limit}
	command.Stdout = &output
	command.Stderr = &output
	err := command.Run()
	if output.exceeded {
		return output.buffer.Bytes(), ErrStackwalkOutputTooLarge
	}
	return output.buffer.Bytes(), err
}

type limitedOutput struct {
	buffer   bytes.Buffer
	exceeded bool
	limit    int64
}

func (o *limitedOutput) Write(value []byte) (int, error) {
	if o.limit > 0 && int64(o.buffer.Len()+len(value)) > o.limit {
		remaining := o.limit - int64(o.buffer.Len())
		if remaining > 0 {
			_, _ = o.buffer.Write(value[:remaining])
		}
		o.exceeded = true
		return len(value), ErrStackwalkOutputTooLarge
	}
	return o.buffer.Write(value)
}

type StackwalkConfig struct {
	Path           string
	SymbolRoot     string
	Timeout        time.Duration
	MaxOutputBytes int64
	Command        CommandRunner
	ToolVersion    string
}

type Stackwalker struct {
	Path           string
	SymbolRoot     string
	Timeout        time.Duration
	MaxOutputBytes int64
	Command        CommandRunner
	ToolVersion    string
}

type StackwalkResult struct {
	Text string
	Tool string
}

func NewStackwalker(config StackwalkConfig) (*Stackwalker, error) {
	if strings.TrimSpace(config.Path) == "" || strings.TrimSpace(config.SymbolRoot) == "" {
		return nil, errors.New("stackwalk path and symbol root are required")
	}
	if config.Timeout <= 0 {
		config.Timeout = 2 * time.Minute
	}
	if config.MaxOutputBytes <= 0 {
		config.MaxOutputBytes = DefaultMaxStackwalkOutputBytes
	}
	if config.Command == nil {
		config.Command = execCommandRunner{limit: config.MaxOutputBytes}
	}
	if strings.TrimSpace(config.ToolVersion) == "" {
		config.ToolVersion = filepath.Base(config.Path)
	}
	return &Stackwalker{
		Path: config.Path, SymbolRoot: config.SymbolRoot, Timeout: config.Timeout,
		MaxOutputBytes: config.MaxOutputBytes, Command: config.Command, ToolVersion: config.ToolVersion,
	}, nil
}

func (s *Stackwalker) Run(ctx context.Context, dumpPath string) (StackwalkResult, error) {
	if s == nil || s.Command == nil || strings.TrimSpace(dumpPath) == "" {
		return StackwalkResult{}, errors.New("stackwalk is not configured")
	}
	if err := ctx.Err(); err != nil {
		return StackwalkResult{}, err
	}
	commandContext, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	output, err := s.Command.Run(commandContext, s.Path, dumpPath, s.SymbolRoot)
	if int64(len(output)) > s.MaxOutputBytes {
		return StackwalkResult{}, ErrStackwalkOutputTooLarge
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(commandContext.Err(), context.DeadlineExceeded) {
		return StackwalkResult{}, ErrStackwalkTimeout
	}
	if err != nil {
		return StackwalkResult{}, fmt.Errorf("run stackwalk: %w", err)
	}
	return StackwalkResult{Text: string(output), Tool: s.ToolVersion}, nil
}

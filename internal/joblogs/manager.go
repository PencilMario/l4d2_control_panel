package joblogs

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const defaultTerminalLimit int64 = 10 << 20

var validJobID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

type Level string

const (
	Output Level = "output"
	Info   Level = "info"
	Warn   Level = "warn"
	Error  Level = "error"
)

type Record struct {
	Seq       uint64    `json:"seq"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"`
	Level     Level     `json:"level"`
	Message   string    `json:"message"`
}

type Query struct {
	AfterSeq  uint64
	BeforeSeq uint64
	Limit     int
	Sources   map[string]bool
	Levels    map[Level]bool
}

type Page struct {
	Records   []Record `json:"records"`
	NextSeq   uint64   `json:"next_seq"`
	Truncated bool     `json:"truncated"`
}

type Options struct {
	TerminalLimit int64
	Redactor      Redactor
	SubscriberCap int
}

type taskState struct {
	mu          sync.Mutex
	nextSeq     uint64
	truncated   bool
	subscribers map[chan Record]struct{}
}

type Manager struct {
	root          string
	terminalLimit int64
	redactor      Redactor
	subscriberCap int
	mu            sync.Mutex
	tasks         map[string]*taskState
}

func Open(root string, options Options) (*Manager, error) {
	if root == "" {
		return nil, errors.New("job log root is required")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, err
	}
	if options.TerminalLimit <= 0 {
		options.TerminalLimit = defaultTerminalLimit
	}
	if options.SubscriberCap <= 0 {
		options.SubscriberCap = 256
	}
	return &Manager{root: root, terminalLimit: options.TerminalLimit, redactor: options.Redactor, subscriberCap: options.SubscriberCap, tasks: map[string]*taskState{}}, nil
}

func (m *Manager) Close() error { return nil }

func (m *Manager) Append(ctx context.Context, jobID, source string, level Level, message string) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	state, err := m.state(jobID)
	if err != nil {
		return Record{}, err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.nextSeq == 0 {
		if err := m.loadStateLocked(jobID, state); err != nil {
			return Record{}, err
		}
	}
	record := Record{Seq: state.nextSeq + 1, Timestamp: time.Now().UTC(), Source: normalizeSource(source), Level: normalizeLevel(level), Message: m.redactor.Redact(message)}
	raw, err := json.Marshal(record)
	if err != nil {
		return Record{}, err
	}
	file, err := os.OpenFile(m.path(jobID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return Record{}, err
	}
	_, writeErr := file.Write(append(raw, '\n'))
	closeErr := file.Close()
	if writeErr != nil {
		return Record{}, writeErr
	}
	if closeErr != nil {
		return Record{}, closeErr
	}
	state.nextSeq = record.Seq
	for subscriber := range state.subscribers {
		select {
		case subscriber <- record:
		default:
			close(subscriber)
			delete(state.subscribers, subscriber)
		}
	}
	return record, nil
}

func (m *Manager) Read(ctx context.Context, jobID string, query Query) (Page, error) {
	state, err := m.state(jobID)
	if err != nil {
		return Page{}, err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Page{}, err
	}
	records, truncated, err := m.readAll(jobID)
	if err != nil {
		return Page{}, err
	}
	filtered := make([]Record, 0, len(records))
	for _, record := range records {
		if query.AfterSeq > 0 && record.Seq <= query.AfterSeq {
			continue
		}
		if query.BeforeSeq > 0 && record.Seq >= query.BeforeSeq {
			continue
		}
		if len(query.Sources) > 0 && !query.Sources[record.Source] {
			continue
		}
		if len(query.Levels) > 0 && !query.Levels[record.Level] {
			continue
		}
		filtered = append(filtered, record)
	}
	limit := query.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	if len(filtered) > limit {
		if query.BeforeSeq > 0 || query.AfterSeq == 0 {
			filtered = filtered[len(filtered)-limit:]
		} else {
			filtered = filtered[:limit]
		}
	}
	next := uint64(0)
	if len(filtered) > 0 {
		next = filtered[len(filtered)-1].Seq
	}
	return Page{Records: filtered, NextSeq: next, Truncated: truncated || state.truncated}, nil
}

func (m *Manager) Subscribe(ctx context.Context, jobID string, afterSeq uint64) ([]Record, <-chan Record, func(), error) {
	state, err := m.state(jobID)
	if err != nil {
		return nil, nil, nil, err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}
	records, _, err := m.readAll(jobID)
	if err != nil {
		return nil, nil, nil, err
	}
	replay := make([]Record, 0, len(records))
	for _, record := range records {
		if record.Seq > afterSeq {
			replay = append(replay, record)
		}
	}
	stream := make(chan Record, m.subscriberCap)
	state.subscribers[stream] = struct{}{}
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			state.mu.Lock()
			if _, ok := state.subscribers[stream]; ok {
				delete(state.subscribers, stream)
				close(stream)
			}
			state.mu.Unlock()
		})
	}
	return replay, stream, cancel, nil
}

func (m *Manager) Finalize(ctx context.Context, jobID string) error {
	state, err := m.state(jobID)
	if err != nil {
		return err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := os.ReadFile(m.path(jobID))
	if errors.Is(err, os.ErrNotExist) || int64(len(raw)) <= m.terminalLimit {
		return nil
	}
	if err != nil {
		return err
	}
	lines := splitJSONLines(raw)
	kept := make([][]byte, 0, len(lines))
	size := int64(0)
	for index := len(lines) - 1; index >= 0; index-- {
		lineSize := int64(len(lines[index]) + 1)
		if size+lineSize > m.terminalLimit/2 {
			break
		}
		kept = append(kept, lines[index])
		size += lineSize
	}
	for left, right := 0, len(kept)-1; left < right; left, right = left+1, right-1 {
		kept[left], kept[right] = kept[right], kept[left]
	}
	markerSeq := uint64(1)
	if len(kept) > 0 {
		var first Record
		if json.Unmarshal(kept[0], &first) == nil && first.Seq > 1 {
			markerSeq = first.Seq - 1
		}
	}
	marker, _ := json.Marshal(Record{Seq: markerSeq, Timestamp: time.Now().UTC(), Source: "task", Level: Warn, Message: "earlier task log records were truncated"})
	temporary := m.path(jobID) + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	write := func(line []byte) error {
		_, writeErr := file.Write(append(line, '\n'))
		return writeErr
	}
	if err = write(marker); err == nil {
		for _, line := range kept {
			if err = write(line); err != nil {
				break
			}
		}
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err := os.Rename(temporary, m.path(jobID)); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	state.truncated = true
	return nil
}

func (m *Manager) Delete(ctx context.Context, jobID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validJobID.MatchString(jobID) {
		return errors.New("invalid job id")
	}
	m.mu.Lock()
	state := m.tasks[jobID]
	delete(m.tasks, jobID)
	m.mu.Unlock()
	if state != nil {
		state.mu.Lock()
		for subscriber := range state.subscribers {
			close(subscriber)
		}
		state.subscribers = nil
		state.mu.Unlock()
	}
	err := os.Remove(m.path(jobID))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (m *Manager) CleanupOrphans(ctx context.Context, keep map[string]bool) error {
	entries, err := os.ReadDir(m.root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		jobID := strings.TrimSuffix(entry.Name(), ".jsonl")
		if !keep[jobID] {
			if err := m.Delete(ctx, jobID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) Snapshot(ctx context.Context, jobID string, output io.Writer) error {
	state, err := m.state(jobID)
	if err != nil {
		return err
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.Open(m.path(jobID))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(output, file)
	return err
}

func (m *Manager) state(jobID string) (*taskState, error) {
	if !validJobID.MatchString(jobID) {
		return nil, errors.New("invalid job id")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.tasks[jobID]
	if state == nil {
		state = &taskState{subscribers: map[chan Record]struct{}{}}
		m.tasks[jobID] = state
	}
	return state, nil
}

func (m *Manager) loadStateLocked(jobID string, state *taskState) error {
	records, truncated, err := m.readAll(jobID)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.Seq > state.nextSeq {
			state.nextSeq = record.Seq
		}
	}
	state.truncated = truncated
	return nil
}

func (m *Manager) readAll(jobID string) ([]Record, bool, error) {
	file, err := os.Open(m.path(jobID))
	if errors.Is(err, os.ErrNotExist) {
		return []Record{}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 2<<20)
	records := []Record{}
	truncated := false
	for scanner.Scan() {
		var record Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, false, fmt.Errorf("decode job log: %w", err)
		}
		if record.Source == "task" && record.Level == Warn && strings.Contains(record.Message, "were truncated") {
			truncated = true
		}
		records = append(records, record)
	}
	return records, truncated, scanner.Err()
}

func (m *Manager) path(jobID string) string {
	return filepath.Join(m.root, jobID+".jsonl")
}

func normalizeSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "task"
	}
	return source
}

func normalizeLevel(level Level) Level {
	switch level {
	case Output, Info, Warn, Error:
		return level
	default:
		return Info
	}
}

func splitJSONLines(raw []byte) [][]byte {
	parts := strings.Split(strings.TrimSpace(string(raw)), "\n")
	result := make([][]byte, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			result = append(result, []byte(part))
		}
	}
	return result
}

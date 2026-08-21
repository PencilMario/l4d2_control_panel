package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
)

const (
	maxConsoleHistoryBytes = 1 << 20
	maxConsoleHistoryLines = 1000
	maxConsolePendingBytes = 4 << 20
)

var errConsoleSessionClosed = errors.New("console session closed")

type consoleHub struct {
	attacher ConsoleAttacher

	mu       sync.Mutex
	sessions map[string]*consoleSession
}

type consoleSession struct {
	instanceID  string
	containerID string
	ready       chan struct{}

	writeMu sync.Mutex
	stream  io.ReadWriteCloser

	history     []byte
	subscribers map[*consoleSubscriber]struct{}
	attachErr   error
	closed      bool
}

// normalizeConsolePayload removes blank refresh lines emitted by a TTY while
// preserving useful output, including lines split across read boundaries.
func normalizeConsolePayload(payload []byte) []byte {
	var output []byte
	start := 0
	for {
		newline := bytes.IndexByte(payload[start:], '\n')
		if newline < 0 {
			line := payload[start:]
			line = bytes.TrimRight(line, "\r")
			if len(bytes.TrimSpace(line)) > 0 {
				output = append(output, line...)
			}
			break
		}
		newline += start
		line := payload[start:newline]
		line = bytes.TrimRight(line, "\r")
		if len(bytes.TrimSpace(line)) > 0 {
			output = append(output, line...)
			output = append(output, '\n')
		}
		start = newline + 1
		if start == len(payload) {
			break
		}
	}
	return output
}

// consoleSubscriber queues output independently of the WebSocket writer. A
// browser can spend time receiving the initial history while the game keeps
// producing output; that burst must not disconnect the subscriber.
type consoleSubscriber struct {
	mu          sync.Mutex
	cond        *sync.Cond
	queue       [][]byte
	queuedBytes int
	closed      bool
}

func newConsoleSubscriber() *consoleSubscriber {
	subscriber := &consoleSubscriber{}
	subscriber.cond = sync.NewCond(&subscriber.mu)
	return subscriber
}

func (s *consoleSubscriber) enqueue(payload []byte) {
	frame := append([]byte(nil), payload...)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.queue = append(s.queue, frame)
	s.queuedBytes += len(frame)
	for s.queuedBytes > maxConsolePendingBytes && len(s.queue) > 1 {
		s.queuedBytes -= len(s.queue[0])
		s.queue[0] = nil
		s.queue = s.queue[1:]
	}
	s.cond.Signal()
}

func (s *consoleSubscriber) next() ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.queue) == 0 && !s.closed {
		s.cond.Wait()
	}
	if len(s.queue) == 0 {
		return nil, false
	}
	frame := s.queue[0]
	s.queue[0] = nil
	s.queue = s.queue[1:]
	s.queuedBytes -= len(frame)
	return frame, true
}

func (s *consoleSubscriber) close() {
	s.mu.Lock()
	s.closed = true
	s.queue = nil
	s.queuedBytes = 0
	s.cond.Broadcast()
	s.mu.Unlock()
}

func newConsoleHub(attacher ConsoleAttacher) *consoleHub {
	return &consoleHub{attacher: attacher, sessions: make(map[string]*consoleSession)}
}

func (h *consoleHub) Subscribe(instanceID, containerID string) (*consoleSession, []byte, *consoleSubscriber, func(), error) {
	for {
		h.mu.Lock()
		session := h.sessions[instanceID]
		if session != nil && session.containerID != containerID {
			delete(h.sessions, instanceID)
			stream := h.closeLocked(session)
			h.mu.Unlock()
			if stream != nil {
				_ = stream.Close()
			}
			continue
		}
		created := session == nil
		if created {
			session = &consoleSession{
				instanceID:  instanceID,
				containerID: containerID,
				ready:       make(chan struct{}),
				subscribers: make(map[*consoleSubscriber]struct{}),
			}
			h.sessions[instanceID] = session
		}
		h.mu.Unlock()

		if created {
			stream, err := h.attacher.AttachSupervisor(context.Background(), containerID)
			h.start(session, stream, err)
		}

		<-session.ready
		h.mu.Lock()
		if session.attachErr != nil {
			err := session.attachErr
			h.mu.Unlock()
			return nil, nil, nil, nil, err
		}
		if session.closed || h.sessions[instanceID] != session {
			h.mu.Unlock()
			continue
		}
		updates := newConsoleSubscriber()
		session.subscribers[updates] = struct{}{}
		history := append([]byte(nil), session.history...)
		h.mu.Unlock()

		var once sync.Once
		return session, history, updates, func() {
			once.Do(func() { h.unsubscribe(session, updates) })
		}, nil
	}
}

func (h *consoleHub) start(session *consoleSession, stream io.ReadWriteCloser, attachErr error) {
	h.mu.Lock()
	if session.closed || h.sessions[session.instanceID] != session {
		h.mu.Unlock()
		if stream != nil {
			_ = stream.Close()
		}
		return
	}
	if attachErr != nil {
		delete(h.sessions, session.instanceID)
		session.attachErr = attachErr
		session.closed = true
		close(session.ready)
		h.mu.Unlock()
		return
	}
	session.stream = stream
	close(session.ready)
	h.mu.Unlock()

	go h.read(session, stream)
}

func (h *consoleHub) read(session *consoleSession, stream io.ReadWriteCloser) {
	buffer := make([]byte, 16*1024)
	for {
		n, err := stream.Read(buffer)
		if n > 0 {
			if payload := normalizeConsolePayload(buffer[:n]); len(payload) > 0 {
				h.publish(session, payload)
			}
		}
		if err != nil {
			h.retire(session)
			return
		}
	}
}

func (h *consoleHub) publish(session *consoleSession, payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if session.closed || h.sessions[session.instanceID] != session {
		return
	}
	session.history = appendConsoleHistory(session.history, payload)
	for updates := range session.subscribers {
		updates.enqueue(payload)
	}
}

func (h *consoleHub) Write(session *consoleSession, payload []byte) error {
	h.mu.Lock()
	if session.closed || h.sessions[session.instanceID] != session || session.stream == nil {
		h.mu.Unlock()
		return errConsoleSessionClosed
	}
	session.writeMu.Lock()
	stream := session.stream
	h.mu.Unlock()
	defer session.writeMu.Unlock()
	_, err := stream.Write(payload)
	return err
}

func (h *consoleHub) unsubscribe(session *consoleSession, updates *consoleSubscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, found := session.subscribers[updates]; found {
		delete(session.subscribers, updates)
		updates.close()
	}
}

func (h *consoleHub) retire(session *consoleSession) {
	h.mu.Lock()
	if h.sessions[session.instanceID] == session {
		delete(h.sessions, session.instanceID)
	}
	stream := h.closeLocked(session)
	h.mu.Unlock()
	if stream != nil {
		_ = stream.Close()
	}
}

func (h *consoleHub) closeLocked(session *consoleSession) io.ReadWriteCloser {
	if session.closed {
		return nil
	}
	session.closed = true
	if session.attachErr == nil {
		session.attachErr = errConsoleSessionClosed
	}
	select {
	case <-session.ready:
	default:
		close(session.ready)
	}
	for updates := range session.subscribers {
		updates.close()
	}
	clear(session.subscribers)
	session.writeMu.Lock()
	stream := session.stream
	session.stream = nil
	session.writeMu.Unlock()
	return stream
}

func appendConsoleHistory(history, payload []byte) []byte {
	combined := append(append([]byte(nil), history...), payload...)
	if len(combined) > maxConsoleHistoryBytes {
		combined = append([]byte(nil), combined[len(combined)-maxConsoleHistoryBytes:]...)
	}
	lines := 0
	for _, value := range combined {
		if value == '\n' {
			lines++
		}
	}
	if len(combined) > 0 && combined[len(combined)-1] != '\n' {
		lines++
	}
	if lines <= maxConsoleHistoryLines {
		return combined
	}
	linesToDrop := lines - maxConsoleHistoryLines
	for index, value := range combined {
		if value != '\n' {
			continue
		}
		linesToDrop--
		if linesToDrop == 0 {
			return append([]byte(nil), combined[index+1:]...)
		}
	}
	return combined
}

package httpapi

import (
	"context"
	"errors"
	"io"
	"sync"
)

const (
	maxConsoleHistoryBytes  = 1 << 20
	maxConsoleHistoryLines  = 1000
	consoleSubscriberBuffer = 64
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
	subscribers map[chan []byte]struct{}
	attachErr   error
	closed      bool
}

func newConsoleHub(attacher ConsoleAttacher) *consoleHub {
	return &consoleHub{attacher: attacher, sessions: make(map[string]*consoleSession)}
}

func (h *consoleHub) Subscribe(instanceID, containerID string) (*consoleSession, []byte, <-chan []byte, func(), error) {
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
				subscribers: make(map[chan []byte]struct{}),
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
		updates := make(chan []byte, consoleSubscriberBuffer)
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
			h.publish(session, buffer[:n])
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
		frame := append([]byte(nil), payload...)
		select {
		case updates <- frame:
		default:
			delete(session.subscribers, updates)
			close(updates)
		}
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

func (h *consoleHub) unsubscribe(session *consoleSession, updates chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, found := session.subscribers[updates]; found {
		delete(session.subscribers, updates)
		close(updates)
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
		close(updates)
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

package traffic

import (
	"errors"
	"fmt"
	"regexp"
	"sync"
)

var safeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

var (
	ErrNotFound    = errors.New("traffic session not found")
	ErrRunMismatch = errors.New("traffic session run ID mismatch")
)

type counterSession struct {
	runID  string
	ports  map[uint16]struct{}
	totals Totals
	active bool
}

type Counter struct {
	mu       sync.RWMutex
	sessions map[string]*counterSession
}

func NewCounter() *Counter {
	return &Counter{sessions: make(map[string]*counterSession)}
}

func (c *Counter) Register(session Session) error {
	if !safeID.MatchString(session.InstanceID) || !safeID.MatchString(session.RunID) {
		return errors.New("instance_id and run_id must be safe nonempty identifiers")
	}
	if len(session.Ports) == 0 {
		return errors.New("at least one port is required")
	}
	ports := make(map[uint16]struct{}, len(session.Ports))
	for _, port := range session.Ports {
		if port < 1 || port > 65535 {
			return fmt.Errorf("invalid port %d", port)
		}
		ports[uint16(port)] = struct{}{}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	current, ok := c.sessions[session.InstanceID]
	if ok && current.runID == session.RunID {
		current.ports = ports
		return nil
	}
	c.sessions[session.InstanceID] = &counterSession{
		runID:  session.RunID,
		ports:  ports,
		totals: Totals{RunID: session.RunID},
		active: true,
	}
	return nil
}

func (c *Counter) Stop(instanceID, runID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	session, ok := c.sessions[instanceID]
	if !ok {
		return ErrNotFound
	}
	if session.runID != runID {
		return ErrRunMismatch
	}
	session.active = false
	return nil
}

func (c *Counter) Observe(packet Packet) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, session := range c.sessions {
		if !session.active {
			continue
		}
		if _, ok := session.ports[packet.DstPort]; ok {
			session.totals.RXBytes += packet.Length
		}
		if _, ok := session.ports[packet.SrcPort]; ok {
			session.totals.TXBytes += packet.Length
		}
	}
}

func (c *Counter) Totals(instanceID string) (Totals, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	session, ok := c.sessions[instanceID]
	if !ok {
		return Totals{}, false
	}
	return session.totals, true
}

package a2sdefense

import (
	"errors"
	"net/netip"
	"sync"
	"time"
)

const DefaultEventCapacity = 256

type QueryType string

const (
	QueryInfo      QueryType = "A2S_INFO"
	QueryPlayer    QueryType = "A2S_PLAYER"
	QueryRules     QueryType = "A2S_RULES"
	QueryChallenge QueryType = "CHALLENGE"
	QueryOther69   QueryType = "OTHER_69"
)

type Event struct {
	Sequence        uint64     `json:"sequence"`
	Timestamp       time.Time  `json:"timestamp"`
	Source          netip.Addr `json:"source"`
	DestinationPort int        `json:"destination_port"`
	Query           QueryType  `json:"query"`
	Action          string     `json:"action"`
}

type EventBatch struct {
	BootID         string  `json:"boot_id"`
	OldestSequence uint64  `json:"oldest_sequence"`
	LatestSequence uint64  `json:"latest_sequence"`
	Lost           uint64  `json:"lost"`
	Restarted      bool    `json:"restarted"`
	Events         []Event `json:"events"`
}

type EventRing struct {
	mu       sync.RWMutex
	bootID   string
	capacity int
	next     uint64
	events   []Event
}

func NewEventRing(bootID string, capacity int) *EventRing {
	if capacity < 1 {
		capacity = DefaultEventCapacity
	}
	return &EventRing{bootID: bootID, capacity: capacity, next: 1, events: make([]Event, 0, capacity)}
}

func (r *EventRing) Add(event Event) error {
	if !event.Source.IsValid() || !event.Source.Is4() || event.DestinationPort < 1 || event.DestinationPort > 65535 || !event.Query.valid() {
		return errors.New("invalid A2S defense event")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	event.Sequence = r.next
	event.Action = "DROP"
	r.next++
	if len(r.events) == r.capacity {
		copy(r.events, r.events[1:])
		r.events[len(r.events)-1] = event
		return nil
	}
	r.events = append(r.events, event)
	return nil
}

func (r *EventRing) Batch(bootID string, after uint64) EventBatch {
	r.mu.RLock()
	defer r.mu.RUnlock()
	batch := EventBatch{BootID: r.bootID, Restarted: bootID != "" && bootID != r.bootID, Events: make([]Event, 0, len(r.events))}
	if len(r.events) == 0 {
		return batch
	}
	batch.OldestSequence = r.events[0].Sequence
	batch.LatestSequence = r.events[len(r.events)-1].Sequence
	if batch.Restarted {
		after = 0
	} else if after+1 < batch.OldestSequence {
		batch.Lost = batch.OldestSequence - after - 1
	}
	for _, event := range r.events {
		if event.Sequence > after {
			batch.Events = append(batch.Events, event)
		}
	}
	return batch
}

func (q QueryType) valid() bool {
	switch q {
	case QueryInfo, QueryPlayer, QueryRules, QueryChallenge, QueryOther69:
		return true
	default:
		return false
	}
}

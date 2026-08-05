//go:build linux

package a2sdefense

import (
	"context"
	"time"

	nflog "github.com/florianl/go-nflog/v2"
)

const NFLogGroup = 100

type NFLogSource struct {
	ring    *EventRing
	now     func() time.Time
	samples *SampleWindow
}

func NewNFLogSource(ring *EventRing, now func() time.Time) *NFLogSource {
	if now == nil {
		now = time.Now
	}
	return &NFLogSource{ring: ring, now: now, samples: NewSampleWindow(time.Minute)}
}

func (s *NFLogSource) Run(ctx context.Context) error {
	connection, err := nflog.Open(&nflog.Config{Group: NFLogGroup, Copymode: nflog.CopyPacket, Bufsize: 128, QThresh: 1, Timeout: 10})
	if err != nil {
		return err
	}
	defer connection.Close()
	if err := connection.RegisterWithErrorFunc(ctx, func(attribute nflog.Attribute) int {
		if attribute.Payload == nil {
			return 0
		}
		timestamp := s.now()
		if attribute.Timestamp != nil {
			timestamp = *attribute.Timestamp
		}
		event, parseErr := ParseNFLogPacket(*attribute.Payload, timestamp)
		if parseErr == nil {
			event.SampledDrops60s = s.samples.Observe(event.Source, event.Timestamp)
			_ = s.ring.Add(event)
		}
		return 0
	}, func(error) int { return 0 }); err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}

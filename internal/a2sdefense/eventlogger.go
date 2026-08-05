package a2sdefense

import (
	"context"
	"errors"
	"time"

	"github.com/not0721here/l4d2-control-panel/internal/domain"
)

type EventClient interface {
	Events(context.Context, string, uint64) (EventBatch, error)
}

type EventInstanceStore interface {
	Instances(context.Context) ([]domain.Instance, error)
}

type EventLogSink interface {
	AppendA2SDefense(context.Context, string, Event) error
	AppendA2SDefenseLoss(context.Context, string, uint64) error
}

type EventLogger struct {
	client EventClient
	store  EventInstanceStore
	sink   EventLogSink
	delay  time.Duration
	report func(error)
	bootID string
	after  uint64
}

func NewEventLogger(client EventClient, store EventInstanceStore, sink EventLogSink, delay time.Duration, report func(error)) *EventLogger {
	if delay <= 0 {
		delay = time.Second
	}
	return &EventLogger{client: client, store: store, sink: sink, delay: delay, report: report}
}

func (l *EventLogger) Poll(ctx context.Context) error {
	batch, err := l.client.Events(ctx, l.bootID, l.after)
	if err != nil {
		return err
	}
	instances, err := l.store.Instances(ctx)
	if err != nil {
		return err
	}
	var failures []error
	if batch.Restarted || batch.Lost > 0 {
		for _, instance := range instances {
			if instance.ActualState == domain.StateRunning {
				if err := l.sink.AppendA2SDefenseLoss(ctx, instance.ID, batch.Lost); err != nil {
					failures = append(failures, err)
				}
			}
		}
	}
	for _, event := range batch.Events {
		for _, instance := range instances {
			if instance.ActualState != domain.StateRunning || (instance.GamePort != event.DestinationPort && (instance.SourceTVPort == 0 || instance.SourceTVPort != event.DestinationPort)) {
				continue
			}
			if err := l.sink.AppendA2SDefense(ctx, instance.ID, event); err != nil {
				failures = append(failures, err)
			}
		}
		if event.Sequence > l.after {
			l.after = event.Sequence
		}
	}
	l.bootID = batch.BootID
	if batch.LatestSequence > l.after {
		l.after = batch.LatestSequence
	}
	return errors.Join(failures...)
}

func (l *EventLogger) Run(ctx context.Context) {
	for ctx.Err() == nil {
		if err := l.Poll(ctx); err != nil && ctx.Err() == nil && l.report != nil {
			l.report(err)
		}
		timer := time.NewTimer(l.delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

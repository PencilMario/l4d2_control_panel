package health

import (
	"context"
	"errors"
	"github.com/not0721here/l4d2-control-panel/internal/a2s"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"net"
	"strconv"
	"time"
)

type Query interface {
	Info(string) (a2s.Info, error)
}
type ContainerProbe interface {
	Running(context.Context, string) (bool, error)
}
type Checker struct {
	Host              string
	Query             Query
	Timeout, Interval time.Duration
	Probe             ContainerProbe
}

func (c Checker) Wait(ctx context.Context, instance domain.Instance) error {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 30 * time.Minute
	}
	interval := c.Interval
	if interval == 0 {
		interval = 2 * time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	address := net.JoinHostPort(c.Host, strconv.Itoa(instance.GamePort))
	var last error
	for {
		if c.Probe != nil && instance.ContainerID != "" {
			running, probeErr := c.Probe.Running(ctx, instance.ContainerID)
			if probeErr == nil && !running {
				return errors.New("managed container exited before A2S became healthy")
			}
		}
		if _, err := c.Query.Info(address); err == nil {
			return nil
		} else {
			last = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("A2S health timeout: " + last.Error())
		case <-ticker.C:
		}
	}
}

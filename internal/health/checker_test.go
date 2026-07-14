package health

import (
	"context"
	"errors"
	"github.com/not0721here/l4d2-control-panel/internal/a2s"
	"github.com/not0721here/l4d2-control-panel/internal/domain"
	"testing"
	"time"
)

type eventualQuery struct{ calls int }

func (q *eventualQuery) Info(string) (a2s.Info, error) {
	q.calls++
	if q.calls < 3 {
		return a2s.Info{}, errors.New("not ready")
	}
	return a2s.Info{Name: "ready"}, nil
}
func TestWaitPollsUntilA2SResponds(t *testing.T) {
	query := &eventualQuery{}
	checker := Checker{Host: "127.0.0.1", Query: query, Timeout: time.Second, Interval: time.Millisecond}
	if err := checker.Wait(context.Background(), domain.Instance{GamePort: 27015}); err != nil {
		t.Fatal(err)
	}
	if query.calls != 3 {
		t.Fatalf("calls=%d", query.calls)
	}
}

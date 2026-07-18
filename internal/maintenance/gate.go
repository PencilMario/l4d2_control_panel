package maintenance

import (
	"context"
	"sync"
)

const exclusiveWeight int64 = 1 << 30

type weightedGate struct {
	mu      sync.Mutex
	cond    *sync.Cond
	active  int64
	writers int
}

type Gate struct {
	state *weightedGate
}

type leaseContextKey struct{}

func NewGate() *Gate {
	state := &weightedGate{}
	state.cond = sync.NewCond(&state.mu)
	return &Gate{state: state}
}

func (g *Gate) Shared(ctx context.Context) (func(), error) {
	_, release, err := g.SharedContext(ctx)
	return release, err
}

func (g *Gate) Exclusive(ctx context.Context) (func(), error) {
	_, release, err := g.ExclusiveContext(ctx)
	return release, err
}

func (g *Gate) SharedContext(ctx context.Context) (context.Context, func(), error) {
	if hasLease(ctx) {
		return ctx, func() {}, nil
	}
	release, err := g.acquire(ctx, 1, false)
	if err != nil {
		return ctx, nil, err
	}
	return context.WithValue(ctx, leaseContextKey{}, true), release, nil
}

func (g *Gate) ExclusiveContext(ctx context.Context) (context.Context, func(), error) {
	if hasLease(ctx) {
		return ctx, func() {}, nil
	}
	release, err := g.acquire(ctx, exclusiveWeight, true)
	if err != nil {
		return ctx, nil, err
	}
	return context.WithValue(ctx, leaseContextKey{}, true), release, nil
}

func hasLease(ctx context.Context) bool {
	value, _ := ctx.Value(leaseContextKey{}).(bool)
	return value
}

func (g *Gate) acquire(ctx context.Context, weight int64, writer bool) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	state := g.state
	state.mu.Lock()
	if writer {
		state.writers++
	}
	stop := context.AfterFunc(ctx, func() {
		state.mu.Lock()
		state.cond.Broadcast()
		state.mu.Unlock()
	})
	defer stop()
	for state.active+weight > exclusiveWeight || !writer && state.writers > 0 {
		if err := ctx.Err(); err != nil {
			if writer {
				state.writers--
				state.cond.Broadcast()
			}
			state.mu.Unlock()
			return nil, err
		}
		state.cond.Wait()
	}
	if writer {
		state.writers--
	}
	state.active += weight
	state.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			state.mu.Lock()
			state.active -= weight
			state.cond.Broadcast()
			state.mu.Unlock()
		})
	}, nil
}

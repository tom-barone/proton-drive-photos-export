package core

import (
	"context"
	"sync"
)

// gate pauses the download feed. A closed channel means running.
type gate struct {
	mu   sync.Mutex
	open chan struct{}
}

func newGate() *gate {
	g := &gate{open: make(chan struct{})}
	close(g.open)
	return g
}

func (g *gate) pause() {
	g.mu.Lock()
	defer g.mu.Unlock()
	select {
	case <-g.open:
		g.open = make(chan struct{})
	default:
	}
}

func (g *gate) resume() {
	g.mu.Lock()
	defer g.mu.Unlock()
	select {
	case <-g.open:
	default:
		close(g.open)
	}
}

func (g *gate) wait(ctx context.Context) error {
	g.mu.Lock()
	open := g.open
	g.mu.Unlock()
	select {
	case <-open:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

package core

import (
	"context"
	"testing"
	"time"
)

// Given a paused gate with a waiter blocked on it
// When resume is called
// Then the waiter is released
func TestGateResumeReleasesWaiter(t *testing.T) {
	g := newGate()
	g.pause()
	released := make(chan error, 1)
	go func() { released <- g.wait(context.Background()) }()

	select {
	case <-released:
		t.Fatal("wait returned while paused")
	case <-time.After(50 * time.Millisecond):
	}
	g.resume()
	if err := <-released; err != nil {
		t.Fatalf("wait returned %v, want nil", err)
	}
}

// Given a paused gate
// When the context is cancelled while waiting
// Then wait returns the context error
func TestGateWaitCancelled(t *testing.T) {
	g := newGate()
	g.pause()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := g.wait(ctx); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// Given counters for finished, skipped and failed items
// When a snapshot is taken
// Then Items returns their sum
func TestProgressItems(t *testing.T) {
	var pr progress
	pr.done.Add(2)
	pr.skipped.Add(3)
	pr.failed.Add(1)
	if got := pr.snapshot().Items(); got != 6 {
		t.Fatalf("got %d, want 6", got)
	}
}

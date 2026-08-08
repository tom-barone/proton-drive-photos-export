// AI GENERATED - Tom Barone
package main

import (
	"context"
	"testing"
	"time"
)

// Given the same link resolved twice in one run
// When photoPath is called both times
// Then it returns the same path, so re-checks stay deterministic
func TestPhotoPathDeterministic(t *testing.T) {
	claimed := map[string]string{}
	ts := time.Unix(1700000000, 0)
	a := photoPath("out", "IMG_1.jpg", ts, "link1", claimed)
	b := photoPath("out", "IMG_1.jpg", ts, "link1", claimed)
	if a != b {
		t.Fatalf("paths differ: %q vs %q", a, b)
	}
}

// Given two different links with the same name and timestamp
// When photoPath is called for both
// Then the second gets a distinct path containing its link ID
func TestPhotoPathCollision(t *testing.T) {
	claimed := map[string]string{}
	ts := time.Unix(1700000000, 0)
	a := photoPath("out", "IMG_1.jpg", ts, "link1xxxx", claimed)
	b := photoPath("out", "IMG_1.jpg", ts, "link2xxxx", claimed)
	if a == b {
		t.Fatalf("expected distinct paths, both %q", a)
	}
}

// Given a decrypted name containing a path separator
// When sanitizeName runs
// Then the separator is replaced
func TestSanitizeNameSlash(t *testing.T) {
	if got := sanitizeName("a/b.jpg"); got != "a_b.jpg" {
		t.Fatalf("got %q, want a_b.jpg", got)
	}
}

// Given an empty decrypted name
// When sanitizeName runs
// Then a placeholder name is returned
func TestSanitizeNameEmpty(t *testing.T) {
	if got := sanitizeName(""); got != "unnamed" {
		t.Fatalf("got %q, want unnamed", got)
	}
}

// Given a name containing every character Windows forbids
// When sanitizeName runs
// Then each forbidden character is replaced
func TestSanitizeNameWindowsChars(t *testing.T) {
	if got := sanitizeName(`a\b:c*d?e"f<g>h|i.jpg`); got != "a_b_c_d_e_f_g_h_i.jpg" {
		t.Fatalf("got %q, want a_b_c_d_e_f_g_h_i.jpg", got)
	}
}

// Given a name containing a control character
// When sanitizeName runs
// Then the control character is replaced
func TestSanitizeNameControlChar(t *testing.T) {
	if got := sanitizeName("a\x01b.jpg"); got != "a_b.jpg" {
		t.Fatalf("got %q, want a_b.jpg", got)
	}
}

// Given a name with a trailing dot, which Windows strips silently
// When sanitizeName runs
// Then the trailing dot is removed
func TestSanitizeNameTrailingDot(t *testing.T) {
	if got := sanitizeName("photo."); got != "photo" {
		t.Fatalf("got %q, want photo", got)
	}
}

// Given a Windows reserved device name as the file stem
// When sanitizeName runs
// Then the name is prefixed so Windows accepts it
func TestSanitizeNameReserved(t *testing.T) {
	if got := sanitizeName("CON.jpg"); got != "_CON.jpg" {
		t.Fatalf("got %q, want _CON.jpg", got)
	}
}

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
// When items is called
// Then it returns their sum
func TestProgressItems(t *testing.T) {
	var pr progress
	pr.done.Add(2)
	pr.skipped.Add(3)
	pr.failed.Add(1)
	if got := pr.items(); got != 6 {
		t.Fatalf("got %d, want 6", got)
	}
}

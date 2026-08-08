// AI GENERATED - Tom Barone
package main

import (
	"path/filepath"
	"testing"
)

// Given a saved session
// When it is loaded again
// Then the same tokens and key password come back
func TestSessionRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	want := session{UID: "uid", AccessToken: "acc", RefreshToken: "ref", SaltedKeyPass: []byte("pass")}
	saveSession(path, want)

	got, err := loadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.UID != want.UID || got.RefreshToken != want.RefreshToken || string(got.SaltedKeyPass) != "pass" {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// Given a session file missing the salted key password
// When it is loaded
// Then an error is returned so a fresh login happens
func TestLoadSessionIncomplete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	saveSession(path, session{UID: "uid", RefreshToken: "ref"})

	if _, err := loadSession(path); err == nil {
		t.Fatal("expected error, got nil")
	}
}

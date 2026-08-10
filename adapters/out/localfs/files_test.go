package localfs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Given a successful write
// When WriteFile runs
// Then the file holds the content with the given modification time
func TestWriteFileCommits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.jpg")
	ts := time.Unix(1700000000, 0)
	err := Files{}.WriteFile(path, ts, func(w io.Writer) error {
		_, err := w.Write([]byte("data"))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "data" {
		t.Fatalf("got (%q, %v), want (data, nil)", b, err)
	}
	fi, _ := os.Stat(path)
	if !fi.ModTime().Equal(ts) {
		t.Fatalf("got mod time %v, want %v", fi.ModTime(), ts)
	}
}

// Given a write that fails midway
// When WriteFile returns
// Then neither the file nor its .part remains
func TestWriteFileFailureLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.jpg")
	err := Files{}.WriteFile(path, time.Now(), func(w io.Writer) error {
		return errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("directory not empty: %v", entries)
	}
}

// Given an existing and a missing path
// When Exists is called for each
// Then only the existing path is reported
func TestExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if !(Files{}).Exists(path) {
		t.Fatal("existing file reported missing")
	}
	if (Files{}).Exists(path + "x") {
		t.Fatal("missing file reported existing")
	}
}

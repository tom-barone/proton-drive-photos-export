package core

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/tom-barone/proton-drive-photos-export/adapters/out/memory"
	"github.com/tom-barone/proton-drive-photos-export/core/ports/out"
)

var ctx = context.Background()

func stamp(ts time.Time) string { return ts.Format("20060102_150405") }

// scanned returns a service that has already scanned the fake drive.
func scanned(t *testing.T, d *memory.Drive, f *memory.Files) *Service {
	t.Helper()
	s := New(d, f)
	if _, _, err := s.Scan(ctx); err != nil {
		t.Fatal(err)
	}
	return s
}

// Given a drive that requires no extra sign-in steps
// When SignIn runs
// Then no extra steps are reported
func TestSignInNoExtraSteps(t *testing.T) {
	s := New(&memory.Drive{}, &memory.Files{})
	needTOTP, needMailbox, err := s.SignIn(ctx, "a@b.c", "pw")
	if err != nil || needTOTP || needMailbox {
		t.Fatalf("got (%v, %v, %v), want (false, false, nil)", needTOTP, needMailbox, err)
	}
}

// Given a drive that requires TOTP and a mailbox password
// When SignIn runs
// Then both extra steps are reported
func TestSignInReportsExtraSteps(t *testing.T) {
	s := New(&memory.Drive{NeedTOTP: true, NeedMailboxPass: true}, &memory.Files{})
	needTOTP, needMailbox, err := s.SignIn(ctx, "a@b.c", "pw")
	if err != nil || !needTOTP || !needMailbox {
		t.Fatalf("got (%v, %v, %v), want (true, true, nil)", needTOTP, needMailbox, err)
	}
}

// Given a drive that rejects the credentials
// When SignIn runs
// Then the error is passed through
func TestSignInError(t *testing.T) {
	boom := errors.New("bad credentials")
	s := New(&memory.Drive{SignInErr: boom}, &memory.Files{})
	if _, _, err := s.SignIn(ctx, "a@b.c", "pw"); !errors.Is(err, boom) {
		t.Fatalf("got %v, want %v", err, boom)
	}
}

// Given a TOTP code
// When SubmitTOTP runs
// Then the code reaches the drive
func TestSubmitTOTP(t *testing.T) {
	d := &memory.Drive{}
	s := New(d, &memory.Files{})
	if err := s.SubmitTOTP(ctx, "123456"); err != nil {
		t.Fatal(err)
	}
	if d.TOTP != "123456" {
		t.Fatalf("drive got code %q, want 123456", d.TOTP)
	}
}

// Given a single-password sign-in
// When FinishSignIn runs with no mailbox password
// Then the login password unlocks the account
func TestFinishSignInLoginPassword(t *testing.T) {
	d := &memory.Drive{}
	s := New(d, &memory.Files{})
	if _, _, err := s.SignIn(ctx, "a@b.c", "loginpw"); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishSignIn(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if string(d.KeyPass) != "loginpw" {
		t.Fatalf("drive got key pass %q, want loginpw", d.KeyPass)
	}
}

// Given a two-password sign-in
// When FinishSignIn runs with the mailbox password
// Then the mailbox password unlocks the account
func TestFinishSignInMailboxPassword(t *testing.T) {
	d := &memory.Drive{NeedMailboxPass: true}
	s := New(d, &memory.Files{})
	if _, _, err := s.SignIn(ctx, "a@b.c", "loginpw"); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishSignIn(ctx, "mailboxpw"); err != nil {
		t.Fatal(err)
	}
	if string(d.KeyPass) != "mailboxpw" {
		t.Fatalf("drive got key pass %q, want mailboxpw", d.KeyPass)
	}
}

// Given a FinishSignIn attempt that fails
// When FinishSignIn is retried after the failure
// Then the login password is still available to unlock the account
func TestFinishSignInRetryAfterError(t *testing.T) {
	d := &memory.Drive{FinishErr: errors.New("transient")}
	s := New(d, &memory.Files{})
	if _, _, err := s.SignIn(ctx, "a@b.c", "loginpw"); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishSignIn(ctx, ""); err == nil {
		t.Fatal("expected error, got nil")
	}
	d.FinishErr = nil
	if err := s.FinishSignIn(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if string(d.KeyPass) != "loginpw" {
		t.Fatalf("drive got key pass %q, want loginpw", d.KeyPass)
	}
}

// Given a photo library with two items
// When Scan runs
// Then the photo count and library size are returned
func TestScan(t *testing.T) {
	d := &memory.Drive{Jobs: []out.Job{{LinkID: "a"}, {LinkID: "b"}}, UsedBytes: 42}
	s := New(d, &memory.Files{})
	photos, usedBytes, err := s.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if photos != 2 || usedBytes != 42 {
		t.Fatalf("got (%d, %d), want (2, 42)", photos, usedBytes)
	}
}

// Given a drive that fails to list photos
// When Scan runs
// Then the error is passed through
func TestScanError(t *testing.T) {
	boom := errors.New("listing failed")
	s := New(&memory.Drive{ListErr: boom}, &memory.Files{})
	if _, _, err := s.Scan(ctx); !errors.Is(err, boom) {
		t.Fatalf("got %v, want %v", err, boom)
	}
}

// Given a scanned photo with a capture time
// When Download runs
// Then the photo is written under its timestamped name with its content
func TestDownloadWritesFile(t *testing.T) {
	ts := time.Unix(1700000000, 0)
	d := &memory.Drive{
		Jobs:    []out.Job{{LinkID: "l1", Taken: ts}},
		Metas:   map[string]out.FileMeta{"l1": {Name: "IMG_1.jpg", Active: true}},
		Content: map[string][]byte{"l1": []byte("data")},
	}
	f := &memory.Files{}
	s := scanned(t, d, f)
	if err := s.Download(ctx, "out"); err != nil {
		t.Fatal(err)
	}
	file, ok := f.Get(filepath.Join("out", stamp(ts)+"_IMG_1.jpg"))
	if !ok || string(file.Data) != "data" {
		t.Fatalf("got (%q, %v), want (data, true)", file.Data, ok)
	}
}

// Given a photo without a capture time
// When Download runs
// Then the file is timestamped with the drive's creation time
func TestDownloadFallsBackToCreated(t *testing.T) {
	created := time.Unix(1600000000, 0)
	d := &memory.Drive{
		Jobs:    []out.Job{{LinkID: "l1"}},
		Metas:   map[string]out.FileMeta{"l1": {Name: "a.jpg", Created: created, Active: true}},
		Content: map[string][]byte{"l1": []byte("x")},
	}
	f := &memory.Files{}
	s := scanned(t, d, f)
	if err := s.Download(ctx, "out"); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Get(filepath.Join("out", stamp(created)+"_a.jpg")); !ok {
		t.Fatal("file not written under creation-time name")
	}
}

// Given a photo's capture time
// When Download commits the file
// Then the capture time is forwarded as the modification time
func TestDownloadForwardsModTime(t *testing.T) {
	ts := time.Unix(1700000000, 0)
	d := &memory.Drive{
		Jobs:    []out.Job{{LinkID: "l1", Taken: ts}},
		Metas:   map[string]out.FileMeta{"l1": {Name: "a.jpg", Active: true}},
		Content: map[string][]byte{"l1": []byte("x")},
	}
	f := &memory.Files{}
	s := scanned(t, d, f)
	if err := s.Download(ctx, "out"); err != nil {
		t.Fatal(err)
	}
	file, _ := f.Get(filepath.Join("out", stamp(ts)+"_a.jpg"))
	if !file.ModTime.Equal(ts) {
		t.Fatalf("got mod time %v, want %v", file.ModTime, ts)
	}
}

// Given a destination file that already exists
// When Download runs
// Then the photo is skipped, not rewritten
func TestDownloadSkipsExisting(t *testing.T) {
	ts := time.Unix(1700000000, 0)
	d := &memory.Drive{
		Jobs:    []out.Job{{LinkID: "l1", Taken: ts}},
		Metas:   map[string]out.FileMeta{"l1": {Name: "a.jpg", Active: true}},
		Content: map[string][]byte{"l1": []byte("new")},
	}
	f := &memory.Files{}
	old := time.Unix(1, 0)
	f.Put(filepath.Join("out", stamp(ts)+"_a.jpg"), []byte("old"), old)
	s := scanned(t, d, f)
	if err := s.Download(ctx, "out"); err != nil {
		t.Fatal(err)
	}
	file, _ := f.Get(filepath.Join("out", stamp(ts)+"_a.jpg"))
	if string(file.Data) != "old" {
		t.Fatalf("existing file was rewritten to %q", file.Data)
	}
	if got := s.Progress().Skipped; got != 1 {
		t.Fatalf("got %d skipped, want 1", got)
	}
}

// Given an item the drive reports as inactive
// When Download runs
// Then the item is skipped without writing anything
func TestDownloadSkipsInactive(t *testing.T) {
	d := &memory.Drive{Jobs: []out.Job{{LinkID: "gone"}}}
	f := &memory.Files{}
	s := scanned(t, d, f)
	if err := s.Download(ctx, "out"); err != nil {
		t.Fatal(err)
	}
	if f.Len() != 0 || s.Progress().Skipped != 1 {
		t.Fatalf("got %d files, %d skipped; want 0 files, 1 skipped", f.Len(), s.Progress().Skipped)
	}
}

// Given two different photos with the same name and timestamp
// When Download runs
// Then both are written, the second under a disambiguated name
func TestDownloadCollision(t *testing.T) {
	ts := time.Unix(1700000000, 0)
	meta := out.FileMeta{Name: "a.jpg", Active: true}
	d := &memory.Drive{
		Jobs:    []out.Job{{LinkID: "link1xxxx", Taken: ts}, {LinkID: "link2xxxx", Taken: ts}},
		Metas:   map[string]out.FileMeta{"link1xxxx": meta, "link2xxxx": meta},
		Content: map[string][]byte{"link1xxxx": []byte("1"), "link2xxxx": []byte("2")},
	}
	f := &memory.Files{}
	s := scanned(t, d, f)
	if err := s.Download(ctx, "out"); err != nil {
		t.Fatal(err)
	}
	if f.Len() != 2 {
		t.Fatalf("got %d files, want 2", f.Len())
	}
}

// Given one failing item among healthy ones
// When Download runs
// Then the failure is counted and the rest still download
func TestDownloadCountsFailures(t *testing.T) {
	ts := time.Unix(1700000000, 0)
	d := &memory.Drive{
		Jobs:    []out.Job{{LinkID: "bad", Taken: ts}, {LinkID: "good", Taken: ts}},
		Metas:   map[string]out.FileMeta{"good": {Name: "g.jpg", Active: true}},
		Content: map[string][]byte{"good": []byte("x")},
		Errs:    map[string]error{"bad": errors.New("boom")},
	}
	f := &memory.Files{}
	s := scanned(t, d, f)
	if err := s.Download(ctx, "out"); err != nil {
		t.Fatal(err)
	}
	p := s.Progress()
	if p.Failed != 1 || p.Done != 1 || f.Len() != 1 {
		t.Fatalf("got %d failed, %d done, %d files; want 1, 1, 1", p.Failed, p.Done, f.Len())
	}
}

// Given a completed download
// When Progress is read
// Then the snapshot reports the totals and plaintext byte count
func TestDownloadProgressTotals(t *testing.T) {
	ts := time.Unix(1700000000, 0)
	d := &memory.Drive{
		Jobs:    []out.Job{{LinkID: "l1", Taken: ts}},
		Metas:   map[string]out.FileMeta{"l1": {Name: "a.jpg", Active: true}},
		Content: map[string][]byte{"l1": []byte("four")},
	}
	s := scanned(t, d, &memory.Files{})
	if err := s.Download(ctx, "out"); err != nil {
		t.Fatal(err)
	}
	p := s.Progress()
	if p.Total != 1 || p.Done != 1 || p.Bytes != 4 {
		t.Fatalf("got total %d, done %d, bytes %d; want 1, 1, 4", p.Total, p.Done, p.Bytes)
	}
}

// Given a destination directory that cannot be created
// When Download runs
// Then the error is passed through
func TestDownloadMkdirError(t *testing.T) {
	boom := errors.New("mkdir failed")
	s := scanned(t, &memory.Drive{}, &memory.Files{MkdirErr: boom})
	if err := s.Download(ctx, "out"); !errors.Is(err, boom) {
		t.Fatalf("got %v, want %v", err, boom)
	}
}

// Given a paused service
// When Download starts
// Then no item is fetched until Resume
func TestDownloadPauseAndResume(t *testing.T) {
	ts := time.Unix(1700000000, 0)
	d := &memory.Drive{
		Jobs:    []out.Job{{LinkID: "l1", Taken: ts}},
		Metas:   map[string]out.FileMeta{"l1": {Name: "a.jpg", Active: true}},
		Content: map[string][]byte{"l1": []byte("x")},
	}
	s := scanned(t, d, &memory.Files{})
	s.Pause()
	done := make(chan error, 1)
	go func() { done <- s.Download(ctx, "out") }()

	select {
	case <-done:
		t.Fatal("download finished while paused")
	case <-time.After(50 * time.Millisecond):
	}
	if got := d.StatCalls.Load(); got != 0 {
		t.Fatalf("got %d fetches while paused, want 0", got)
	}
	s.Resume()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// Given a cancelled context
// When Download runs
// Then the context error is returned
func TestDownloadCancelled(t *testing.T) {
	s := scanned(t, &memory.Drive{}, &memory.Files{})
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Download(cancelled, "out"); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}

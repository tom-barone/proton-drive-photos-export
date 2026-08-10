// AI GENERATED - Tom Barone

// Package memory provides in-memory fakes of Drive and Files for tests.
package memory

import (
	"bytes"
	"context"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tom-barone/proton-drive-photos-export/core/ports/out"
)

// Drive is a configurable in-memory out.Drive.
type Drive struct {
	NeedTOTP, NeedMailboxPass              bool
	SignInErr, TOTPErr, FinishErr, ListErr error

	Jobs      []out.Job
	UsedBytes int64
	Metas     map[string]out.FileMeta // linkID -> metadata; missing means inactive
	Content   map[string][]byte       // linkID -> plaintext
	Errs      map[string]error        // linkID -> Stat failure

	KeyPass   []byte // captured by FinishSignIn
	TOTP      string // captured by SubmitTOTP
	StatCalls atomic.Int64
}

func (d *Drive) SignIn(ctx context.Context, email, password string) (bool, bool, error) {
	if d.SignInErr != nil {
		return false, false, d.SignInErr
	}
	return d.NeedTOTP, d.NeedMailboxPass, nil
}

func (d *Drive) SubmitTOTP(ctx context.Context, code string) error {
	d.TOTP = code
	return d.TOTPErr
}

func (d *Drive) FinishSignIn(ctx context.Context, keyPass []byte) error {
	if d.FinishErr != nil {
		return d.FinishErr
	}
	d.KeyPass = keyPass
	return nil
}

func (d *Drive) ListPhotos(ctx context.Context) ([]out.Job, int64, error) {
	if d.ListErr != nil {
		return nil, 0, d.ListErr
	}
	return d.Jobs, d.UsedBytes, nil
}

func (d *Drive) Stat(ctx context.Context, linkID string) (out.FileMeta, error) {
	d.StatCalls.Add(1)
	if err := d.Errs[linkID]; err != nil {
		return out.FileMeta{}, err
	}
	return d.Metas[linkID], nil
}

func (d *Drive) Fetch(ctx context.Context, linkID string, w io.Writer) error {
	_, err := w.Write(d.Content[linkID])
	return err
}

// File is one committed write.
type File struct {
	Data    []byte
	ModTime time.Time
}

// Files is an in-memory out.Files.
type Files struct {
	MkdirErr error

	mu    sync.Mutex
	Dirs  []string
	files map[string]File
}

func (f *Files) MkdirAll(dir string) error {
	if f.MkdirErr != nil {
		return f.MkdirErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Dirs = append(f.Dirs, dir)
	return nil
}

func (f *Files) Exists(path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.files[path]
	return ok
}

func (f *Files) WriteFile(path string, modTime time.Time, write func(io.Writer) error) error {
	var buf bytes.Buffer
	if err := write(&buf); err != nil {
		return err
	}
	f.Put(path, buf.Bytes(), modTime)
	return nil
}

// Put seeds or overwrites a file.
func (f *Files) Put(path string, data []byte, modTime time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.files == nil {
		f.files = map[string]File{}
	}
	f.files[path] = File{data, modTime}
}

// Get returns a committed file, if any.
func (f *Files) Get(path string) (File, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	file, ok := f.files[path]
	return file, ok
}

// Len is the number of committed files.
func (f *Files) Len() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.files)
}

package core

import (
	"context"
	"io"
	"log"
	"sync"

	"github.com/tom-barone/proton-drive-photos-export/core/ports/in"
	"github.com/tom-barone/proton-drive-photos-export/core/ports/out"
)

const workers = 4

// Service implements in.Exporter. Sign-in happens in steps so the UI
// can show separate screens: SignIn (+ SubmitTOTP if needed) followed
// by FinishSignIn.
type Service struct {
	drive out.Drive
	files out.Files

	pass []byte    // held between SignIn and FinishSignIn
	jobs []out.Job // filled by Scan, used by Download
	g    *gate

	mu sync.Mutex // guards pr, swapped per Download run
	pr *progress
}

var _ in.Exporter = (*Service)(nil)

func New(drive out.Drive, files out.Files) *Service {
	return &Service{drive: drive, files: files, g: newGate(), pr: &progress{}}
}

func (s *Service) SignIn(ctx context.Context, email, password string) (needTOTP, needMailboxPass bool, err error) {
	needTOTP, needMailboxPass, err = s.drive.SignIn(ctx, email, password)
	if err != nil {
		return false, false, err
	}
	s.pass = []byte(password)
	return needTOTP, needMailboxPass, nil
}

func (s *Service) SubmitTOTP(ctx context.Context, code string) error {
	return s.drive.SubmitTOTP(ctx, code)
}

// FinishSignIn unlocks the account with the mailbox password, or the
// login password in single-password mode. The login password is kept on
// failure so the step can be retried.
func (s *Service) FinishSignIn(ctx context.Context, mailboxPass string) error {
	keyPass := s.pass
	if mailboxPass != "" {
		keyPass = []byte(mailboxPass)
	}
	if err := s.drive.FinishSignIn(ctx, keyPass); err != nil {
		return err
	}
	s.pass = nil
	return nil
}

func (s *Service) Scan(ctx context.Context) (int, int64, error) {
	jobs, usedBytes, err := s.drive.ListPhotos(ctx)
	if err != nil {
		return 0, 0, err
	}
	s.jobs = jobs
	return len(jobs), usedBytes, nil
}

// run holds the state shared by one Download's workers.
type run struct {
	pr *progress

	mu      sync.Mutex
	claimed map[string]string // local path -> linkID
}

// Download fetches every job found by Scan into dir. Per-file failures
// are counted and logged, never fatal; the returned error is only the
// context's.
func (s *Service) Download(ctx context.Context, dir string) error {
	if err := s.files.MkdirAll(dir); err != nil {
		return err
	}
	r := &run{pr: &progress{total: int64(len(s.jobs))}, claimed: map[string]string{}}
	s.mu.Lock()
	s.pr = r.pr
	s.mu.Unlock()

	ch := make(chan out.Job)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				switch skip, err := s.downloadOne(ctx, dir, j, r); {
				case err != nil:
					log.Printf("FAIL %s: %v", j.LinkID, err)
					r.pr.failed.Add(1)
				case skip:
					r.pr.skipped.Add(1)
				default:
					r.pr.done.Add(1)
				}
			}
		}()
	}
feed:
	for _, j := range s.jobs {
		if err := s.g.wait(ctx); err != nil {
			break
		}
		select {
		case ch <- j:
		case <-ctx.Done():
			break feed
		}
	}
	close(ch)
	wg.Wait()
	return ctx.Err()
}

func (s *Service) downloadOne(ctx context.Context, dir string, j out.Job, r *run) (skipped bool, err error) {
	meta, err := s.drive.Stat(ctx, j.LinkID)
	if err != nil {
		return false, err
	}
	if !meta.Active {
		return true, nil
	}

	ts := j.Taken
	if ts.IsZero() {
		ts = meta.Created
	}
	r.mu.Lock()
	path := photoPath(dir, meta.Name, ts, j.LinkID, r.claimed)
	r.mu.Unlock()
	if s.files.Exists(path) {
		return true, nil
	}

	return false, s.files.WriteFile(path, ts, func(w io.Writer) error {
		return s.drive.Fetch(ctx, j.LinkID, &countingWriter{w, &r.pr.bytes})
	})
}

func (s *Service) Pause()  { s.g.pause() }
func (s *Service) Resume() { s.g.resume() }

func (s *Service) Progress() in.Progress {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pr.snapshot()
}

package out

import (
	"context"
	"io"
	"time"
)

// Job is one downloadable photo library item.
type Job struct {
	LinkID string
	Taken  time.Time // zero if unknown; core falls back to FileMeta.Created
}

// FileMeta describes a single item without touching its content.
type FileMeta struct {
	Name    string // decrypted original name; empty when !Active
	Created time.Time
	Active  bool // false: trashed, draft or not a file; skip it
}

// Drive is the photo cloud. Implementations own all transport, token
// and cryptography concerns; callers only ever see plaintext.
type Drive interface {
	// SignIn starts a fresh login and reports which extra steps are
	// needed before FinishSignIn.
	SignIn(ctx context.Context, email, password string) (needTOTP, needMailboxPass bool, err error)
	SubmitTOTP(ctx context.Context, code string) error
	// FinishSignIn unlocks account access with keyPass, the login or
	// mailbox password.
	FinishSignIn(ctx context.Context, keyPass []byte) error
	// ListPhotos returns every item in the photo library plus the
	// library's total size in bytes.
	ListPhotos(ctx context.Context) (jobs []Job, usedBytes int64, err error)
	Stat(ctx context.Context, linkID string) (FileMeta, error)
	// Fetch writes the item's verified, decrypted content to w.
	Fetch(ctx context.Context, linkID string, w io.Writer) error
}

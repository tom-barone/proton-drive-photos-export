// AI GENERATED - Tom Barone
package in

import "context"

// Exporter is the app's full use-case API. It is not safe for
// concurrent use; call it from one flow at a time.
type Exporter interface {
	// SignIn starts a fresh login and reports which extra steps are
	// needed before FinishSignIn.
	SignIn(ctx context.Context, email, password string) (needTOTP, needMailboxPass bool, err error)
	SubmitTOTP(ctx context.Context, code string) error
	// FinishSignIn completes login; mailboxPass is empty in
	// single-password mode.
	FinishSignIn(ctx context.Context, mailboxPass string) error
	// Scan lists everything to download; usedBytes is the photo
	// library's total size, for display only.
	Scan(ctx context.Context) (photos int, usedBytes int64, err error)
	// Download fetches everything found by Scan into dir. Per-file
	// failures are counted, never fatal; the returned error is only
	// the context's.
	Download(ctx context.Context, dir string) error
	// Pause and Resume toggle a persistent switch honored by
	// Download; a download started while paused stays parked until
	// Resume.
	Pause()
	Resume()
	Progress() Progress
}

// Progress is a point-in-time snapshot of a running download.
type Progress struct {
	Total, Done, Skipped, Failed, Bytes int64
}

// Items is the number of jobs finished one way or another.
func (p Progress) Items() int64 { return p.Done + p.Skipped + p.Failed }

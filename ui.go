// AI GENERATED - Tom Barone
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/ProtonMail/go-proton-api"
)

// ui owns the window and walks the screens:
// SignedOut -> Scanning -> Ready -> Downloading <-> Paused -> Done.
// Engine calls run in goroutines; widget updates go through fyne.Do.
type ui struct {
	win fyne.Window
	eng *engine
	ctx context.Context

	jobs      []job
	usedBytes int64
	dir       string

	needMailboxPass bool
	pr              *progress
	gate            *gate
	paused          atomic.Bool
}

// setScreen replaces the window content with items centered vertically.
func (u *ui) setScreen(items ...fyne.CanvasObject) {
	all := append([]fyne.CanvasObject{layout.NewSpacer()}, items...)
	all = append(all, layout.NewSpacer())
	u.win.SetContent(container.NewPadded(container.NewVBox(all...)))
}

func (u *ui) startup() {
	if err := u.eng.restoreSession(u.ctx); err != nil {
		log.Printf("no usable cached session: %v", err)
		fyne.Do(func() { u.showSignIn("") })
		return
	}
	u.runScan()
}

func (u *ui) showSignIn(errMsg string) {
	heading := widget.NewLabelWithStyle("Sign in to Proton", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	email := widget.NewEntry()
	email.SetPlaceHolder("Email")
	pass := widget.NewPasswordEntry()
	pass.SetPlaceHolder("Password")
	errLabel := errorLabel(errMsg)
	btn := widget.NewButton("Sign in", nil)
	btn.Importance = widget.HighImportance
	btn.OnTapped = func() {
		btn.Disable()
		go u.runSignIn(email.Text, pass.Text)
	}
	pass.OnSubmitted = func(string) { btn.OnTapped() }
	u.setScreen(heading, email, pass, errLabel, btn)
}

func (u *ui) runSignIn(email, password string) {
	needTOTP, needMailbox, err := u.eng.signIn(u.ctx, email, password)
	if err != nil {
		log.Printf("sign in: %v", err)
		fyne.Do(func() { u.showSignIn(friendlyError(err)) })
		return
	}
	fyne.Do(func() {
		u.needMailboxPass = needMailbox
		switch {
		case needTOTP:
			u.showTOTP("")
		case needMailbox:
			u.showMailboxPass()
		default:
			go u.runFinishSignIn("")
		}
	})
}

func (u *ui) showTOTP(errMsg string) {
	heading := widget.NewLabelWithStyle("Two-factor authentication", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	code := widget.NewEntry()
	code.SetPlaceHolder("6-digit code")
	errLabel := errorLabel(errMsg)
	btn := widget.NewButton("Continue", nil)
	btn.Importance = widget.HighImportance
	btn.OnTapped = func() {
		btn.Disable()
		go func() {
			if err := u.eng.submitTOTP(u.ctx, code.Text); err != nil {
				log.Printf("totp: %v", err)
				fyne.Do(func() { u.showTOTP(friendlyError(err)) })
				return
			}
			fyne.Do(func() {
				if u.needMailboxPass {
					u.showMailboxPass()
				} else {
					go u.runFinishSignIn("")
				}
			})
		}()
	}
	code.OnSubmitted = func(string) { btn.OnTapped() }
	u.setScreen(heading, code, errLabel, btn)
}

func (u *ui) showMailboxPass() {
	heading := widget.NewLabelWithStyle("Mailbox password", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	pass := widget.NewPasswordEntry()
	pass.SetPlaceHolder("Mailbox password")
	btn := widget.NewButton("Continue", nil)
	btn.Importance = widget.HighImportance
	btn.OnTapped = func() {
		btn.Disable()
		go u.runFinishSignIn(pass.Text)
	}
	pass.OnSubmitted = func(string) { btn.OnTapped() }
	u.setScreen(heading, pass, btn)
}

func (u *ui) runFinishSignIn(mailboxPass string) {
	if err := u.eng.finishSignIn(u.ctx, mailboxPass); err != nil {
		log.Printf("finishing sign in: %v", err)
		fyne.Do(func() { u.showSignIn(friendlyError(err)) })
		return
	}
	u.runScan()
}

func (u *ui) showScanning(msg string) {
	u.setScreen(
		widget.NewLabelWithStyle(msg, fyne.TextAlignCenter, fyne.TextStyle{}),
		widget.NewProgressBarInfinite(),
	)
}

func (u *ui) runScan() {
	fyne.Do(func() { u.showScanning("Scanning for photos...") })
	res, err := u.eng.scan(u.ctx)
	if err != nil {
		if u.ctx.Err() != nil {
			return
		}
		log.Printf("scan: %v", err)
		fyne.Do(func() { u.showSignIn(friendlyError(err)) })
		return
	}
	log.Printf("scan: %d items, %d bytes in volume", len(res.jobs), res.usedBytes)
	fyne.Do(func() {
		u.jobs, u.usedBytes = res.jobs, res.usedBytes
		u.showReady()
	})
}

func (u *ui) showReady() {
	found := widget.NewLabelWithStyle(
		fmt.Sprintf("%d photos found in your Proton Drive, %.1f GB", len(u.jobs), float64(u.usedBytes)/1e9),
		fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	pathLabel := widget.NewLabel(u.dir)
	pathLabel.Truncation = fyne.TextTruncateEllipsis
	choose := widget.NewButton("Choose...", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			u.dir = uri.Path()
			pathLabel.SetText(u.dir)
		}, u.win)
	})
	folder := container.NewBorder(nil, nil, widget.NewLabel("Save to:"), choose, pathLabel)
	dl := widget.NewButton("Download all photos", u.startDownload)
	dl.Importance = widget.HighImportance
	u.setScreen(found, folder, dl)
}

func (u *ui) startDownload() {
	u.pr = &progress{total: int64(len(u.jobs))}
	u.gate = newGate()
	u.paused.Store(false)
	u.showDownloading()
	go func() {
		if err := u.eng.download(u.ctx, u.dir, u.jobs, u.pr, u.gate); err != nil {
			if u.ctx.Err() == nil {
				log.Printf("download: %v", err)
				fyne.Do(func() {
					dialog.ShowError(errors.New(friendlyError(err)), u.win)
					u.showReady()
				})
			}
			return
		}
		log.Printf("download finished: %d done, %d skipped, %d failed",
			u.pr.done.Load(), u.pr.skipped.Load(), u.pr.failed.Load())
		fyne.Do(func() { u.showDone() })
	}()
}

func (u *ui) showDownloading() {
	bar := widget.NewProgressBar()
	counts := widget.NewLabelWithStyle("Starting download...", fyne.TextAlignCenter, fyne.TextStyle{})
	eta := widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{})
	var pauseBtn *widget.Button
	pauseBtn = widget.NewButton("Pause", func() {
		if u.paused.Load() {
			u.paused.Store(false)
			u.gate.resume()
			pauseBtn.SetText("Pause")
		} else {
			u.paused.Store(true)
			u.gate.pause()
			pauseBtn.SetText("Resume")
		}
	})
	u.setScreen(counts, bar, eta, pauseBtn)
	go u.trackProgress(bar, counts, eta)
}

// sample is one progress reading; the tracker keeps a short window of
// them to compute recent throughput and ETA.
type sample struct {
	t            time.Time
	items, bytes int64
}

func (u *ui) trackProgress(bar *widget.ProgressBar, counts, eta *widget.Label) {
	var window []sample
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-u.ctx.Done():
			return
		case <-tick.C:
		}
		items := u.pr.items()
		window = append(window, sample{time.Now(), items, u.pr.bytes.Load()})
		if len(window) > 30 {
			window = window[1:]
		}
		countText := fmt.Sprintf("Downloading %d of %d photos (%d%%)",
			items, u.pr.total, 100*items/max(u.pr.total, 1))
		etaText := u.rateText(window[0], window[len(window)-1])
		fyne.Do(func() {
			bar.SetValue(float64(items) / float64(max(u.pr.total, 1)))
			counts.SetText(countText)
			eta.SetText(etaText)
		})
		if items >= u.pr.total {
			return
		}
	}
}

func (u *ui) rateText(first, last sample) string {
	if u.paused.Load() {
		return "Paused"
	}
	elapsed := last.t.Sub(first.t).Seconds()
	if elapsed <= 0 {
		return ""
	}
	mbps := float64(last.bytes-first.bytes) / elapsed / 1e6
	itemRate := float64(last.items-first.items) / elapsed
	if itemRate <= 0 {
		return fmt.Sprintf("%.1f MB/s", mbps)
	}
	remaining := time.Duration(float64(u.pr.total-last.items) / itemRate * float64(time.Second))
	return fmt.Sprintf("%s, %.1f MB/s", etaPhrase(remaining), mbps)
}

func etaPhrase(d time.Duration) string {
	switch m := int(d.Minutes()); {
	case d < time.Minute:
		return "Less than a minute remaining"
	case m == 1:
		return "About 1 minute remaining"
	case m < 60:
		return fmt.Sprintf("About %d minutes remaining", m)
	default:
		return fmt.Sprintf("About %dh %dm remaining", m/60, m%60)
	}
}

func (u *ui) showDone() {
	msg := "Done! All photos have been downloaded successfully."
	if failed := u.pr.failed.Load(); failed > 0 {
		msg = fmt.Sprintf("Done, but %d of %d photos failed to download.\nCheck the log file for details.", failed, u.pr.total)
	}
	label := widget.NewLabelWithStyle(msg, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	label.Wrapping = fyne.TextWrapWord
	open := widget.NewButton("Open folder", func() { openFolder(u.dir) })
	open.Importance = widget.HighImportance
	u.setScreen(label, open)
}

// errorLabel is a red wrapping label, hidden while there is no message.
func errorLabel(msg string) *widget.Label {
	l := widget.NewLabel(msg)
	l.Wrapping = fyne.TextWrapWord
	l.Importance = widget.DangerImportance
	if msg == "" {
		l.Hide()
	}
	return l
}

func friendlyError(err error) string {
	var apiErr *proton.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Message
	}
	return err.Error()
}

func openFolder(path string) {
	cmd := "xdg-open"
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "explorer"
	}
	if err := exec.Command(cmd, path).Start(); err != nil {
		log.Printf("opening folder: %v", err)
	}
}

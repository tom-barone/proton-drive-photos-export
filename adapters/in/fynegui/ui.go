package fynegui

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/ncruces/zenity"

	"github.com/tom-barone/proton-drive-photos-export/core"
	"github.com/tom-barone/proton-drive-photos-export/core/ports/in"
)

// Run builds the window and runs the app until it is closed.
func Run(svc in.Exporter) {
	a := app.NewWithID("net.tombarone.protonphotosexport")
	a.Settings().SetTheme(&protonTheme{Theme: theme.DefaultTheme()})
	w := a.NewWindow("Photos Export for Proton Drive")
	w.Resize(fyne.NewSize(460, 320))

	ctx, cancel := context.WithCancel(context.Background())
	w.SetOnClosed(cancel)

	dir := a.Preferences().StringWithFallback(dirPref, defaultDir())
	u := &ui{win: w, svc: svc, ctx: ctx, dir: dir}
	u.showSignIn("")
	w.ShowAndRun()
}

// dirPref is the preferences key holding the destination folder.
const dirPref = "dir"

// ui owns the window and walks the screens:
// SignedOut -> Scanning -> Ready -> Downloading <-> Paused -> Done.
// Core calls run in goroutines; widget updates go through fyne.Do.
type ui struct {
	win fyne.Window
	svc in.Exporter
	ctx context.Context

	photos    int
	usedBytes int64
	dir       string

	needMailboxPass bool
	paused          atomic.Bool
}

func defaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, "Downloads", "Proton Drive Photos")
}

// setScreen replaces the window content with items centered vertically.
func (u *ui) setScreen(items ...fyne.CanvasObject) {
	all := append([]fyne.CanvasObject{layout.NewSpacer()}, items...)
	all = append(all, layout.NewSpacer())
	u.win.SetContent(container.NewPadded(container.NewVBox(all...)))
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
	needTOTP, needMailbox, err := u.svc.SignIn(u.ctx, email, password)
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
			if err := u.svc.SubmitTOTP(u.ctx, code.Text); err != nil {
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
	if err := u.svc.FinishSignIn(u.ctx, mailboxPass); err != nil {
		log.Printf("finishing sign in: %v", err)
		fyne.Do(func() { u.showSignIn(friendlyError(err)) })
		return
	}
	u.runScan()
}

func (u *ui) showScanning() {
	u.setScreen(
		widget.NewLabelWithStyle("Scanning for photos...", fyne.TextAlignCenter, fyne.TextStyle{}),
		widget.NewProgressBarInfinite(),
	)
}

func (u *ui) runScan() {
	fyne.Do(func() { u.showScanning() })
	photos, usedBytes, err := u.svc.Scan(u.ctx)
	if err != nil {
		if u.ctx.Err() != nil {
			return
		}
		log.Printf("scan: %v", err)
		fyne.Do(func() { u.showSignIn(friendlyError(err)) })
		return
	}
	log.Printf("scan: %d items, %d bytes in volume", photos, usedBytes)
	fyne.Do(func() {
		u.photos, u.usedBytes = photos, usedBytes
		u.showReady()
	})
}

func (u *ui) showReady() {
	found := widget.NewLabelWithStyle(
		fmt.Sprintf("%d photos found in your Proton Drive, %.1f GB", u.photos, float64(u.usedBytes)/1e9),
		fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	pathLabel := widget.NewLabel(u.dir)
	pathLabel.Truncation = fyne.TextTruncateEllipsis
	choose := widget.NewButton("Choose...", func() {
		go func() {
			path, err := zenity.SelectFile(zenity.Directory(), zenity.Filename(u.dir))
			if err != nil {
				if !errors.Is(err, zenity.ErrCanceled) {
					log.Printf("folder picker: %v", err)
					fyne.Do(func() { u.chooseFallback(pathLabel) })
				}
				return
			}
			fyne.Do(func() { u.setDir(path, pathLabel) })
		}()
	})
	folder := container.NewBorder(nil, nil, widget.NewLabel("Save to:"), choose, pathLabel)
	dl := widget.NewButton("Download all photos", u.startDownload)
	dl.Importance = widget.HighImportance
	u.setScreen(found, folder, dl)
}

// chooseFallback shows Fyne's folder dialog when no native picker is available.
func (u *ui) chooseFallback(pathLabel *widget.Label) {
	dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil || uri == nil {
			return
		}
		u.setDir(uri.Path(), pathLabel)
	}, u.win)
}

// setDir switches the destination folder and remembers it for the next
// launch.
func (u *ui) setDir(dir string, pathLabel *widget.Label) {
	u.dir = dir
	fyne.CurrentApp().Preferences().SetString(dirPref, dir)
	pathLabel.SetText(dir)
}

func (u *ui) startDownload() {
	u.paused.Store(false)
	u.svc.Resume()
	u.showDownloading()
	go func() {
		if err := u.svc.Download(u.ctx, u.dir); err != nil {
			if u.ctx.Err() == nil {
				log.Printf("download: %v", err)
				fyne.Do(func() {
					dialog.ShowError(errors.New(friendlyError(err)), u.win)
					u.showReady()
				})
			}
			return
		}
		p := u.svc.Progress()
		log.Printf("download finished: %d done, %d skipped, %d failed", p.Done, p.Skipped, p.Failed)
		fyne.Do(func() { u.showDone() })
	}()
}

func (u *ui) showDownloading() {
	bar, counts, eta := u.downloadingScreen()
	go u.trackProgress(bar, counts, eta)
}

// downloadingScreen builds the progress screen and returns the widgets
// the tracker updates.
func (u *ui) downloadingScreen() (*widget.ProgressBar, *widget.Label, *widget.Label) {
	bar := widget.NewProgressBar()
	counts := widget.NewLabelWithStyle("Starting download...", fyne.TextAlignCenter, fyne.TextStyle{})
	eta := widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{})
	var pauseBtn *widget.Button
	pauseBtn = widget.NewButton("Pause", func() {
		if u.paused.Load() {
			u.paused.Store(false)
			u.svc.Resume()
			pauseBtn.SetText("Pause")
		} else {
			u.paused.Store(true)
			u.svc.Pause()
			pauseBtn.SetText("Resume")
		}
	})
	u.setScreen(counts, bar, eta, pauseBtn)
	return bar, counts, eta
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
		p := u.svc.Progress()
		items := p.Items()
		window = append(window, sample{time.Now(), items, p.Bytes})
		if len(window) > 30 {
			window = window[1:]
		}
		first, last := window[0], window[len(window)-1]
		fyne.Do(func() { u.updateProgress(bar, counts, eta, p.Total, first, last) })
		if p.Total > 0 && items >= p.Total {
			return
		}
	}
}

// updateProgress renders one progress reading into the widgets.
func (u *ui) updateProgress(bar *widget.ProgressBar, counts, eta *widget.Label, total int64, first, last sample) {
	bar.SetValue(float64(last.items) / float64(max(total, 1)))
	counts.SetText(fmt.Sprintf("Downloading %d of %d photos (%d%%)",
		last.items, total, 100*last.items/max(total, 1)))
	eta.SetText(u.rateText(total, first, last))
}

func (u *ui) rateText(total int64, first, last sample) string {
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
	remaining := time.Duration(float64(total-last.items) / itemRate * float64(time.Second))
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
	p := u.svc.Progress()
	msg := "Done! All photos have been downloaded successfully."
	if p.Failed > 0 {
		msg = fmt.Sprintf("Done, but %d of %d photos failed to download.\nCheck the log file for details.", p.Failed, p.Total)
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
	var ue *core.UserError
	if errors.As(err, &ue) {
		return ue.Msg
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

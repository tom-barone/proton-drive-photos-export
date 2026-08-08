// AI GENERATED - Tom Barone
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"
)

func main() {
	setupLog()
	a := app.NewWithID("net.tombarone.protonphotoexport")
	a.Settings().SetTheme(&protonTheme{Theme: theme.DefaultTheme()})
	w := a.NewWindow("Photo Export for Proton Drive")
	w.Resize(fyne.NewSize(460, 320))
	w.SetFixedSize(true)

	ctx, cancel := context.WithCancel(context.Background())
	u := &ui{win: w, eng: newEngine(), ctx: ctx, dir: defaultDir()}
	w.SetOnClosed(cancel)

	u.showScanning("Signing in...")
	go u.startup()
	w.ShowAndRun()
}

// setupLog sends the standard logger to a file, since a GUI app has no
// useful stderr. Best effort: on failure logs just go nowhere visible.
func setupLog() {
	dir, err := os.UserCacheDir()
	if err != nil {
		return
	}
	dir = filepath.Join(dir, "ProtonPhotoExport")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "app.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(f)
	log.Printf("--- app start ---")
}

func defaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, "Downloads", "Proton Drive Photos")
}

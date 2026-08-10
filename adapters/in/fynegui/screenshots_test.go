package fynegui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

// Given the sign-in and downloading screens rendered with mocked values
// When the test runs with SCREENSHOTS_DIR set
// Then a PNG of each screen is written to that directory
func TestScreenshots(t *testing.T) {
	dir := os.Getenv("SCREENSHOTS_DIR")
	if dir == "" {
		t.Skip("SCREENSHOTS_DIR not set")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	a := test.NewApp()
	a.Settings().SetTheme(&protonTheme{Theme: theme.DefaultTheme()})
	w := test.NewWindow(nil)
	w.Resize(fyne.NewSize(460, 320))
	u := &ui{win: w, dir: defaultDir()}

	u.showSignIn("")
	save(t, w, filepath.Join(dir, "sign-in.png"))

	bar, counts, eta := u.downloadingScreen()
	t0 := time.Unix(0, 0)
	u.updateProgress(bar, counts, eta, 12179,
		sample{t0, 1911, 0},
		sample{t0.Add(60 * time.Second), 2248, 630e6})
	save(t, w, filepath.Join(dir, "downloading.png"))
}

func save(t *testing.T, w fyne.Window, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, w.Canvas().Capture()); err != nil {
		t.Fatal(err)
	}
}

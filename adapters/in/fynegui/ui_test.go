package fynegui

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// Given a folder chosen by the user
// When setDir runs
// Then the folder is saved in the app preferences
func TestSetDirSavesPreference(t *testing.T) {
	a := test.NewApp()
	u := &ui{}
	u.setDir("/photos", widget.NewLabel(""))
	if got := a.Preferences().String(dirPref); got != "/photos" {
		t.Fatalf("got preference %q, want /photos", got)
	}
}

// Given a folder chosen by the user
// When setDir runs
// Then the folder becomes the download destination
func TestSetDirUpdatesDestination(t *testing.T) {
	test.NewApp()
	u := &ui{}
	u.setDir("/photos", widget.NewLabel(""))
	if u.dir != "/photos" {
		t.Fatalf("got dir %q, want /photos", u.dir)
	}
}

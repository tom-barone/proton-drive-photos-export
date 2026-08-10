// AI GENERATED - Tom Barone

// The composition root: it wires the driven adapters into the core and
// hands the core to the driving Fyne adapter.
package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/tom-barone/proton-drive-photos-export/adapters/in/fynegui"
	"github.com/tom-barone/proton-drive-photos-export/adapters/out/localfs"
	"github.com/tom-barone/proton-drive-photos-export/adapters/out/protondrive"
	"github.com/tom-barone/proton-drive-photos-export/core"
)

func main() {
	setupLog()
	removeStaleSession()
	fynegui.Run(core.New(protondrive.New(), localfs.Files{}))
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

// removeStaleSession deletes the session file older versions cached;
// leaving a refresh token on disk after dropping the feature would be
// a liability.
func removeStaleSession() {
	if dir, err := os.UserConfigDir(); err == nil {
		os.Remove(filepath.Join(dir, "ProtonPhotoExport", "session.json"))
	}
}

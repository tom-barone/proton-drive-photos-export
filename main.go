package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/tom-barone/proton-drive-photos-export/adapters/in/fynegui"
	"github.com/tom-barone/proton-drive-photos-export/adapters/out/localfs"
	"github.com/tom-barone/proton-drive-photos-export/adapters/out/protondrive"
	"github.com/tom-barone/proton-drive-photos-export/core"
)

func main() {
	logFile, err := setupLog()
	if err != nil {
		log.Printf("file logging disabled: %v", err)
	} else {
		defer logFile.Close()
	}
	fynegui.Run(core.New(protondrive.New(), localfs.Files{}))
}

func setupLog() (*os.File, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("locating cache dir: %w", err)
	}
	dir = filepath.Join(dir, "ProtonPhotosExport")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating log dir %s: %w", dir, err)
	}
	path := filepath.Join(dir, "app.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening log file %s: %w", path, err)
	}
	log.SetOutput(f)
	return f, nil
}

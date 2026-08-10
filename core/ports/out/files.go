// AI GENERATED - Tom Barone
package out

import (
	"io"
	"time"
)

// Files is the local destination for downloaded photos.
type Files interface {
	MkdirAll(dir string) error
	Exists(path string) bool
	// WriteFile calls write with a destination and commits the result
	// with the given modification time; nothing is left behind when
	// write fails.
	WriteFile(path string, modTime time.Time, write func(io.Writer) error) error
}

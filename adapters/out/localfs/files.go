
// Package localfs stores downloaded photos on the local disk.
package localfs

import (
	"io"
	"os"
	"time"
)

type Files struct{}

func (Files) MkdirAll(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

func (Files) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// WriteFile streams through a .part file renamed into place on success,
// so interrupted downloads never leave half files behind.
func (Files) WriteFile(path string, modTime time.Time, write func(io.Writer) error) (err error) {
	part := path + ".part"
	f, err := os.Create(part)
	if err != nil {
		return err
	}
	defer func() {
		f.Close()
		if err != nil {
			os.Remove(part)
		}
	}()

	if err = write(f); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(part, path); err != nil {
		return err
	}
	os.Chtimes(path, modTime, modTime) // best effort
	return nil
}

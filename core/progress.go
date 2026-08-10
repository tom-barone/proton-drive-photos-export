package core

import (
	"io"
	"sync/atomic"

	"github.com/tom-barone/proton-drive-photos-export/core/ports/in"
)

// progress is the shared mutable state behind in.Progress snapshots.
type progress struct {
	total                        int64
	done, skipped, failed, bytes atomic.Int64
}

func (p *progress) snapshot() in.Progress {
	return in.Progress{
		Total:   p.total,
		Done:    p.done.Load(),
		Skipped: p.skipped.Load(),
		Failed:  p.failed.Load(),
		Bytes:   p.bytes.Load(),
	}
}

// countingWriter counts decrypted bytes as they stream to the sink.
type countingWriter struct {
	w io.Writer
	n *atomic.Int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n.Add(int64(n))
	return n, err
}

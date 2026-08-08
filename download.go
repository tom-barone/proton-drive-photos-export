// AI GENERATED - Tom Barone
package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ProtonMail/go-proton-api"
	"github.com/ProtonMail/gopenpgp/v2/crypto"
)

const (
	blockPageSize = 50
	workers       = 4
)

// progress is read by the UI ticker while a download runs.
type progress struct {
	total                        int64
	done, skipped, failed, bytes atomic.Int64
}

// items is the number of jobs finished one way or another.
func (p *progress) items() int64 {
	return p.done.Load() + p.skipped.Load() + p.failed.Load()
}

// gate pauses the download feed. A closed channel means running.
type gate struct {
	mu   sync.Mutex
	open chan struct{}
}

func newGate() *gate {
	g := &gate{open: make(chan struct{})}
	close(g.open)
	return g
}

func (g *gate) pause() {
	g.mu.Lock()
	defer g.mu.Unlock()
	select {
	case <-g.open:
		g.open = make(chan struct{})
	default:
	}
}

func (g *gate) resume() {
	g.mu.Lock()
	defer g.mu.Unlock()
	select {
	case <-g.open:
	default:
		close(g.open)
	}
}

func (g *gate) wait(ctx context.Context) error {
	g.mu.Lock()
	open := g.open
	g.mu.Unlock()
	select {
	case <-open:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// download fetches all jobs into dir. Per-file failures are counted in
// pr and logged, never fatal; the returned error is only the context's.
func (e *engine) download(ctx context.Context, dir string, jobs []job, pr *progress, g *gate) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	d := &downloader{
		c:       e.client,
		shareID: e.share.ShareID,
		shareKR: e.shareKR,
		addrKR:  e.addrKR,
		krs:     map[string]*crypto.KeyRing{},
		claimed: map[string]string{},
		pr:      pr,
	}

	ch := make(chan job)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				switch skip, err := d.downloadOne(ctx, dir, j); {
				case err != nil:
					log.Printf("FAIL %s: %v", j.linkID, err)
					pr.failed.Add(1)
				case skip:
					pr.skipped.Add(1)
				default:
					pr.done.Add(1)
				}
			}
		}()
	}
feed:
	for _, j := range jobs {
		if err := g.wait(ctx); err != nil {
			break
		}
		select {
		case ch <- j:
		case <-ctx.Done():
			break feed
		}
	}
	close(ch)
	wg.Wait()
	return ctx.Err()
}

type downloader struct {
	c       *proton.Client
	shareID string
	shareKR *crypto.KeyRing
	addrKR  *crypto.KeyRing // all address keys, for signature verification
	pr      *progress

	mu      sync.Mutex
	krs     map[string]*crypto.KeyRing // linkID -> node keyring
	claimed map[string]string          // local path -> linkID
}

// nodeKR resolves a link's node keyring by walking up to the share root,
// caching every keyring on the way.
func (d *downloader) nodeKR(ctx context.Context, linkID string) (*crypto.KeyRing, error) {
	if linkID == "" {
		return d.shareKR, nil
	}
	d.mu.Lock()
	kr, ok := d.krs[linkID]
	d.mu.Unlock()
	if ok {
		return kr, nil
	}

	link, err := d.c.GetLink(ctx, d.shareID, linkID)
	if err != nil {
		return nil, err
	}
	parentKR, err := d.nodeKR(ctx, link.ParentLinkID)
	if err != nil {
		return nil, err
	}
	kr, err = link.GetKeyRing(parentKR, d.addrKR)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	d.krs[linkID] = kr
	d.mu.Unlock()
	return kr, nil
}

func (d *downloader) downloadOne(ctx context.Context, dir string, j job) (skipped bool, err error) {
	link, err := d.c.GetLink(ctx, d.shareID, j.linkID)
	if err != nil {
		return false, err
	}
	if link.State != proton.LinkStateActive || link.Type != proton.LinkTypeFile {
		return true, nil
	}

	parentKR, err := d.nodeKR(ctx, link.ParentLinkID)
	if err != nil {
		return false, err
	}
	name, err := link.GetName(parentKR, nil)
	if err != nil {
		return false, fmt.Errorf("decrypting name: %w", err)
	}

	ts := time.Unix(j.captureTime, 0)
	if j.captureTime == 0 {
		ts = time.Unix(link.CreateTime, 0)
	}

	d.mu.Lock()
	path := photoPath(dir, name, ts, link.LinkID, d.claimed)
	d.mu.Unlock()

	if _, err := os.Stat(path); err == nil {
		return true, nil
	}

	nodeKR, err := d.nodeKR(ctx, link.LinkID)
	if err != nil {
		return false, err
	}
	sk, err := link.GetSessionKey(nodeKR)
	if err != nil {
		return false, fmt.Errorf("decrypting session key: %w", err)
	}

	if err := d.fetch(ctx, path, link, sk); err != nil {
		return false, err
	}
	os.Chtimes(path, ts, ts)
	return false, nil
}

// fetch downloads, verifies and decrypts all blocks of the file's active
// revision, writing through a .part file renamed into place on success.
func (d *downloader) fetch(ctx context.Context, path string, link proton.Link, sk *crypto.SessionKey) (err error) {
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

	for from := 1; ; from += blockPageSize {
		rev, err := d.c.GetRevision(ctx, d.shareID, link.LinkID, link.FileProperties.ActiveRevision.ID, from, blockPageSize)
		if err != nil {
			return err
		}
		for _, b := range rev.Blocks {
			if err := d.writeBlock(ctx, f, sk, b); err != nil {
				return fmt.Errorf("block %d: %w", b.Index, err)
			}
		}
		if len(rev.Blocks) < blockPageSize {
			break
		}
	}

	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(part, path)
}

func (d *downloader) writeBlock(ctx context.Context, w io.Writer, sk *crypto.SessionKey, b proton.Block) error {
	rc, err := d.c.GetBlock(ctx, b.BareURL, b.Token)
	if err != nil {
		return err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if got := base64.StdEncoding.EncodeToString(sum[:]); got != b.Hash {
		return fmt.Errorf("hash mismatch: got %s want %s", got, b.Hash)
	}
	msg, err := sk.Decrypt(data)
	if err != nil {
		return err
	}
	n, err := w.Write(msg.GetBinary())
	d.pr.bytes.Add(int64(n))
	return err
}

// photoPath returns a deterministic destination path. If a different link
// already claimed the same path this run, the link ID disambiguates.
func photoPath(dir, name string, ts time.Time, linkID string, claimed map[string]string) string {
	name = sanitizeName(name)
	stamp := ts.Format("20060102_150405")
	path := filepath.Join(dir, stamp+"_"+name)
	if owner, ok := claimed[path]; ok && owner != linkID {
		path = filepath.Join(dir, stamp+"_"+linkID[:min(8, len(linkID))]+"_"+name)
	}
	claimed[path] = linkID
	return path
}

var reservedName = regexp.MustCompile(`^(?i)(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(\.|$)`)

// sanitizeName makes a decrypted file name safe on every target OS,
// replacing characters Windows forbids and dodging its reserved names.
func sanitizeName(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimRight(name, ". ")
	if name == "" {
		return "unnamed"
	}
	if reservedName.MatchString(name) {
		return "_" + name
	}
	return name
}

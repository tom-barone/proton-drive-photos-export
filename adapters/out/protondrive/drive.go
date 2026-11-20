// Package protondrive downloads and decrypts photos from the Proton API.
package protondrive

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/ProtonMail/go-proton-api"
	"github.com/ProtonMail/gopenpgp/v2/crypto"

	"github.com/tom-barone/proton-drive-photos-export/core"
	"github.com/tom-barone/proton-drive-photos-export/core/ports/out"
)

const (
	// The API rejects requests without an app version.
	// We'll use rclone's version string, since it is a well-known third-party client.
	appVersion = "external-drive-rclone@1.0.0-stable"

	// The Photos section share type; go-proton-api's constants stop at 3.
	shareTypePhotos = proton.ShareType(4)

	blockPageSize = 50
)

// Drive holds the Proton session. Sign-in happens in steps: SignIn
// (+ SubmitTOTP if needed) followed by FinishSignIn.
type Drive struct {
	m  *proton.Manager
	st *authState

	client        *proton.Client
	auth          proton.Auth
	saltedKeyPass []byte

	// filled by ListPhotos, used by Stat and Fetch
	share   proton.Share
	shareKR *crypto.KeyRing
	addrKR  *crypto.KeyRing // all address keys, for signature verification

	mu    sync.Mutex
	krs   map[string]*crypto.KeyRing // linkID -> node keyring
	links map[string]proton.Link     // Stat's cache, consumed by Fetch
}

var _ out.Drive = (*Drive)(nil)

func New() *Drive {
	return &Drive{
		m:     proton.New(proton.WithAppVersion(appVersion)),
		st:    &authState{},
		krs:   map[string]*crypto.KeyRing{},
		links: map[string]proton.Link{},
	}
}

// wrapErr marks API errors as safe to show the user verbatim.
func wrapErr(err *error) {
	var apiErr *proton.APIError
	if errors.As(*err, &apiErr) {
		*err = &core.UserError{Msg: apiErr.Message, Err: *err}
	}
}

func (d *Drive) SignIn(ctx context.Context, email, password string) (needTOTP, needMailboxPass bool, err error) {
	defer wrapErr(&err)
	// A retry starts a new login; revoke the old session so it does not
	// stay live on the server.
	if err := d.SignOut(ctx); err != nil {
		log.Printf("revoking previous session: %v", err)
	}
	client, auth, err := d.m.NewClientWithLogin(ctx, email, []byte(password))
	if err != nil {
		return false, false, err
	}
	d.client, d.auth = client, auth
	return auth.TwoFA.Enabled&proton.HasTOTP != 0,
		auth.PasswordMode == proton.TwoPasswordMode, nil
}

// SignOut revokes the session on the server. Safe to call when not
// signed in, or twice. The client is kept so download workers still
// winding down never see a nil client.
func (d *Drive) SignOut(ctx context.Context) (err error) {
	defer wrapErr(&err)
	if d.auth.UID == "" {
		return nil
	}
	d.auth = proton.Auth{}
	return d.client.AuthDelete(ctx)
}

func (d *Drive) SubmitTOTP(ctx context.Context, code string) (err error) {
	defer wrapErr(&err)
	return d.client.Auth2FA(ctx, proton.Auth2FAReq{TwoFactorCode: code})
}

// FinishSignIn derives the salted key password and starts tracking the
// tokens the raw photos-endpoint calls need.
func (d *Drive) FinishSignIn(ctx context.Context, keyPass []byte) (err error) {
	defer wrapErr(&err)
	u, err := d.client.GetUser(ctx)
	if err != nil {
		return err
	}
	salts, err := d.client.GetSalts(ctx)
	if err != nil {
		return err
	}
	saltedKeyPass, err := salts.SaltForKey(keyPass, u.Keys.Primary().ID)
	if err != nil || len(saltedKeyPass) == 0 {
		return fmt.Errorf("salting key password: %v", err)
	}
	d.saltedKeyPass = saltedKeyPass
	d.st.set(d.auth.UID, d.auth.AccessToken)
	d.client.AddAuthHandler(func(a proton.Auth) {
		d.st.set(a.UID, a.AccessToken)
	})
	return nil
}

// ListPhotos unlocks the account keys, finds the photos share and lists
// everything to download.
func (d *Drive) ListPhotos(ctx context.Context) (jobs []out.Job, usedBytes int64, err error) {
	defer wrapErr(&err)
	addrKRs, allAddrKR, err := unlock(ctx, d.client, d.saltedKeyPass)
	if err != nil {
		return nil, 0, fmt.Errorf("unlocking keys: %w", err)
	}
	share, shareKR, err := photosShare(ctx, d.client, addrKRs)
	if err != nil {
		return nil, 0, err
	}
	d.share, d.shareKR, d.addrKR = share, shareKR, allAddrKR

	photos, err := listPhotos(ctx, d.st, d.client, proton.DefaultHostURL, share.VolumeID)
	if err != nil {
		return nil, 0, fmt.Errorf("listing photos: %w", err)
	}
	vol, err := d.client.GetVolume(ctx, share.VolumeID)
	if err != nil {
		return nil, 0, err
	}
	return flatten(photos, 0), vol.UsedSpace, nil
}

func (d *Drive) Stat(ctx context.Context, linkID string) (meta out.FileMeta, err error) {
	defer wrapErr(&err)
	link, err := d.client.GetLink(ctx, d.share.ShareID, linkID)
	if err != nil {
		return out.FileMeta{}, err
	}
	if link.State != proton.LinkStateActive || link.Type != proton.LinkTypeFile {
		return out.FileMeta{}, nil
	}
	parentKR, err := d.nodeKR(ctx, link.ParentLinkID)
	if err != nil {
		return out.FileMeta{}, err
	}
	name, err := link.GetName(parentKR, nil)
	if err != nil {
		return out.FileMeta{}, fmt.Errorf("decrypting name: %w", err)
	}
	d.mu.Lock()
	d.links[linkID] = link
	d.mu.Unlock()
	return out.FileMeta{Name: name, Created: time.Unix(link.CreateTime, 0), Active: true}, nil
}

// Fetch downloads, verifies and decrypts all blocks of the file's
// active revision, streaming the plaintext to w.
func (d *Drive) Fetch(ctx context.Context, linkID string, w io.Writer) (err error) {
	defer wrapErr(&err)
	d.mu.Lock()
	link, ok := d.links[linkID]
	delete(d.links, linkID)
	d.mu.Unlock()
	if !ok {
		if link, err = d.client.GetLink(ctx, d.share.ShareID, linkID); err != nil {
			return err
		}
	}

	nodeKR, err := d.nodeKR(ctx, linkID)
	if err != nil {
		return err
	}
	sk, err := link.GetSessionKey(nodeKR)
	if err != nil {
		return fmt.Errorf("decrypting session key: %w", err)
	}

	for from := 1; ; from += blockPageSize {
		rev, err := d.client.GetRevision(ctx, d.share.ShareID, linkID, link.FileProperties.ActiveRevision.ID, from, blockPageSize)
		if err != nil {
			return err
		}
		for _, b := range rev.Blocks {
			if err := d.writeBlock(ctx, w, sk, b); err != nil {
				return fmt.Errorf("block %d: %w", b.Index, err)
			}
		}
		if len(rev.Blocks) < blockPageSize {
			return nil
		}
	}
}

// nodeKR resolves a link's node keyring by walking up to the share root,
// caching every keyring on the way.
func (d *Drive) nodeKR(ctx context.Context, linkID string) (*crypto.KeyRing, error) {
	if linkID == "" {
		return d.shareKR, nil
	}
	d.mu.Lock()
	kr, ok := d.krs[linkID]
	d.mu.Unlock()
	if ok {
		return kr, nil
	}

	link, err := d.client.GetLink(ctx, d.share.ShareID, linkID)
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

func (d *Drive) writeBlock(ctx context.Context, w io.Writer, sk *crypto.SessionKey, b proton.Block) error {
	rc, err := d.client.GetBlock(ctx, b.BareURL, b.Token)
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
	_, err = w.Write(msg.GetBinary())
	return err
}

func unlock(ctx context.Context, client *proton.Client, saltedKeyPass []byte) (map[string]*crypto.KeyRing, *crypto.KeyRing, error) {
	u, err := client.GetUser(ctx)
	if err != nil {
		return nil, nil, err
	}
	addrs, err := client.GetAddresses(ctx)
	if err != nil {
		return nil, nil, err
	}
	_, addrKRs, err := proton.Unlock(u, addrs, saltedKeyPass, nil)
	if err != nil {
		return nil, nil, err
	}

	// Node signatures can come from any of the account's addresses, so
	// verification needs a keyring holding every address key.
	all, err := crypto.NewKeyRing(nil)
	if err != nil {
		return nil, nil, err
	}
	for _, kr := range addrKRs {
		for _, key := range kr.GetKeys() {
			if err := all.AddKey(key); err != nil {
				return nil, nil, err
			}
		}
	}
	return addrKRs, all, nil
}

func photosShare(ctx context.Context, client *proton.Client, addrKRs map[string]*crypto.KeyRing) (proton.Share, *crypto.KeyRing, error) {
	shares, err := client.ListShares(ctx, true)
	if err != nil {
		return proton.Share{}, nil, err
	}
	for _, meta := range shares {
		if meta.Type != shareTypePhotos || meta.State != proton.ShareStateActive {
			continue
		}
		share, err := client.GetShare(ctx, meta.ShareID)
		if err != nil {
			return proton.Share{}, nil, err
		}
		addrKR, ok := addrKRs[share.AddressID]
		if !ok {
			return proton.Share{}, nil, fmt.Errorf("no keyring for share address %s", share.AddressID)
		}
		kr, err := share.GetKeyRing(addrKR)
		return share, kr, err
	}
	return proton.Share{}, nil, errors.New("no photos share found; does the account have a Photos section?")
}

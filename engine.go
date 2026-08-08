// AI GENERATED - Tom Barone
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/ProtonMail/go-proton-api"
	"github.com/ProtonMail/gopenpgp/v2/crypto"
)

const (
	// The API rejects requests without a known app version.
	appVersion = "macos-drive@1.0.0-alpha.1"

	// The Photos section share type; go-proton-api's constants stop at 3.
	shareTypePhotos = proton.ShareType(4)
)

// session is cached to disk so re-launches skip the password/TOTP screens.
// SaltedKeyPass must be cached too: salts are only retrievable right
// after a password login.
type session struct {
	UID           string
	AccessToken   string
	RefreshToken  string
	SaltedKeyPass []byte
}

// authState holds the tokens used by the raw photos-endpoint calls,
// kept current by the client's auth refresh handler.
type authState struct {
	mu  sync.Mutex
	uid string
	acc string
}

func (st *authState) set(uid, acc string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if uid != "" {
		st.uid = uid
	}
	st.acc = acc
}

func (st *authState) get() (uid, acc string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.uid, st.acc
}

// engine drives the Proton API. Sign-in happens in steps so the UI can
// show separate screens: restoreSession, or signIn (+ submitTOTP if
// needed) followed by finishSignIn.
type engine struct {
	m        *proton.Manager
	sessPath string

	client        *proton.Client
	st            *authState
	auth          proton.Auth
	pass          []byte // held between signIn and finishSignIn
	saltedKeyPass []byte

	// filled by scan, used by download
	share   proton.Share
	shareKR *crypto.KeyRing
	addrKR  *crypto.KeyRing
}

func newEngine() *engine {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	dir = filepath.Join(dir, "ProtonPhotoExport")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Printf("creating config dir: %v", err)
	}
	return &engine{
		m:        proton.New(proton.WithAppVersion(appVersion)),
		sessPath: filepath.Join(dir, "session.json"),
		st:       &authState{},
	}
}

// restoreSession signs in with the cached session file, if any.
func (e *engine) restoreSession(ctx context.Context) error {
	s, err := loadSession(e.sessPath)
	if err != nil {
		return err
	}
	client, auth, err := e.m.NewClientWithRefresh(ctx, s.UID, s.RefreshToken)
	if err != nil {
		return err
	}
	s.AccessToken, s.RefreshToken = auth.AccessToken, auth.RefreshToken
	e.client, e.saltedKeyPass = client, s.SaltedKeyPass
	e.finishLogin(s)
	return nil
}

// signIn starts a fresh login and reports which extra steps the UI must
// run before finishSignIn.
func (e *engine) signIn(ctx context.Context, email, password string) (needTOTP, needMailboxPass bool, err error) {
	client, auth, err := e.m.NewClientWithLogin(ctx, email, []byte(password))
	if err != nil {
		return false, false, err
	}
	e.client, e.auth, e.pass = client, auth, []byte(password)
	return auth.TwoFA.Enabled&proton.HasTOTP != 0,
		auth.PasswordMode == proton.TwoPasswordMode, nil
}

func (e *engine) submitTOTP(ctx context.Context, code string) error {
	return e.client.Auth2FA(ctx, proton.Auth2FAReq{TwoFactorCode: code})
}

// finishSignIn derives the salted key password and persists the session.
// mailboxPass is empty in single-password mode.
func (e *engine) finishSignIn(ctx context.Context, mailboxPass string) error {
	keyPass := e.pass
	if mailboxPass != "" {
		keyPass = []byte(mailboxPass)
	}
	u, err := e.client.GetUser(ctx)
	if err != nil {
		return err
	}
	salts, err := e.client.GetSalts(ctx)
	if err != nil {
		return err
	}
	saltedKeyPass, err := salts.SaltForKey(keyPass, u.Keys.Primary().ID)
	if err != nil || len(saltedKeyPass) == 0 {
		return fmt.Errorf("salting key password: %v", err)
	}
	e.pass = nil
	e.saltedKeyPass = saltedKeyPass
	e.finishLogin(session{e.auth.UID, e.auth.AccessToken, e.auth.RefreshToken, saltedKeyPass})
	return nil
}

func (e *engine) finishLogin(s session) {
	e.st.set(s.UID, s.AccessToken)
	saveSession(e.sessPath, s)
	e.client.AddAuthHandler(func(a proton.Auth) {
		if a.UID != "" {
			s.UID = a.UID
		}
		s.AccessToken, s.RefreshToken = a.AccessToken, a.RefreshToken
		e.st.set(s.UID, s.AccessToken)
		saveSession(e.sessPath, s)
	})
}

type scanResult struct {
	jobs      []job
	usedBytes int64
}

// scan unlocks the account keys, finds the photos share and lists
// everything to download. usedBytes is the photo volume's total size,
// for display only.
func (e *engine) scan(ctx context.Context) (scanResult, error) {
	addrKRs, allAddrKR, err := unlock(ctx, e.client, e.saltedKeyPass)
	if err != nil {
		return scanResult{}, fmt.Errorf("unlocking keys: %w", err)
	}
	share, shareKR, err := photosShare(ctx, e.client, addrKRs)
	if err != nil {
		return scanResult{}, err
	}
	e.share, e.shareKR, e.addrKR = share, shareKR, allAddrKR

	photos, err := listPhotos(ctx, e.st, e.client, proton.DefaultHostURL, share.VolumeID)
	if err != nil {
		return scanResult{}, fmt.Errorf("listing photos: %w", err)
	}
	vol, err := e.client.GetVolume(ctx, share.VolumeID)
	if err != nil {
		return scanResult{}, err
	}
	return scanResult{flatten(photos, 0), vol.UsedSpace}, nil
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

func loadSession(path string) (session, error) {
	var s session
	b, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, err
	}
	if s.UID == "" || s.RefreshToken == "" || len(s.SaltedKeyPass) == 0 {
		return s, errors.New("incomplete session file")
	}
	return s, nil
}

func saveSession(path string, s session) {
	b, err := json.Marshal(s)
	if err == nil {
		err = os.WriteFile(path, b, 0o600)
	}
	if err != nil {
		log.Printf("saving session: %v", err)
	}
}

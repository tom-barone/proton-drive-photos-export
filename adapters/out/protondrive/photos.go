// AI GENERATED - Tom Barone
package protondrive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/ProtonMail/go-proton-api"

	"github.com/tom-barone/proton-drive-photos-export/core/ports/out"
)

const photoPageSize = 500

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

type photo struct {
	LinkID        string
	CaptureTime   int64
	RelatedPhotos []photo
}

// listPhotos pages through the photo listing endpoint
// (GET /drive/volumes/{id}/photos), which go-proton-api does not implement.
func listPhotos(ctx context.Context, st *authState, client *proton.Client, baseURL, volumeID string) ([]photo, error) {
	var all []photo
	cursor := ""
	for {
		page, err := photosPage(ctx, st, client, baseURL, volumeID, cursor)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < photoPageSize {
			return all, nil
		}
		cursor = page[len(page)-1].LinkID
	}
}

func photosPage(ctx context.Context, st *authState, client *proton.Client, baseURL, volumeID, cursor string) ([]photo, error) {
	q := url.Values{"PageSize": {fmt.Sprint(photoPageSize)}}
	if cursor != "" {
		q.Set("PreviousPageLastLinkID", cursor)
	}
	reqURL := fmt.Sprintf("%s/drive/volumes/%s/photos?%s", baseURL, volumeID, q.Encode())

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, err
		}
		uid, acc := st.get()
		req.Header.Set("x-pm-uid", uid)
		req.Header.Set("Authorization", "Bearer "+acc)
		req.Header.Set("x-pm-appversion", appVersion)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		// The upstream client refreshes expired tokens itself and notifies
		// our auth handler; piggyback on that by making any client call,
		// then retry with the updated tokens.
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 && client != nil {
			if _, err := client.GetUser(ctx); err != nil {
				return nil, err
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GET %s: %s: %s", reqURL, resp.Status, body)
		}

		var res struct{ Photos []photo }
		if err := json.Unmarshal(body, &res); err != nil {
			return nil, err
		}
		return res.Photos, nil
	}
}

// flatten turns the photo listing into a flat download list, including
// related links (live photo videos, bursts), truncated to limit if > 0.
func flatten(photos []photo, limit int) []out.Job {
	var jobs []out.Job
	for _, p := range photos {
		jobs = append(jobs, toJob(p.LinkID, p.CaptureTime))
		for _, r := range p.RelatedPhotos {
			ct := r.CaptureTime
			if ct == 0 {
				ct = p.CaptureTime
			}
			jobs = append(jobs, toJob(r.LinkID, ct))
		}
	}
	if limit > 0 && len(jobs) > limit {
		jobs = jobs[:limit]
	}
	return jobs
}

// toJob maps a capture time of 0 to the zero time, marking the capture
// time unknown.
func toJob(linkID string, captureTime int64) out.Job {
	j := out.Job{LinkID: linkID}
	if captureTime != 0 {
		j.Taken = time.Unix(captureTime, 0)
	}
	return j
}

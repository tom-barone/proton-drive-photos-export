// AI GENERATED - Tom Barone
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func photosServer(t *testing.T, pages map[string][]photo) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cursor := r.URL.Query().Get("PreviousPageLastLinkID")
		if err := json.NewEncoder(w).Encode(struct{ Photos []photo }{pages[cursor]}); err != nil {
			t.Error(err)
		}
	}))
}

// Given a volume with fewer photos than one page
// When listPhotos runs
// Then it returns all photos from the single request
func TestListPhotosSinglePage(t *testing.T) {
	srv := photosServer(t, map[string][]photo{"": {{LinkID: "a"}, {LinkID: "b"}}})
	defer srv.Close()

	got, err := listPhotos(context.Background(), &authState{}, nil, srv.URL, "vol")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d photos, want 2", len(got))
	}
}

// Given a volume with more photos than one page
// When listPhotos runs
// Then it follows the cursor and returns all pages combined
func TestListPhotosPaginates(t *testing.T) {
	full := make([]photo, photoPageSize)
	for i := range full {
		full[i] = photo{LinkID: fmt.Sprintf("p%d", i)}
	}
	last := full[len(full)-1].LinkID
	srv := photosServer(t, map[string][]photo{"": full, last: {{LinkID: "extra"}}})
	defer srv.Close()

	got, err := listPhotos(context.Background(), &authState{}, nil, srv.URL, "vol")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != photoPageSize+1 {
		t.Fatalf("got %d photos, want %d", len(got), photoPageSize+1)
	}
}

// Given a server that rejects the tokens
// When listPhotos runs without a client to refresh them
// Then it returns an error
func TestListPhotosUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	if _, err := listPhotos(context.Background(), &authState{}, nil, srv.URL, "vol"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// Given photos with related links
// When flatten runs
// Then the related links appear in the download list
func TestFlattenIncludesRelated(t *testing.T) {
	jobs := flatten([]photo{{LinkID: "a", RelatedPhotos: []photo{{LinkID: "a-video"}}}}, 0)
	if len(jobs) != 2 || jobs[1].linkID != "a-video" {
		t.Fatalf("got %v, want [a a-video]", jobs)
	}
}

// Given a related link without its own capture time
// When flatten runs
// Then the related link inherits the parent's capture time
func TestFlattenRelatedInheritsCaptureTime(t *testing.T) {
	jobs := flatten([]photo{{LinkID: "a", CaptureTime: 42, RelatedPhotos: []photo{{LinkID: "b"}}}}, 0)
	if jobs[1].captureTime != 42 {
		t.Fatalf("got capture time %d, want 42", jobs[1].captureTime)
	}
}

// Given more items than the limit
// When flatten runs with a limit
// Then the download list is truncated to the limit
func TestFlattenLimit(t *testing.T) {
	jobs := flatten([]photo{{LinkID: "a"}, {LinkID: "b"}, {LinkID: "c"}}, 2)
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
}

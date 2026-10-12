package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// A stream token is minted for ONE media item (a cast, a SyncPlay guest, a
// player URL). Presented on another item's route it must be refused before
// anything is looked up: otherwise it opens every title its owner can reach.
// Neutralising the check on these five routes left the package green
// (sabotage sweep, 2026-09-28); each case would answer something other than
// 403 without it.
func TestStreamTokenScopedToAnotherItem_Forbidden(t *testing.T) {
	item := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/tmp/absent.mkv"}}
	cases := map[string]struct {
		h    http.HandlerFunc
		path string
		vals map[string]string
	}{
		"media stream": {
			h:    mediaHandler(&fakeMediaItemGetter{item: item}, nil, "http://localhost", allowAll()),
			path: "/api/v1/media/m1/stream", vals: map[string]string{"id": "m1"},
		},
		"subtitle stream": {
			h:    subtitleHandler(&fakeMediaStreamsGetter{item: item, sErr: errTest}, "http://localhost", allowAll(), nil, subtitleCache{}),
			path: "/api/v1/media/m1/subtitle/0", vals: map[string]string{"id": "m1", "track": "0"},
		},
		"playback-info": {
			h:    playbackInfoHandler(nil, &fakeMediaStreamsGetter{item: item}, allowAll(), nil, nil),
			path: "/api/v1/media/m1/playback-info", vals: map[string]string{"id": "m1"},
		},
		"trickplay manifest": {
			h:    trickplayManifestHandler(&fakeTrickplayGetter{}, "http://localhost", allowAll(), &fakeMediaItemGetter{item: item}, allowAllParental{}, nil),
			path: "/api/v1/media/m1/trickplay", vals: map[string]string{"id": "m1"},
		},
		"trickplay image": {
			h:    trickplayImageHandler(&fakeTrickplayGetter{}, "http://localhost", allowAll(), &fakeMediaItemGetter{item: item}, allowAllParental{}),
			path: "/api/v1/media/m1/trickplay/0.jpg", vals: map[string]string{"id": "m1", "image": "0.jpg"},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, c.path, nil)
			for k, v := range c.vals {
				req.SetPathValue(k, v)
			}
			req = withHTTPClaims(req, testClaims(false))
			req = req.WithContext(context.WithValue(req.Context(), streamTokenCtxKey, &shared.StreamToken{Token: "t", MediaItemID: "m-other"}))
			w := httptest.NewRecorder()
			c.h.ServeHTTP(w, req)
			if w.Code != http.StatusForbidden {
				t.Errorf("a token minted for another item must be refused with 403, got %d", w.Code)
			}
		})
	}
}

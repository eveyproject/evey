package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/evey-project/evey/internal/domain/shared"
)

// A syncplay guest holds a media-scoped stream token minted for watching along;
// it may stream, never download - the raw download already refused it, the
// compat and reduced variants let it through.
func withGuestToken(r *http.Request, mediaID string) *http.Request {
	st := &shared.StreamToken{MediaItemID: shared.MediaItemID(mediaID), Scope: shared.ScopeSyncplayGuest}
	return r.WithContext(context.WithValue(r.Context(), streamTokenCtxKey, st))
}

func TestCompatDownload_RefusesASyncplayGuestToken(t *testing.T) {
	h := newTestCompatServe(t, &fakeCompatTranscoder{payload: []byte("x")}, allowAll())
	for _, serve := range []http.HandlerFunc{h.handlePrepare, h.handleServe, h.handleStatus} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/compat-download", nil)
		req.SetPathValue("id", "m1")
		w := httptest.NewRecorder()
		serve(w, withGuestToken(req, "m1"))
		assert.Equal(t, http.StatusForbidden, w.Code)
	}
}

func TestReducedDownload_RefusesASyncplayGuestToken(t *testing.T) {
	h := newTestReducedServe(t, newFakeReducedTranscoder(3), allowAll(), nil)
	for _, serve := range []http.HandlerFunc{h.handlePlan, h.handlePrepare, h.handleStatus} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/reduced-download", nil)
		req.SetPathValue("id", "m1")
		w := httptest.NewRecorder()
		serve(w, withGuestToken(req, "m1"))
		assert.Equal(t, http.StatusForbidden, w.Code)
	}
}

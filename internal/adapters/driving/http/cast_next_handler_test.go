package httpapi

// cast-next is the receiver's self-service "what do I play after this
// episode" door. Its caller holds NOTHING but a cast stream token, so the
// token IS the whole identity: every test here exercises the trust boundary
// (token kind, media pinning, per-item access on the NEXT episode) that a
// JWT route would get from claims middleware.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appmedia "github.com/eveyproject/evey/internal/app/media"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

func castToken(mediaID string) *shared.StreamToken {
	return &shared.StreamToken{UserID: "u1", ProfileID: "p1", MediaItemID: shared.MediaItemID(mediaID), Cast: true}
}

func doCastNextRequest(h *hlsVODServe, mediaID string, st *shared.StreamToken) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/"+mediaID+"/cast-next", nil)
	req.SetPathValue("id", mediaID)
	if st != nil {
		req = req.WithContext(context.WithValue(req.Context(), streamTokenCtxKey, st))
	}
	w := httptest.NewRecorder()
	h.serveCastNext(w, req)
	return w
}

// newCastNextHandler builds the handler with a live token store so the happy
// path can prove the minted token is real (validates against the store).
func newCastNextHandler(t *testing.T, libSvc MediaStreamsGetter, access LibraryAccessChecker, parental ParentalChecker) (*hlsVODServe, *appmedia.StreamTokenStore) {
	t.Helper()
	h, _ := buildVODHandler(t, libSvc, access, parental)
	store := appmedia.NewStreamTokenStore(appmedia.DefaultStreamTokenTTL)
	h.streamTokens = store
	return h, store
}

func TestCastNext_RefusesWithoutStreamToken(t *testing.T) {
	cur := newEpisode("ep-a", "lib-a")
	libSvc := &prewarmStreamsGetter{items: map[string]*domainlibrary.MediaItem{"ep-a": cur}}
	h, _ := newCastNextHandler(t, libSvc, &libIDAccessChecker{allowed: map[string]bool{"lib-a": true}}, testFakeParentalChecker{})

	w := doCastNextRequest(h, "ep-a", nil)
	assert.Equal(t, http.StatusForbidden, w.Code, "a JWT-only caller has richer routes; this door is receiver-only")
}

func TestCastNext_RefusesNonCastToken(t *testing.T) {
	cur := newEpisode("ep-a", "lib-a")
	libSvc := &prewarmStreamsGetter{items: map[string]*domainlibrary.MediaItem{"ep-a": cur}}
	h, _ := newCastNextHandler(t, libSvc, &libIDAccessChecker{allowed: map[string]bool{"lib-a": true}}, testFakeParentalChecker{})

	st := castToken("ep-a")
	st.Cast = false
	w := doCastNextRequest(h, "ep-a", st)
	assert.Equal(t, http.StatusForbidden, w.Code, "a plain stream token must not mint tokens for other media")
}

func TestCastNext_RefusesTokenScopedToOtherMedia(t *testing.T) {
	cur := newEpisode("ep-a", "lib-a")
	libSvc := &prewarmStreamsGetter{items: map[string]*domainlibrary.MediaItem{"ep-a": cur}}
	h, _ := newCastNextHandler(t, libSvc, &libIDAccessChecker{allowed: map[string]bool{"lib-a": true}}, testFakeParentalChecker{})

	w := doCastNextRequest(h, "ep-a", castToken("ep-other"))
	assert.Equal(t, http.StatusForbidden, w.Code, "the token is pinned to its own media, not a pivot")
}

func TestCastNext_SeriesFinale_NoContent(t *testing.T) {
	cur := newEpisode("ep-a", "lib-a")
	libSvc := &prewarmStreamsGetter{items: map[string]*domainlibrary.MediaItem{"ep-a": cur}, next: nil}
	h, _ := newCastNextHandler(t, libSvc, &libIDAccessChecker{allowed: map[string]bool{"lib-a": true}}, testFakeParentalChecker{})

	w := doCastNextRequest(h, "ep-a", castToken("ep-a"))
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestCastNext_MovieCurrent_NoContent(t *testing.T) {
	cur := newEpisode("mov-a", "lib-a")
	cur.Type = "movie"
	libSvc := &prewarmStreamsGetter{items: map[string]*domainlibrary.MediaItem{"mov-a": cur}}
	h, _ := newCastNextHandler(t, libSvc, &libIDAccessChecker{allowed: map[string]bool{"lib-a": true}}, testFakeParentalChecker{})

	w := doCastNextRequest(h, "mov-a", castToken("mov-a"))
	assert.Equal(t, http.StatusNoContent, w.Code)
}

// Same IDOR shape as prewarm-next: the current episode's access does not
// imply the next one (series straddling libraries).
func TestCastNext_CrossLibraryNext_NotFound(t *testing.T) {
	cur := newEpisode("ep-a", "lib-a")
	next := newEpisode("ep-b", "lib-b")
	libSvc := &prewarmStreamsGetter{items: map[string]*domainlibrary.MediaItem{"ep-a": cur, "ep-b": next}, next: next}
	h, _ := newCastNextHandler(t, libSvc, &libIDAccessChecker{allowed: map[string]bool{"lib-a": true}}, testFakeParentalChecker{})

	w := doCastNextRequest(h, "ep-a", castToken("ep-a"))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCastNext_ParentalBlockedNext_NotFound(t *testing.T) {
	cur := newEpisode("ep-a", "lib-a")
	next := newEpisode("ep-b", "lib-a")
	libSvc := &prewarmStreamsGetter{items: map[string]*domainlibrary.MediaItem{"ep-a": cur, "ep-b": next}, next: next}
	parental := &blockMediaParental{blocked: map[string]bool{"ep-b": true}}
	h, _ := newCastNextHandler(t, libSvc, &libIDAccessChecker{allowed: map[string]bool{"lib-a": true}}, parental)

	w := doCastNextRequest(h, "ep-a", castToken("ep-a"))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// Fail-closed: a handler wired without a token store cannot answer - it must
// refuse rather than reply without a playable token.
func TestCastNext_NoTokenStore_Unavailable(t *testing.T) {
	cur := newEpisode("ep-a", "lib-a")
	next := newEpisode("ep-b", "lib-a")
	libSvc := &prewarmStreamsGetter{items: map[string]*domainlibrary.MediaItem{"ep-a": cur, "ep-b": next}, next: next}
	h, _ := buildVODHandler(t, libSvc, &libIDAccessChecker{allowed: map[string]bool{"lib-a": true}}, testFakeParentalChecker{})

	w := doCastNextRequest(h, "ep-a", castToken("ep-a"))
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestCastNext_HappyPath_MintsCastTokenForNext(t *testing.T) {
	cur := newEpisode("ep-a", "lib-a")
	next := newEpisode("ep-b", "lib-a")
	epIdx := 5
	next.IndexNumber = &epIdx
	next.Title = "Some Episode"
	next.ParentID = "season-2"
	seasonIdx := 2
	season := &domainlibrary.MediaItem{ID: "season-2", Type: "season", IndexNumber: &seasonIdx, ParentID: "series-1"}
	series := &domainlibrary.MediaItem{ID: "series-1", Type: "series", Title: "Some Series"}
	libSvc := &prewarmStreamsGetter{
		items: map[string]*domainlibrary.MediaItem{"ep-a": cur, "ep-b": next, "season-2": season, "series-1": series},
		next:  next,
	}
	h, store := newCastNextHandler(t, libSvc, &libIDAccessChecker{allowed: map[string]bool{"lib-a": true}}, testFakeParentalChecker{})

	w := doCastNextRequest(h, "ep-a", castToken("ep-a"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		NextMediaItemID string `json:"next_media_item_id"`
		Token           string `json:"token"`
		Title           string `json:"title"`
		SeasonNumber    int    `json:"season_number"`
		EpisodeNumber   int    `json:"episode_number"`
		SeriesTitle     string `json:"series_title"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ep-b", resp.NextMediaItemID)
	assert.Equal(t, "Some Episode", resp.Title)
	assert.Equal(t, 2, resp.SeasonNumber)
	assert.Equal(t, 5, resp.EpisodeNumber)
	assert.Equal(t, "Some Series", resp.SeriesTitle)

	minted, ok := store.Validate(resp.Token, "ep-b")
	require.True(t, ok, "the minted token must be live and scoped to the NEXT episode")
	assert.True(t, minted.Cast, "the minted token must stay a cast token (progress tracking keys on it)")
	assert.Equal(t, shared.UserID("u1"), minted.UserID)
	assert.Equal(t, shared.ProfileID("p1"), minted.ProfileID)
}

package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appmusic "github.com/eveyproject/evey/internal/app/music"
	domainmusic "github.com/eveyproject/evey/internal/domain/music"
	musicfakes "github.com/eveyproject/evey/internal/domain/music/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/auth"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePlayRepository implements music.PlayRepository for recordPlay tests.
type fakePlayRepository struct {
	lastProfileID string
	lastTrackID   string
	err           error
	callCount     int
}

func (f *fakePlayRepository) IncrementPlay(_ context.Context, profileID, trackID string) error {
	f.callCount++
	f.lastProfileID = profileID
	f.lastTrackID = trackID
	return f.err
}

func (f *fakePlayRepository) PlayCountsForProfile(_ context.Context, _ string) (map[string]int, error) {
	return nil, nil
}

// newRecordPlayMux builds a mux with the music routes registered.
// The token includes ProfileID so recordPlay can attribute the play.
func newRecordPlayMux(t *testing.T, checker LibraryAccessChecker, repo *musicfakes.FakeMusicTrackRepository, plays *fakePlayRepository, profileID string) (*http.ServeMux, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tokenGen := auth.NewJWTGenerator(key, time.Minute)
	token, err := tokenGen.GenerateAccessToken(domainuser.Claims{
		UserID:    "user-1",
		ProfileID: profileID,
		IsAdmin:   false,
	})
	require.NoError(t, err)
	bus := sharedfakes.NewFakeEventBus()
	svc := appmusic.NewService(repo, bus, nil, musicfakes.NewFakeMusicScanner())
	if plays != nil {
		svc.WithPlayRepository(plays)
	}
	mux := http.NewServeMux()
	registerMusicRoutes(mux, svc, tokenGen, checker, nil, ratelimit.New(10000), nil, nil, nil)
	return mux, token
}

// seedTrack inserts a track into the fake repo and returns its ID.
func seedTrack(t *testing.T, repo *musicfakes.FakeMusicTrackRepository, id, libID string) {
	t.Helper()
	_, err := repo.Upsert(context.Background(), domainmusic.Track{
		ID:        id,
		LibraryID: shared.LibraryID(libID),
		Title:     "Test Track",
		FilePath:  "/fake/audio.mp3",
	})
	require.NoError(t, err)
}

// --- tests: recordPlay ---

// Happy path: authenticated user with a profile - 204 returned and play recorded.
func TestRecordPlay_HappyPath_204(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	seedTrack(t, repo, "t1", "lib-1")
	plays := &fakePlayRepository{}

	mux, token := newRecordPlayMux(t, allowAll(), repo, plays, "prof-1")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/music/tracks/t1/played", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, 1, plays.callCount)
	assert.Equal(t, "prof-1", plays.lastProfileID)
	assert.Equal(t, "t1", plays.lastTrackID)
}

// No profile ID in claims: 204 returned, play NOT recorded (graceful no-op).
func TestRecordPlay_NoProfileID_NoOp_204(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	seedTrack(t, repo, "t2", "lib-1")
	plays := &fakePlayRepository{}

	// profileID="" → handler short-circuits before RecordPlay
	mux, token := newRecordPlayMux(t, allowAll(), repo, plays, "")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/music/tracks/t2/played", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, 0, plays.callCount)
}

// Track not found → 404.
func TestRecordPlay_TrackNotFound_404(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	plays := &fakePlayRepository{}

	mux, token := newRecordPlayMux(t, allowAll(), repo, plays, "prof-1")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/music/tracks/nonexistent/played", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, 0, plays.callCount)
}

// Library access denied → 403, play NOT recorded.
func TestRecordPlay_LibraryAccessDenied_403(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	seedTrack(t, repo, "t3", "lib-secret")
	plays := &fakePlayRepository{}

	mux, token := newRecordPlayMux(t, denyAll(), repo, plays, "prof-1")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/music/tracks/t3/played", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, 0, plays.callCount)
}

// PlayRepository.IncrementPlay returns an error → 500.
func TestRecordPlay_ServiceError_500(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	seedTrack(t, repo, "t4", "lib-1")
	plays := &fakePlayRepository{err: errors.New("db down")}

	mux, token := newRecordPlayMux(t, allowAll(), repo, plays, "prof-1")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/music/tracks/t4/played", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, 1, plays.callCount)
}

// No auth header → 401.
func TestRecordPlay_NoAuth_401(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	seedTrack(t, repo, "t5", "lib-1")

	mux, _ := newRecordPlayMux(t, allowAll(), repo, nil, "prof-1")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/music/tracks/t5/played", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// Nil play repository (no WithPlayRepository called): 204, graceful no-op.
func TestRecordPlay_NilPlayRepo_NoOp_204(t *testing.T) {
	repo := musicfakes.NewFakeMusicTrackRepository()
	seedTrack(t, repo, "t6", "lib-1")

	// plays=nil → svc.plays is nil → RecordPlay is a no-op
	mux, token := newRecordPlayMux(t, allowAll(), repo, nil, "prof-1")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/music/tracks/t6/played", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

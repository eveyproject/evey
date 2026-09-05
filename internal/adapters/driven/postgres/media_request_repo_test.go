package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/mediarequest"
	"github.com/evey-project/evey/internal/domain/shared"
)

// The media-request search must flag titles the server already holds, matched
// on provider id ONLY: a title match would flag remakes and same-name releases,
// silently blocking a legitimate request.
func TestLibraryLookupRepo_FindByProviderIDs(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	lookup := postgres.NewLibraryLookupRepository(db)
	ctx := context.Background()

	lib := newTestLibrary("lookuplib")
	require.NoError(t, libRepo.Create(ctx, lib))

	movie := newTestMediaItem("lookupmovie", lib.ID)
	movie.Type = library.MovieItem
	movie.Catalog = &library.MediaCatalog{ProviderIDs: map[string]string{"tmdb": "438631", "imdb": "tt1160419"}}
	require.NoError(t, itemRepo.Create(ctx, movie))

	got, err := lookup.FindByProviderIDs(ctx, "movie", []string{"tmdb:438631", "tmdb:999999"})
	require.NoError(t, err)
	assert.Equal(t, string(movie.ID), got["tmdb:438631"], "the held title resolves to its media item")
	assert.NotContains(t, got, "tmdb:999999", "an unheld id must not be flagged")

	// A second id on the SAME row must not credit a candidate nobody asked about.
	got, err = lookup.FindByProviderIDs(ctx, "movie", []string{"tmdb:438631"})
	require.NoError(t, err)
	assert.Len(t, got, 1)

	// Type-scoped: the same id under the series vocabulary is a different thing.
	got, err = lookup.FindByProviderIDs(ctx, "tv", []string{"tmdb:438631"})
	require.NoError(t, err)
	assert.Empty(t, got, "a movie must not answer a series lookup")

	// Music carries no provider id anywhere - never guess.
	got, err = lookup.FindByProviderIDs(ctx, "music", []string{"musicbrainz:abc"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

// seedRequestOwner creates the user + profile the media_requests foreign keys
// require, and returns the repo under test.
func seedRequestOwner(t *testing.T, db *sql.DB, suffix string) (*postgres.MediaRequestRepository, shared.UserID, shared.ProfileID) {
	t.Helper()
	ctx := context.Background()
	u := newTestUser("mru" + suffix)
	require.NoError(t, postgres.NewUserRepository(db).Create(ctx, u))
	p := newTestProfile("mrp"+suffix, u.ID)
	require.NoError(t, postgres.NewProfileRepository(db).Create(ctx, p))
	return postgres.NewMediaRequestRepository(db), u.ID, p.ID
}

func newTestRequest(id, providerID string, status mediarequest.Status, userID shared.UserID, profileID shared.ProfileID) *mediarequest.Request {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return &mediarequest.Request{
		ID:         id,
		ProfileID:  profileID,
		UserID:     string(userID),
		MediaType:  mediarequest.MediaTypeMovie,
		Provider:   "tmdb",
		ProviderID: providerID,
		Title:      "Some Movie",
		Status:     status,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// The duplicate guard used to be a plain (non-unique) index, with the service
// doing a read-then-write around it and nothing in between. Two concurrent
// calls - a double-click, a client retry - both passed the check and both
// inserted, leaving two identical open requests in the operator's moderation
// queue. This proves the database now refuses the second one, and that the
// loser gets the domain error rather than a raw driver failure.
func TestMediaRequestRepo_DuplicateOpenRequestIsRejected(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	repo, uid, pid := seedRequestOwner(t, db, "dupe")

	require.NoError(t, repo.Create(ctx, newTestRequest("req-1", "900001", mediarequest.StatusPending, uid, pid)))

	err := repo.Create(ctx, newTestRequest("req-2", "900001", mediarequest.StatusPending, uid, pid))
	require.ErrorIs(t, err, mediarequest.ErrDuplicate)
}

// A rejected request must not block asking again - that is what "open" means in
// HasOpenRequest, and the partial index has to agree with it or the two
// disagree about the same rule.
func TestMediaRequestRepo_RejectedDoesNotBlockReRequest(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	repo, uid, pid := seedRequestOwner(t, db, "rej")

	require.NoError(t, repo.Create(ctx, newTestRequest("req-1", "900001", mediarequest.StatusRejected, uid, pid)))
	require.NoError(t, repo.Create(ctx, newTestRequest("req-2", "900001", mediarequest.StatusPending, uid, pid)),
		"a previously rejected title must be requestable again")
}

// Free-text requests carry no provider id, so they have no identity to
// deduplicate on. Constraining them would stop a user filing two different
// hand-typed requests.
func TestMediaRequestRepo_EmptyProviderIDIsNotDeduplicated(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	repo, uid, pid := seedRequestOwner(t, db, "free")

	require.NoError(t, repo.Create(ctx, newTestRequest("req-1", "", mediarequest.StatusPending, uid, pid)))
	require.NoError(t, repo.Create(ctx, newTestRequest("req-2", "", mediarequest.StatusPending, uid, pid)),
		"free-text requests have no identity to deduplicate on")
}

// Requests created in the same burst - a client retry, a seeded batch - share
// their created_at to the millisecond. Ordering by that column alone leaves
// their relative order up to the planner, so a row can show up on two pages or
// on none at all while the operator moderates the queue. The id tiebreaker is
// what makes the order total; this walks two pages of a tied set and pins that
// every request appears exactly once.
func TestMediaRequestRepo_PaginationIsStableOnTiedTimestamps(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	repo, uid, pid := seedRequestOwner(t, db, "tie")

	tied := time.Now().UTC().Truncate(time.Millisecond)
	ids := []string{"req-a", "req-b", "req-c", "req-d"}
	for i, id := range ids {
		req := newTestRequest(id, fmt.Sprintf("tmdb-%d", i), mediarequest.StatusPending, uid, pid)
		req.CreatedAt = tied
		req.UpdatedAt = tied
		require.NoError(t, repo.Create(ctx, req))
	}

	var paged []string
	for offset := 0; offset < len(ids); offset += 2 {
		page, err := repo.ListByProfile(ctx, string(pid), 2, offset)
		require.NoError(t, err)
		for _, r := range page {
			paged = append(paged, r.ID)
		}
	}
	// Asserting the EXACT order, not just the set: with every created_at equal,
	// a plain "ORDER BY created_at DESC" lets the planner hand back insertion
	// order, which on this small a table it happily does - so a set-equality
	// assertion passes with or without the tiebreaker and proves nothing. Only
	// "id DESC" makes the order total, and only descending ids show it.
	assert.Equal(t, []string{"req-d", "req-c", "req-b", "req-a"}, paged,
		"tied timestamps must fall back to the id tiebreaker, giving one total order")
}

// Update had NO test at all, and its SET list puts three text-ish columns
// side by side: status, admin_note, answered_by. Swap two positional
// arguments and the write succeeds - the operator's handwritten note is
// stored as the answering profile, or the status becomes the note. The
// requester then reads a moderation answer that says something nobody wrote.
//
// Distinct non-zero values everywhere, so no pair can hide a swap.
func TestMediaRequestRepo_Update_EveryColumnRoundTripsToItsOwnField(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo, uid, pid := seedRequestOwner(t, db, "-rt")
	ctx := context.Background()
	req := newTestRequest("req-rt", "770001", mediarequest.StatusPending, uid, pid)
	require.NoError(t, repo.Create(ctx, req))

	year := 2019
	answeredAt := time.Now().UTC().Truncate(time.Millisecond)
	req.MediaType = mediarequest.MediaTypeTV
	req.Provider = "imdb"
	req.ProviderID = "tt7654321"
	req.Title = "Some Series"
	req.Year = &year
	req.PosterURL = "https://example.invalid/poster.jpg"
	req.Status = mediarequest.StatusRejected
	req.AdminNote = "already in the library"
	req.AnsweredBy = pid
	req.AnsweredAt = &answeredAt
	req.UpdatedAt = answeredAt
	require.NoError(t, repo.Update(ctx, req))

	got, err := repo.GetByID(ctx, "req-rt")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, mediarequest.MediaTypeTV, got.MediaType)
	assert.Equal(t, "imdb", got.Provider)
	assert.Equal(t, "tt7654321", got.ProviderID)
	assert.Equal(t, "Some Series", got.Title)
	require.NotNil(t, got.Year)
	assert.Equal(t, 2019, *got.Year)
	assert.Equal(t, "https://example.invalid/poster.jpg", got.PosterURL)
	// The three adjacent columns this test exists for.
	assert.Equal(t, mediarequest.StatusRejected, got.Status)
	assert.Equal(t, "already in the library", got.AdminNote)
	assert.Equal(t, pid, got.AnsweredBy)
	require.NotNil(t, got.AnsweredAt)
}

// Answering a request that is no longer there must SAY so.
//
// Update threw its result away, so an UPDATE that matched nothing returned
// nil - and the service above it read that as "saved", then published
// RequestAnswered. The requester got told their request was approved while
// the decision was never written anywhere. The window is narrow (the service
// reads the row immediately before), but the whole point of this repository's
// RowsAffected discipline is that "narrow" is not "impossible": every other
// mutation here already reports a no-op as ErrNotFound.
func TestMediaRequestRepo_UpdateOnAVanishedRowIsNotASuccess(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	repo, userID, profileID := seedRequestOwner(t, db, "upd-gone")

	req := newTestRequest("req-gone-1", "tt-gone-1", mediarequest.StatusPending, userID, profileID)
	require.NoError(t, repo.Create(context.Background(), req))
	require.NoError(t, repo.Delete(context.Background(), req.ID))

	req.Status = mediarequest.StatusApproved
	err := repo.Update(context.Background(), req)

	require.ErrorIs(t, err, mediarequest.ErrNotFound,
		"an update that touched no row must not be reported as saved")
}

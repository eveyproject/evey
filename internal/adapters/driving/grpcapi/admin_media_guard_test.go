package grpcapi

import (
	"context"
	"testing"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	applibrary "github.com/evey-project/evey/internal/app/library"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	mkvfakes "github.com/evey-project/evey/internal/domain/mkv/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeLibraryGetter serves one canned library (or error) to the type guard.
type fakeLibraryGetter struct {
	lib *library.Library
	err error
}

func (f fakeLibraryGetter) GetLibrary(_ context.Context, _ string) (*library.Library, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.lib, nil
}

func TestRefreshAllMetadata_MusicLibrary_FailedPrecondition(t *testing.T) {
	h, _ := newOrphansHandler()
	h.heavyOpSemaphore = make(chan struct{}, 1)
	h.libQuery = fakeLibraryGetter{lib: &library.Library{ID: "lib-music", Type: library.Music}}

	_, err := h.RefreshAllMetadata(context.Background(), &eveyv1.RefreshAllMetadataRequest{LibraryId: "lib-music"})
	require.Error(t, err)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	// The rejected call must not consume the heavy-op slot.
	assert.True(t, h.acquireHeavyOp())
}

func TestRefreshAllMetadata_UnknownLibrary_NotFound(t *testing.T) {
	h, _ := newOrphansHandler()
	h.libQuery = fakeLibraryGetter{err: shared.ErrNotFound}

	_, err := h.RefreshAllMetadata(context.Background(), &eveyv1.RefreshAllMetadataRequest{LibraryId: "lib-gone"})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestMatchOrphansInLibrary_PhotoLibrary_FailedPrecondition(t *testing.T) {
	h, _ := newOrphansHandler()
	h.libQuery = fakeLibraryGetter{lib: &library.Library{ID: "lib-photos", Type: library.Photo}}

	_, err := h.MatchOrphansInLibrary(context.Background(), &eveyv1.MatchOrphansInLibraryRequest{LibraryId: "lib-photos"})
	require.Error(t, err)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestMatchOrphansInLibrary_MovieLibrary_PassesGuard(t *testing.T) {
	h, _ := newOrphansHandler()
	h.libQuery = fakeLibraryGetter{lib: &library.Library{ID: "lib-movies", Type: library.Movie}}

	res, err := h.MatchOrphansInLibrary(context.Background(), &eveyv1.MatchOrphansInLibraryRequest{LibraryId: "lib-movies"})
	require.NoError(t, err)
	assert.Zero(t, res.Queued)
}

func TestMatchOrphansInLibrary_ShowLibrary_PassesGuard(t *testing.T) {
	h, _ := newOrphansHandler()
	h.libQuery = fakeLibraryGetter{lib: &library.Library{ID: "lib-shows", Type: library.TVShow}}

	res, err := h.MatchOrphansInLibrary(context.Background(), &eveyv1.MatchOrphansInLibraryRequest{LibraryId: "lib-shows"})
	require.NoError(t, err)
	assert.Zero(t, res.Queued)
}

// The heavy-op slot: a second library-wide job is refused while one runs
// (ResourceExhausted, not queued), the slot is held by the DETACHED work and
// handed back only when it finishes, and an unwired service is Unimplemented.
func TestHeavyOpSlot_RefreshAllAndRelink(t *testing.T) {
	h, _ := newOrphansHandler()
	h.heavyOpSemaphore = make(chan struct{}, 1)
	h.libQuery = fakeLibraryGetter{lib: &library.Library{ID: "lib-1", Type: library.Movie}}

	_, err := (&adminMediaHandler{}).RefreshAllMetadata(context.Background(), &eveyv1.RefreshAllMetadataRequest{LibraryId: "lib-1"})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
	_, err = (&adminMediaHandler{}).RelinkCollections(context.Background(), &eveyv1.RelinkCollectionsRequest{})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
	_, err = (&adminMediaHandler{}).GenerateAllTrickplays(context.Background(), &eveyv1.GenerateAllTrickplaysRequest{})
	assert.Equal(t, codes.Unimplemented, status.Code(err))

	// Occupy the slot as a running job would.
	require.True(t, h.acquireHeavyOp())
	_, err = h.RefreshAllMetadata(context.Background(), &eveyv1.RefreshAllMetadataRequest{LibraryId: "lib-1"})
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	_, err = h.RelinkCollections(context.Background(), &eveyv1.RelinkCollectionsRequest{})
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	h.releaseHeavyOp()

	// The RPC returns at once; the slot is released by the background work.
	_, err = h.RelinkCollections(context.Background(), &eveyv1.RelinkCollectionsRequest{Limit: 5})
	require.NoError(t, err)
	require.Eventually(t, h.acquireHeavyOp, 2*time.Second, 10*time.Millisecond, "the detached relink must hand the slot back")
	h.releaseHeavyOp()
}

// Deleting a title that is already gone (a double click, a stale admin tab)
// is NotFound, not a server fault paged as a 500; nothing is audited for it.
func TestDeleteMediaItem_UnknownIsNotFound(t *testing.T) {
	mediaRepo := libfakes.NewFakeMediaItemCommandRepo()
	libCmd := applibrary.NewCommandService(
		libfakes.NewFakeLibraryCommandRepo(), libfakes.NewFakeLibraryQueryRepo(), mediaRepo,
		libfakes.NewFakeMediaStreamRepository(), libfakes.NewFakePersonRepository(), libfakes.NewFakeMediaPersonRepository(),
		libfakes.NewFakeFileScanner(), libfakes.NewFakeStreamProber(), libfakes.NewFakeSearchIndex(), sharedfakes.NewFakeEventBus(),
		libfakes.NewFakeChapterRepository(), mkvfakes.NewFakeContainerReader(),
		libfakes.NewFakeTrickplayRepository(), libfakes.NewFakeTrickplayGenerator(), t.TempDir(), sharedfakes.NewFakeServerConfigRepository(),
	)
	d, repo := newAuditedMediaHandler(t)
	d.libCmd = libCmd
	ctx := ctxWithClaims("adm-1", "prf-adm", true)

	_, err := d.DeleteMediaItem(ctx, &eveyv1.DeleteMediaItemRequest{Id: "ghost"})
	assert.Equal(t, codes.NotFound, status.Code(err))
	assert.Empty(t, auditEntries(t, repo, "delete_media_item"))

	require.NoError(t, mediaRepo.Create(context.Background(), &library.MediaItem{ID: "mi-1", LibraryID: "lib-1", Title: "Heat"}))
	_, err = d.DeleteMediaItem(ctx, &eveyv1.DeleteMediaItemRequest{Id: "mi-1"})
	require.NoError(t, err)
	assert.Len(t, auditEntries(t, repo, "delete_media_item"), 1)
}

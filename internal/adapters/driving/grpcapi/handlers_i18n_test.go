package grpcapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	appsyncplay "github.com/evey-project/evey/internal/app/syncplay"
	"github.com/evey-project/evey/internal/domain/mediarequest"
	"github.com/evey-project/evey/internal/domain/user"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
)

func realTranslator(t *testing.T) *eveyi18n.Translator {
	t.Helper()
	tr, err := eveyi18n.NewFromDir("../../../../i18n")
	require.NoError(t, err)
	return tr
}

func frenchCtx(userID, profileID string, admin bool) context.Context {
	return eveyi18n.ContextWithLocale(interceptors.ContextWithClaims(context.Background(), &user.Claims{
		UserID: userID, Username: userID, IsAdmin: admin, ProfileID: profileID,
	}), "fr")
}

func grpcMessage(err error) string { return status.Convert(err).Message() }

// The "not found" a French viewer reads on a library, a playlist or a share
// link comes from the catalogue, not from a hard-coded English sentence.
func TestNotFoundRefusals_AreTranslated(t *testing.T) {
	tr := realTranslator(t)
	fr := frenchCtx("user-1", "profile-1", true)

	lib, _, _, _, _, _, _ := newTestLibraryHandler()
	lib.translator = tr
	_, err := lib.GetLibrary(fr, &eveyv1.GetLibraryRequest{Id: "ghost"})
	assert.Equal(t, codes.NotFound, status.Code(err))
	assert.Equal(t, tr.T("fr", "errors.library.not_found"), grpcMessage(err))
	_, err = lib.UpdateLibrary(fr, &eveyv1.UpdateLibraryRequest{})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Equal(t, tr.T("fr", "errors.library.id_required"), grpcMessage(err))
	_, err = lib.GetMediaItem(fr, &eveyv1.GetMediaItemRequest{Id: "ghost"})
	assert.Equal(t, tr.T("fr", "errors.media.not_found"), grpcMessage(err))

	pl := newTestPlaylistHandler()
	pl.translator = tr
	_, err = pl.GetPlaylist(fr, &eveyv1.GetPlaylistRequest{Id: "ghost"})
	assert.Equal(t, codes.NotFound, status.Code(err))
	assert.Equal(t, tr.T("fr", "errors.playlist.not_found"), grpcMessage(err))

	share := newShareTestEnv()
	share.handler.translator = tr
	_, err = share.handler.RevokeShareLink(fr, &eveyv1.RevokeShareLinkRequest{Id: "ghost"})
	assert.Equal(t, codes.NotFound, status.Code(err))
	assert.Equal(t, tr.T("fr", "errors.share.not_found"), grpcMessage(err))
	_, err = share.handler.ValidateShareLink(fr, &eveyv1.ValidateShareLinkRequest{Token: "ghost"})
	assert.Equal(t, tr.T("fr", "errors.share.not_found"), grpcMessage(err))

	users := setupUserHandler()
	users.handler.translator = tr
	_, err = users.handler.SetDefaultProfile(fr, &eveyv1.SetDefaultProfileRequest{Id: "ghost"})
	assert.Equal(t, codes.NotFound, status.Code(err))
	assert.Equal(t, tr.T("fr", "errors.auth.profile_not_found"), grpcMessage(err))
	_, err = users.handler.SetDefaultProfile(fr, &eveyv1.SetDefaultProfileRequest{})
	assert.Equal(t, tr.T("fr", "errors.user.profile_id_required"), grpcMessage(err))

	for _, key := range []string{"errors.library.not_found", "errors.library.id_required", "errors.playlist.not_found", "errors.share.not_found", "errors.share.exhausted", "errors.user.profile_id_required", "errors.auth.profiles_need_non_kids", "errors.user.invalid_avatar_path", "errors.library.invalid_path", "errors.library.cron_required", "errors.library.cron_invalid", "errors.library.cursor_invalid"} {
		assert.NotEqual(t, tr.T("en", key), tr.T("fr", key), key)
		assert.NotEqual(t, key, tr.T("en", key), "missing in the English catalogue")
	}
}

// Watch-together and media-request refusals are read by every household
// member, not only admins: they must come out of the catalogue.
func TestSyncplayAndRequestRefusals_AreTranslated(t *testing.T) {
	tr := realTranslator(t)
	fr := frenchCtx("user-1", "profile-1", false)

	sp := newSyncplayTestEnv()
	sp.handler.translator = tr
	_, err := sp.handler.JoinByCode(fr, &eveyv1.JoinSyncPlayByCodeRequest{Code: "ZZZZ"})
	assert.Equal(t, codes.NotFound, status.Code(err))
	assert.Equal(t, tr.T("fr", "errors.syncplay.room_not_found"), grpcMessage(err))
	_, err = sp.handler.JoinByCode(fr, &eveyv1.JoinSyncPlayByCodeRequest{})
	assert.Equal(t, tr.T("fr", "errors.syncplay.code_required"), grpcMessage(err))
	_, err = sp.handler.GetRoom(fr, &eveyv1.GetSyncPlayRoomRequest{RoomId: "ghost"})
	assert.Equal(t, tr.T("fr", "errors.syncplay.room_not_found"), grpcMessage(err))
	_, err = sp.handler.CreateRoom(fr, &eveyv1.CreateSyncPlayRoomRequest{PlaylistId: "ghost"})
	assert.Equal(t, tr.T("fr", "errors.playlist.not_found"), grpcMessage(err))
	_, err = sp.handler.CreateRoom(fr, &eveyv1.CreateSyncPlayRoomRequest{MediaId: "ghost"})
	assert.Equal(t, tr.T("fr", "errors.media.not_found"), grpcMessage(err))

	frCtx := eveyi18n.ContextWithLocale(context.Background(), "fr")
	assert.Equal(t, tr.T("fr", "errors.syncplay.too_many_rooms"), grpcMessage(createRoomError(frCtx, tr, appsyncplay.ErrTooManyRooms)))
	assert.Equal(t, tr.T("fr", "errors.request.disabled"), grpcMessage(mapMediaRequestError(frCtx, tr, "op", mediarequest.ErrDisabled)))
	assert.Equal(t, tr.T("fr", "errors.request.duplicate"), grpcMessage(mapMediaRequestError(frCtx, tr, "op", mediarequest.ErrDuplicate)))
	assert.Equal(t, tr.T("fr", "errors.request.already_held"), grpcMessage(mapMediaRequestError(frCtx, tr, "op", mediarequest.ErrAlreadyHeld)))
	assert.Equal(t, tr.T("fr", "errors.request.not_found"), grpcMessage(mapMediaRequestError(frCtx, tr, "op", mediarequest.ErrNotFound)))

	for _, key := range []string{"errors.syncplay.room_not_found", "errors.syncplay.code_required", "errors.syncplay.too_many_rooms", "errors.syncplay.playlist_empty", "errors.syncplay.queue_empty", "errors.syncplay.name_too_long", "errors.request.disabled", "errors.request.duplicate", "errors.request.already_held", "errors.request.not_found"} {
		assert.NotEqual(t, tr.T("en", key), tr.T("fr", key), key)
		assert.NotEqual(t, key, tr.T("en", key), "missing in the English catalogue")
	}
}

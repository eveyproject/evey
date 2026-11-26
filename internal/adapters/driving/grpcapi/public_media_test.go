package grpcapi

import (
	"context"
	"net/url"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/grpc-ecosystem/grpc-gateway/v2/utilities"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/eveyproject/evey/internal/domain/library"
	domainmetadata "github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/pkg/auth"
	"github.com/stretchr/testify/assert"
)

func TestPublicMediaItemToProto_ProjectsSafeFields(t *testing.T) {
	year := 1999
	item := &library.MediaItem{
		ID:       "m1",
		Type:     library.MovieItem,
		Title:    "Some Movie",
		Overview: "a computer hacker",
		Catalog:  &library.MediaCatalog{Year: &year, ProviderIDs: map[string]string{"tmdb": "603", "imdb": "tt0133093"}},
		// File carries the path that must NEVER reach the public projection.
		File: &library.MediaFile{Path: "/secret/movies/some-movie.mkv"},
	}
	pb := publicMediaItemToProto(item, "https://host/poster.jpg", false)

	assert.Equal(t, "m1", pb.Id)
	assert.Equal(t, "Some Movie", pb.Title)
	assert.Equal(t, int32(1999), pb.Year)
	assert.Equal(t, "a computer hacker", pb.Overview)
	assert.Equal(t, int32(603), pb.TmdbId)
	assert.Equal(t, "tt0133093", pb.ImdbId)
	assert.Equal(t, "https://host/poster.jpg", pb.PosterUrl)
	assert.Empty(t, pb.FilePath, "the path is only for a media.files plugin token")
	assert.Zero(t, pb.FileSize)
}

func TestPublicMediaItemToProto_FilesOnlyWhenAsked(t *testing.T) {
	item := &library.MediaItem{ID: "m1", Type: library.MovieItem, Title: "T",
		File: &library.MediaFile{Path: "/movies/t.mkv", Size: 42}}
	pb := publicMediaItemToProto(item, "", true)
	assert.Equal(t, "/movies/t.mkv", pb.FilePath)
	assert.Equal(t, int64(42), pb.FileSize)
	// A folder item (a series) has no file: nothing to show, no panic.
	assert.Empty(t, publicMediaItemToProto(&library.MediaItem{ID: "s", Title: "S"}, "", true).FilePath)
}

// The scope decides, and only on a plugin token: a user session (no plugin
// claims) and a plugin token without media.files both get no paths.
func TestWithFiles_RequiresMediaFilesPluginScope(t *testing.T) {
	assert.False(t, withFiles(context.Background()))
	assert.False(t, withFiles(interceptors.ContextWithPluginClaims(context.Background(),
		&auth.PluginClaims{Scopes: []string{"public"}})))
	assert.True(t, withFiles(interceptors.ContextWithPluginClaims(context.Background(),
		&auth.PluginClaims{Scopes: []string{"public", "media.files"}})))
}

func TestPosterURLForRecords_PicksPoster(t *testing.T) {
	url := "https://host/p.jpg"
	recs := []*domainmetadata.ArtworkRecord{
		{Type: "backdrop", SourceURL: new("https://host/b.jpg")},
		{Type: "poster", SourceURL: &url},
	}
	assert.NotEmpty(t, posterURLForRecords(recs))
	assert.Empty(t, posterURLForRecords(nil))
	assert.Empty(t, posterURLForRecords([]*domainmetadata.ArtworkRecord{{Type: "logo"}}))
}

// The plugin bridge (frontend pluginRpc mediaListQuery) builds this exact
// query; the gateway must decode it into the request, or a plugin silently
// reads the first 500 items of every type.
func TestPublicMediaListQuery_DecodedByGateway(t *testing.T) {
	q, err := url.ParseQuery("libraryId=L&type=MEDIA_ITEM_TYPE_MOVIE&pagination.page=2&pagination.pageSize=500")
	require.NoError(t, err)
	req := &eveyv1.ListPublicMediaItemsRequest{}
	require.NoError(t, runtime.PopulateQueryParameters(req, q, utilities.NewDoubleArray(nil)))
	assert.Equal(t, "L", req.LibraryId)
	assert.Equal(t, eveyv1.MediaItemType_MEDIA_ITEM_TYPE_MOVIE, req.Type)
	assert.Equal(t, int32(2), req.GetPagination().GetPage())
	assert.Equal(t, int32(500), req.GetPagination().GetPageSize())
}

// Quality: the first video track and the default audio track, even when the
// default is not the first one.
func TestPublicQuality_PicksVideoAndDefaultAudio(t *testing.T) {
	w, h, depth, ch2, ch6 := 3840, 2160, 10, 2, 6
	hdr := "HDR10"
	f := &library.MediaFile{Bitrate: 25_000_000, ContainerFormat: "matroska"}
	q := publicQuality(f, []*library.MediaStream{
		{StreamType: library.StreamTypeAudio, Codec: "aac", Channels: &ch2},
		{StreamType: library.StreamTypeVideo, Codec: "hevc", Width: &w, Height: &h, BitDepth: &depth, HDRType: &hdr},
		{StreamType: library.StreamTypeAudio, Codec: "eac3", Channels: &ch6, IsDefault: true},
	})
	require.NotNil(t, q)
	assert.Equal(t, int32(2160), q.Height)
	assert.Equal(t, "hevc", q.VideoCodec)
	assert.Equal(t, "HDR10", q.HdrType)
	assert.Equal(t, int32(10), q.BitDepth)
	assert.Equal(t, "eac3", q.AudioCodec)
	assert.Equal(t, int32(6), q.AudioChannels)
	assert.Equal(t, int64(25_000_000), q.Bitrate)
	assert.Nil(t, publicQuality(nil, nil))
}

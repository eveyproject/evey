package grpcapi

import (
	"testing"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLibraryTypeEnumMapping(t *testing.T) {
	t.Parallel()
	want := map[string]eveyv1.LibraryType{
		"movie":  eveyv1.LibraryType_LIBRARY_TYPE_MOVIES,
		"tvshow": eveyv1.LibraryType_LIBRARY_TYPE_SHOWS,
		"music":  eveyv1.LibraryType_LIBRARY_TYPE_MUSIC,
		"photo":  eveyv1.LibraryType_LIBRARY_TYPE_PHOTOS,
	}
	for domainT, protoT := range want {
		assert.Equal(t, protoT, libraryTypeToProto[domainT], "to-proto mapping for %q", domainT)
		assert.Equal(t, domainT, libraryTypeFromProto[protoT], "from-proto mapping for %v", protoT)
	}
	// Round-trip all values
	for domainT := range want {
		got := libraryTypeFromProto[libraryTypeToProto[domainT]]
		assert.Equal(t, domainT, got, "round-trip for %q", domainT)
	}
}

func TestMediaItemTypeEnumMapping(t *testing.T) {
	t.Parallel()
	cases := []string{"movie", "series", "season", "episode", "album", "artist", "track", "photo"}
	for _, c := range cases {
		pt := mediaItemTypeToProto[c]
		require.NotEqual(t, eveyv1.MediaItemType_MEDIA_ITEM_TYPE_UNSPECIFIED, pt, "type %q not mapped", c)
		assert.Equal(t, c, mediaItemTypeFromProto[pt], "round-trip for %q", c)
	}
}

func TestLibraryToProto_FullFields(t *testing.T) {
	t.Parallel()
	last := time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC)
	lib := &library.Library{
		ID:                 "lib-x",
		Name:               "Photos",
		Type:               "photo",
		Paths:              []string{"/a", "/b"},
		ScanOnStartup:      true,
		RealtimeMonitoring: true,
		LastScannedAt:      &last,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	pb := libraryToProto(lib)
	assert.Equal(t, "lib-x", pb.Id)
	assert.Equal(t, eveyv1.LibraryType_LIBRARY_TYPE_PHOTOS, pb.Type)
	assert.Equal(t, []string{"/a", "/b"}, pb.Paths)
	assert.True(t, pb.ScanOnStartup)
	assert.True(t, pb.RealtimeMonitoring)
	require.NotNil(t, pb.LastScannedAt)
	assert.Equal(t, last.Unix(), pb.LastScannedAt.AsTime().Unix())
}

func TestLibraryToProto_NilLastScanned(t *testing.T) {
	t.Parallel()
	lib := &library.Library{ID: "l1", Name: "N", Type: "movie"}
	pb := libraryToProto(lib)
	assert.Nil(t, pb.LastScannedAt)
	assert.Equal(t, eveyv1.LibraryType_LIBRARY_TYPE_MOVIES, pb.Type)
}

func TestSortFieldAndOrderMapping(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "title", mediaSortFieldFromProto[eveyv1.MediaSortField_MEDIA_SORT_FIELD_TITLE])
	assert.Equal(t, "year", mediaSortFieldFromProto[eveyv1.MediaSortField_MEDIA_SORT_FIELD_YEAR])
	assert.Equal(t, "rating", mediaSortFieldFromProto[eveyv1.MediaSortField_MEDIA_SORT_FIELD_RATING])
	assert.Equal(t, "date_added", mediaSortFieldFromProto[eveyv1.MediaSortField_MEDIA_SORT_FIELD_DATE_ADDED])
	assert.Equal(t, "premiere_date", mediaSortFieldFromProto[eveyv1.MediaSortField_MEDIA_SORT_FIELD_PREMIERE_DATE])
	assert.Equal(t, "asc", sortOrderFromProto[eveyv1.SortOrder_SORT_ORDER_ASC])
	assert.Equal(t, "desc", sortOrderFromProto[eveyv1.SortOrder_SORT_ORDER_DESC])
}

func TestStreamTypeEnumMapping(t *testing.T) {
	t.Parallel()
	cases := map[library.StreamType]eveyv1.MediaStreamType{
		library.StreamTypeVideo:      eveyv1.MediaStreamType_MEDIA_STREAM_TYPE_VIDEO,
		library.StreamTypeAudio:      eveyv1.MediaStreamType_MEDIA_STREAM_TYPE_AUDIO,
		library.StreamTypeSubtitle:   eveyv1.MediaStreamType_MEDIA_STREAM_TYPE_SUBTITLE,
		library.StreamTypeAttachment: eveyv1.MediaStreamType_MEDIA_STREAM_TYPE_ATTACHMENT,
	}
	for k, v := range cases {
		assert.Equal(t, v, streamTypeToProto[k])
	}
}

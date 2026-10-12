package grpcapi

import (
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	domainmetadata "github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/stretchr/testify/assert"
)

func TestArtworkTypeEnumMapping(t *testing.T) {
	t.Parallel()
	cases := map[string]eveyv1.ArtworkType{
		"poster":   eveyv1.ArtworkType_ARTWORK_TYPE_PRIMARY,
		"backdrop": eveyv1.ArtworkType_ARTWORK_TYPE_BACKDROP,
		"banner":   eveyv1.ArtworkType_ARTWORK_TYPE_BANNER,
		"thumb":    eveyv1.ArtworkType_ARTWORK_TYPE_THUMB,
		"logo":     eveyv1.ArtworkType_ARTWORK_TYPE_LOGO,
		"disc":     eveyv1.ArtworkType_ARTWORK_TYPE_DISC,
	}
	for k, v := range cases {
		assert.Equal(t, v, artworkTypeToProto[k], "type=%s", k)
	}
	// Unknown type → zero value (UNSPECIFIED)
	assert.Equal(t, eveyv1.ArtworkType_ARTWORK_TYPE_UNSPECIFIED, artworkTypeToProto["bogus"])
}

func TestMetadataOverrideToProto_NonStringCustomValue(t *testing.T) {
	t.Parallel()
	o := &domainmetadata.MetadataOverride{
		ID:          "mo-int",
		MediaItemID: "m-1",
		FieldName:   "year",
		CustomValue: 2024,
	}
	pb := metadataOverrideToProto(o)
	assert.Equal(t, "2024", pb.CustomValue)
}

func TestMetadataResultToProto_NilTagline(t *testing.T) {
	t.Parallel()
	r := domainmetadata.MetadataResult{
		Title:    "X",
		Overview: "desc",
		Genres:   []string{"A", "B"},
	}
	pb := metadataResultToProto(r)
	assert.Equal(t, "X", pb.Title)
	assert.Equal(t, []string{"A", "B"}, pb.Genres)
	assert.Equal(t, int32(0), pb.Year)
	assert.Equal(t, float64(0), pb.Rating)
}

func TestArtworkRecordToProto_AllTypes(t *testing.T) {
	t.Parallel()
	for _, typ := range []string{"poster", "backdrop", "banner", "thumb", "logo", "disc"} {
		a := &domainmetadata.ArtworkRecord{ID: "a-" + typ, Provider: "tmdb", Type: typ}
		pb := artworkRecordToProto(a)
		assert.Equal(t, artworkTypeToProto[typ], pb.Type, "type=%s", typ)
		assert.Equal(t, "tmdb", pb.Provider)
	}
}

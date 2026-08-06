package grpcapi

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/library"
	domainmetadata "github.com/evey-project/evey/internal/domain/metadata"
	"github.com/stretchr/testify/assert"
)

func TestPublicMediaItemToProto_ProjectsSafeFields(t *testing.T) {
	year := 1999
	item := &library.MediaItem{
		ID:       "m1",
		Type:     library.MovieItem,
		Title:    "Some Movie",
		Overview: "a computer hacker",
		Catalog:  &library.MediaCatalog{Year: &year},
		// File carries the path that must NEVER reach the public projection.
		File: &library.MediaFile{Path: "/secret/movies/some-movie.mkv"},
	}
	pb := publicMediaItemToProto(item, "https://host/poster.jpg")

	assert.Equal(t, "m1", pb.Id)
	assert.Equal(t, "Some Movie", pb.Title)
	assert.Equal(t, int32(1999), pb.Year)
	assert.Equal(t, "a computer hacker", pb.Overview)
	assert.Equal(t, "https://host/poster.jpg", pb.PosterUrl)
	// PublicMediaItem has no file_path field at all - the leak is impossible by
	// construction. This test pins the safe-field mapping.
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

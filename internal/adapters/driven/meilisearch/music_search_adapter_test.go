package meilisearch

import (
	"context"
	"fmt"
	"testing"

	ms "github.com/meilisearch/meilisearch-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/music"
)

// The music document must reconstruct a playable track without a DB hydration:
// every field round-trips, with all values distinct so a swapped pair shows.
func TestMusicDocument_RoundTripsEveryField(t *testing.T) {
	in := music.Track{
		ID: "t-1", LibraryID: "lib-2", Title: "Grey Rail", Artist: "Saxwell", Album: "Grey Rail LP",
		AlbumArtist: "John Saxwell", Year: 1958, TrackNumber: 3, DiskNumber: 2, Duration: 642.5,
		FilePath: "/music/blue.flac", FileSize: 41_000_000, MimeType: "audio/flac", Bitrate: 900,
		SampleRate: 44100, Channels: 6, Genre: "Jazz", AlbumFolder: "/music", CoverArtURL: "/art/blue.jpg",
	}
	doc := musicToDocument(in)
	assert.Equal(t, "lib-2", doc.LibraryID, "library id is the filterable attribute")
	assert.Equal(t, in, doc.toTrack())
}

func TestNewMusicSearchAdapter_IndexLifecycle(t *testing.T) {
	// Missing index: created with the track id as primary key.
	svc := &fakeServiceClient{getIndexErr: fmt.Errorf("index music_tracks not found")}
	adapter, err := newMusicSearchAdapterFromClient(context.Background(), svc)
	require.NoError(t, err)
	require.NotNil(t, adapter)
	require.NotNil(t, svc.createdConfig)
	assert.Equal(t, musicIndexName, svc.createdConfig.Uid)
	assert.Equal(t, "id", svc.createdConfig.PrimaryKey)
	assert.Equal(t, musicIndexName, svc.indexUID)

	// Existing index: left alone.
	svc = &fakeServiceClient{indexResult: &ms.IndexResult{UID: musicIndexName}}
	_, err = newMusicSearchAdapterFromClient(context.Background(), svc)
	require.NoError(t, err)
	assert.Nil(t, svc.createdConfig)

	// Unreachable: the error names the step.
	svc = &fakeServiceClient{getIndexErr: fmt.Errorf("dial tcp"), createIndexErr: fmt.Errorf("connection refused")}
	_, err = newMusicSearchAdapterFromClient(context.Background(), svc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create music index")
	assert.Contains(t, err.Error(), "connection refused")
}

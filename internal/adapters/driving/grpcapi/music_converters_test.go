package grpcapi

import (
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/music"
	"github.com/stretchr/testify/assert"
)

func TestTrackToProto_FullFields(t *testing.T) {
	t.Parallel()
	created := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	updated := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	tr := music.Track{
		ID:          "tr-1",
		LibraryID:   "lib-1",
		Title:       "Song",
		Artist:      "Artist",
		Album:       "Album",
		AlbumArtist: "AA",
		Year:        2020,
		TrackNumber: 3,
		DiskNumber:  1,
		Duration:    240.5,
		FilePath:    "/music/song.mp3",
		FileSize:    5_000_000,
		MimeType:    "audio/mpeg",
		Bitrate:     320,
		SampleRate:  44100,
		Channels:    2,
		Genre:       "Rock",
		CoverArtURL: "/cover.jpg",
		CreatedAt:   created,
		UpdatedAt:   updated,
	}
	pb := trackToProto(tr, true)
	assert.Equal(t, "tr-1", pb.Id)
	assert.Equal(t, "lib-1", pb.LibraryId)
	assert.Equal(t, "Song", pb.Title)
	assert.Equal(t, "Artist", pb.Artist)
	assert.Equal(t, "Album", pb.Album)
	assert.Equal(t, "AA", pb.AlbumArtist)
	assert.Equal(t, int32(2020), pb.Year)
	assert.Equal(t, int32(3), pb.TrackNumber)
	assert.Equal(t, int32(1), pb.DiskNumber)
	assert.Equal(t, 240.5, pb.Duration)
	assert.Equal(t, "/music/song.mp3", pb.FilePath)
	assert.Equal(t, int64(5_000_000), pb.FileSize)
	assert.Equal(t, int32(320), pb.Bitrate)
	assert.Equal(t, int32(44100), pb.SampleRate)
	assert.Equal(t, int32(2), pb.Channels)
	assert.Equal(t, "Rock", pb.Genre)
	assert.Equal(t, "/cover.jpg", pb.CoverArtUrl)
	assert.Equal(t, created.Unix(), pb.CreatedAt.AsTime().Unix())
	assert.Equal(t, updated.Unix(), pb.UpdatedAt.AsTime().Unix())
}

func TestTrackToProto_EmptyFields(t *testing.T) {
	t.Parallel()
	tr := music.Track{ID: "tr-empty"}
	pb := trackToProto(tr, true)
	assert.Equal(t, "tr-empty", pb.Id)
	assert.Equal(t, int32(0), pb.Year)
	assert.Equal(t, int32(0), pb.TrackNumber)
	assert.Equal(t, "", pb.Title)
	// Zero time → nil timestamp
	assert.Nil(t, pb.CreatedAt)
	assert.Nil(t, pb.UpdatedAt)
}

func TestTrackToProto_RedactsFilePathForNonAdmin(t *testing.T) {
	t.Parallel()
	tr := music.Track{ID: "tr-2", FilePath: "/music/secret.mp3"}
	nonAdmin := trackToProto(tr, false)
	assert.Equal(t, "", nonAdmin.FilePath, "non-admin must not see file_path")
	admin := trackToProto(tr, true)
	assert.Equal(t, "/music/secret.mp3", admin.FilePath)
}

package grpcapi

import (
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/photo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhotoToProto_FullFields(t *testing.T) {
	t.Parallel()
	taken := time.Date(2023, 7, 4, 12, 0, 0, 0, time.UTC)
	p := photo.Photo{
		ID:          "ph-1",
		LibraryID:   "lib-p",
		Title:       "Beach",
		FilePath:    "/photos/beach.jpg",
		FileSize:    2_000_000,
		MimeType:    "image/jpeg",
		Width:       4000,
		Height:      3000,
		Album:       "Summer",
		TakenAt:     &taken,
		CameraModel: "Canon EOS",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	pb := photoToProto(p, true)
	assert.Equal(t, "/photos/beach.jpg", pb.FilePath, "admin should receive the file path")
	assert.Empty(t, photoToProto(p, false).FilePath, "non-admin must not receive the absolute file path")
	assert.Equal(t, "ph-1", pb.Id)
	assert.Equal(t, "lib-p", pb.LibraryId)
	assert.Equal(t, "Beach", pb.Title)
	assert.Equal(t, int32(4000), pb.Width)
	assert.Equal(t, int32(3000), pb.Height)
	assert.Equal(t, "Summer", pb.Album)
	assert.Equal(t, "Canon EOS", pb.CameraModel)
	require.NotNil(t, pb.TakenAt)
	assert.Equal(t, taken.Unix(), pb.TakenAt.AsTime().Unix())
}

func TestPhotoToProto_NilTakenAt(t *testing.T) {
	t.Parallel()
	p := photo.Photo{ID: "ph-2", Width: 100, Height: 50}
	pb := photoToProto(p, true)
	assert.Nil(t, pb.TakenAt)
	assert.Equal(t, int32(100), pb.Width)
	assert.Equal(t, int32(50), pb.Height)
	assert.Nil(t, pb.CreatedAt)
}

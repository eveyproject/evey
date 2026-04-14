package grpcapi

import (
	"context"
	"errors"
	"testing"
	"time"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/domain/transcoding"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	tcfakes "github.com/evey-project/evey/internal/domain/transcoding/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestTranscodeCacheHandler() (*transcodeCacheHandler, *tcfakes.FakeCacheStore) {
	transcoder := tcfakes.NewFakeTranscoder()
	hwDetector := tcfakes.NewFakeHWDetector()
	jobRepo := tcfakes.NewFakeTranscodeJobRepository()
	profileRepo := tcfakes.NewFakeTranscodeProfileRepository()
	events := sharedfakes.NewFakeEventBus()
	svc := apptranscoding.NewService(transcoder, hwDetector, jobRepo, profileRepo, events, 5)
	cacheStore := tcfakes.NewFakeCacheStore()
	apptranscoding.WithCacheStore(cacheStore)(svc)
	return &transcodeCacheHandler{svc: svc}, cacheStore
}

func TestTranscodeCacheHandler_GetCacheInfo_Empty(t *testing.T) {
	h, _ := newTestTranscodeCacheHandler()

	resp, err := h.GetTranscodeCache(context.Background(), &eveyv1.GetTranscodeCacheRequest{})

	require.NoError(t, err)
	assert.Equal(t, int64(0), resp.TotalSize)
	assert.Equal(t, int32(0), resp.FileCount)
	assert.Empty(t, resp.Files)
}

func TestTranscodeCacheHandler_GetCacheInfo_WithFiles(t *testing.T) {
	h, store := newTestTranscodeCacheHandler()
	now := time.Now()

	store.AddFile(transcoding.CacheFileInfo{Name: "segment-1.ts", Size: 1024 * 1024, LastModified: now})
	store.AddFile(transcoding.CacheFileInfo{Name: "segment-2.ts", Size: 2 * 1024 * 1024, LastModified: now})

	resp, err := h.GetTranscodeCache(context.Background(), &eveyv1.GetTranscodeCacheRequest{})

	require.NoError(t, err)
	assert.Equal(t, int64(3*1024*1024), resp.TotalSize)
	assert.Equal(t, int32(2), resp.FileCount)
	assert.Len(t, resp.Files, 2)
}

func TestTranscodeCacheHandler_ClearCache(t *testing.T) {
	h, store := newTestTranscodeCacheHandler()
	now := time.Now()

	store.AddFile(transcoding.CacheFileInfo{Name: "segment-1.ts", Size: 512, LastModified: now})

	_, err := h.ClearTranscodeCache(context.Background(), &eveyv1.ClearTranscodeCacheRequest{})
	require.NoError(t, err)

	resp, err := h.GetTranscodeCache(context.Background(), &eveyv1.GetTranscodeCacheRequest{})
	require.NoError(t, err)
	assert.Equal(t, int32(0), resp.FileCount)
}

func TestTranscodeCacheHandler_ClearCache_StoreError(t *testing.T) {
	h, store := newTestTranscodeCacheHandler()
	store.Err = errors.New("disk error")

	_, err := h.ClearTranscodeCache(context.Background(), &eveyv1.ClearTranscodeCacheRequest{})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestTranscodeCacheHandler_DeleteFile_Success(t *testing.T) {
	h, store := newTestTranscodeCacheHandler()
	now := time.Now()

	store.AddFile(transcoding.CacheFileInfo{Name: "job-1.ts", Size: 256, LastModified: now})

	_, err := h.DeleteTranscodeCacheFile(context.Background(), &eveyv1.DeleteTranscodeCacheFileRequest{Filename: "job-1.ts"})
	require.NoError(t, err)

	resp, err := h.GetTranscodeCache(context.Background(), &eveyv1.GetTranscodeCacheRequest{})
	require.NoError(t, err)
	assert.Equal(t, int32(0), resp.FileCount)
}

func TestTranscodeCacheHandler_DeleteFile_NotFound(t *testing.T) {
	h, _ := newTestTranscodeCacheHandler()

	_, err := h.DeleteTranscodeCacheFile(context.Background(), &eveyv1.DeleteTranscodeCacheFileRequest{Filename: "nonexistent.ts"})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

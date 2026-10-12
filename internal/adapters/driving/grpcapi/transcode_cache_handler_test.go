package grpcapi

import (
	"context"
	"errors"
	"testing"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/eveyproject/evey/internal/domain/transcoding"
	tcfakes "github.com/eveyproject/evey/internal/domain/transcoding/fakes"
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

// A file already gone (the admin double-clicked, or the sweep got there
// first) is NotFound, and a name that tries to leave the cache directory is
// InvalidArgument - neither is a server fault worth a logged 500.
func TestTranscodeCacheHandler_DeleteFile_NotFoundAndInvalid(t *testing.T) {
	h, store := newTestTranscodeCacheHandler()

	_, err := h.DeleteTranscodeCacheFile(context.Background(), &eveyv1.DeleteTranscodeCacheFileRequest{Filename: "nonexistent.ts"})
	assert.Equal(t, codes.NotFound, status.Code(err))

	store.Err = transcoding.ErrCacheFilenameInvalid
	_, err = h.DeleteTranscodeCacheFile(context.Background(), &eveyv1.DeleteTranscodeCacheFileRequest{Filename: "../etc/passwd"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	store.Err = errors.New("disk gone")
	_, err = h.DeleteTranscodeCacheFile(context.Background(), &eveyv1.DeleteTranscodeCacheFileRequest{Filename: "job-1.ts"})
	assert.Equal(t, codes.Internal, status.Code(err))
	assert.NotContains(t, err.Error(), "disk gone")
}

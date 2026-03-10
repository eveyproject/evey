package transcoding_test

import (
	"context"
	"testing"
	"time"

	apptranscode "github.com/gravity-zero/evey/internal/app/transcoding"
	"github.com/gravity-zero/evey/internal/domain/transcoding"
	tcfakes "github.com/gravity-zero/evey/internal/domain/transcoding/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartTranscode(t *testing.T) {
	tc := tcfakes.NewFakeTranscoder()
	hw := tcfakes.NewFakeHWDetector()
	jobRepo := tcfakes.NewFakeTranscodeJobRepository()
	profileRepo := tcfakes.NewFakeTranscodeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	svc := apptranscode.NewService(tc, hw, jobRepo, profileRepo, bus, 2)

	job := &transcoding.TranscodeJob{
		ID:          "j1",
		MediaItemID: "m1",
		InputCodec:  "h264",
		OutputCodec: "h265",
	}

	err := svc.StartTranscode(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, string(transcoding.Running), job.Status)
	assert.Len(t, bus.PublishedEvents(), 1)
}

func newSvcWithCache() (*apptranscode.Service, *tcfakes.FakeCacheStore) {
	tc := tcfakes.NewFakeTranscoder()
	hw := tcfakes.NewFakeHWDetector()
	jobRepo := tcfakes.NewFakeTranscodeJobRepository()
	profileRepo := tcfakes.NewFakeTranscodeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	cache := tcfakes.NewFakeCacheStore()
	svc := apptranscode.NewService(tc, hw, jobRepo, profileRepo, bus, 2)
	svc.WithCacheStore(cache)
	return svc, cache
}

func TestGetCacheInfo_Empty(t *testing.T) {
	svc, _ := newSvcWithCache()
	info, err := svc.GetCacheInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), info.TotalSize)
	assert.Equal(t, 0, info.FileCount)
}

func TestGetCacheInfo_WithFiles(t *testing.T) {
	svc, cache := newSvcWithCache()
	cache.AddFile(transcoding.CacheFileInfo{Name: "a.mp4", Size: 1024, LastModified: time.Now()})
	cache.AddFile(transcoding.CacheFileInfo{Name: "b.mp4", Size: 2048, LastModified: time.Now()})

	info, err := svc.GetCacheInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(3072), info.TotalSize)
	assert.Equal(t, 2, info.FileCount)
}

func TestClearCache(t *testing.T) {
	svc, cache := newSvcWithCache()
	cache.AddFile(transcoding.CacheFileInfo{Name: "a.mp4", Size: 1024})
	cache.AddFile(transcoding.CacheFileInfo{Name: "b.mp4", Size: 2048})

	require.NoError(t, svc.ClearCache(context.Background()))

	info, _ := svc.GetCacheInfo(context.Background())
	assert.Equal(t, 0, info.FileCount)
}

func TestDeleteCacheFile(t *testing.T) {
	svc, cache := newSvcWithCache()
	cache.AddFile(transcoding.CacheFileInfo{Name: "a.mp4", Size: 1024})

	require.NoError(t, svc.DeleteCacheFile(context.Background(), "a.mp4"))

	info, _ := svc.GetCacheInfo(context.Background())
	assert.Equal(t, 0, info.FileCount)
}

func TestDeleteCacheFile_NotFound(t *testing.T) {
	svc, _ := newSvcWithCache()
	err := svc.DeleteCacheFile(context.Background(), "missing.mp4")
	assert.Error(t, err)
}

func TestGetCacheInfo_NoCacheStore(t *testing.T) {
	tc := tcfakes.NewFakeTranscoder()
	hw := tcfakes.NewFakeHWDetector()
	jobRepo := tcfakes.NewFakeTranscodeJobRepository()
	profileRepo := tcfakes.NewFakeTranscodeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := apptranscode.NewService(tc, hw, jobRepo, profileRepo, bus, 2)

	info, err := svc.GetCacheInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), info.TotalSize)
}

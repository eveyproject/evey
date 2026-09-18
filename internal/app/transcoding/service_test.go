package transcoding_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"

	apptranscode "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/evey-project/evey/internal/domain/transcoding"
	tcfakes "github.com/evey-project/evey/internal/domain/transcoding/fakes"
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
		InputPath:   "/media/movie.mkv",
		InputCodec:  "h264",
		OutputCodec: "h265",
	}

	err := svc.StartTranscode(context.Background(), job)
	require.NoError(t, err)
	assert.Equal(t, transcoding.Running, job.Status)
	assert.Len(t, bus.PublishedEvents(), 1)
}

func TestStartTranscode_MaxConcurrentBoundary(t *testing.T) {
	ctx := context.Background()

	t.Run("exactly at limit returns ErrMaxConcurrent", func(t *testing.T) {
		tc := tcfakes.NewFakeTranscoder()
		hw := tcfakes.NewFakeHWDetector()
		jobRepo := tcfakes.NewFakeTranscodeJobRepository()
		profileRepo := tcfakes.NewFakeTranscodeProfileRepository()
		bus := sharedfakes.NewFakeEventBus()
		svc := apptranscode.NewService(tc, hw, jobRepo, profileRepo, bus, 2)

		j1 := &transcoding.TranscodeJob{ID: "j1", MediaItemID: "m1", InputPath: "/a.mkv", InputCodec: "h264", OutputCodec: "h265"}
		j2 := &transcoding.TranscodeJob{ID: "j2", MediaItemID: "m2", InputPath: "/b.mkv", InputCodec: "h264", OutputCodec: "h265"}
		require.NoError(t, svc.StartTranscode(ctx, j1))
		require.NoError(t, svc.StartTranscode(ctx, j2))
		assert.Equal(t, transcoding.Running, j1.Status)
		assert.Equal(t, transcoding.Running, j2.Status)

		j3 := &transcoding.TranscodeJob{ID: "j3", MediaItemID: "m3", InputPath: "/c.mkv", InputCodec: "h264", OutputCodec: "h265"}
		err := svc.StartTranscode(ctx, j3)
		require.ErrorIs(t, err, apptranscode.ErrMaxConcurrent)
		assert.Equal(t, transcoding.Queued, j3.Status)
	})

	t.Run("one below limit succeeds", func(t *testing.T) {
		tc := tcfakes.NewFakeTranscoder()
		hw := tcfakes.NewFakeHWDetector()
		jobRepo := tcfakes.NewFakeTranscodeJobRepository()
		profileRepo := tcfakes.NewFakeTranscodeProfileRepository()
		bus := sharedfakes.NewFakeEventBus()
		svc := apptranscode.NewService(tc, hw, jobRepo, profileRepo, bus, 2)

		j1 := &transcoding.TranscodeJob{ID: "j1", MediaItemID: "m1", InputPath: "/a.mkv", InputCodec: "h264", OutputCodec: "h265"}
		require.NoError(t, svc.StartTranscode(ctx, j1))

		j2 := &transcoding.TranscodeJob{ID: "j2", MediaItemID: "m2", InputPath: "/b.mkv", InputCodec: "h264", OutputCodec: "h265"}
		err := svc.StartTranscode(ctx, j2)
		require.NoError(t, err)
		assert.Equal(t, transcoding.Running, j2.Status)
	})
}

func TestStartTranscode_MaxConcurrentZero_Unlimited(t *testing.T) {
	ctx := context.Background()
	tc := tcfakes.NewFakeTranscoder()
	hw := tcfakes.NewFakeHWDetector()
	jobRepo := tcfakes.NewFakeTranscodeJobRepository()
	profileRepo := tcfakes.NewFakeTranscodeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := apptranscode.NewService(tc, hw, jobRepo, profileRepo, bus, 0)

	for i := range 10 {
		j := &transcoding.TranscodeJob{
			ID: fmt.Sprintf("j%d", i), MediaItemID: shared.MediaItemID(fmt.Sprintf("m%d", i)),
			InputPath: "/a.mkv", InputCodec: "h264", OutputCodec: "h265",
		}
		err := svc.StartTranscode(ctx, j)
		require.NoError(t, err, "maxConcurrent=0 should allow unlimited transcodes")
		assert.Equal(t, transcoding.Running, j.Status)
	}
}

func TestStartTranscode_MaxConcurrentOne_RejectsSecond(t *testing.T) {
	ctx := context.Background()
	tc := tcfakes.NewFakeTranscoder()
	hw := tcfakes.NewFakeHWDetector()
	jobRepo := tcfakes.NewFakeTranscodeJobRepository()
	profileRepo := tcfakes.NewFakeTranscodeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := apptranscode.NewService(tc, hw, jobRepo, profileRepo, bus, 1)

	j1 := &transcoding.TranscodeJob{ID: "j1", MediaItemID: "m1", InputPath: "/a.mkv", InputCodec: "h264", OutputCodec: "h265"}
	require.NoError(t, svc.StartTranscode(ctx, j1))
	assert.Equal(t, transcoding.Running, j1.Status)

	j2 := &transcoding.TranscodeJob{ID: "j2", MediaItemID: "m2", InputPath: "/b.mkv", InputCodec: "h264", OutputCodec: "h265"}
	err := svc.StartTranscode(ctx, j2)
	require.ErrorIs(t, err, apptranscode.ErrMaxConcurrent)
	assert.Equal(t, transcoding.Queued, j2.Status)

	// The queue-depth gauge must reflect the queued job. It was declared and
	// registered but never fed - a permanent 0 that lied about saturation.
	assert.InDelta(t, 1.0, testutil.ToFloat64(metrics.TranscodeQueueDepth), 0.001,
		"one job queued for a concurrency slot must show as queue depth 1")
	assert.InDelta(t, 1.0, testutil.ToFloat64(metrics.TranscodesActive), 0.001,
		"the running job must still show as one active transcode")
}

func TestStartTranscode_TranscoderError_FailsJob(t *testing.T) {
	ctx := context.Background()
	tc := tcfakes.NewFakeTranscoder()
	hw := tcfakes.NewFakeHWDetector()
	jobRepo := tcfakes.NewFakeTranscodeJobRepository()
	profileRepo := tcfakes.NewFakeTranscodeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := apptranscode.NewService(tc, hw, jobRepo, profileRepo, bus, 2)

	tc.Err = assert.AnError
	j := &transcoding.TranscodeJob{ID: "j1", MediaItemID: "m1", InputPath: "/a.mkv", InputCodec: "h264", OutputCodec: "h265"}
	err := svc.StartTranscode(ctx, j)
	require.Error(t, err)

	stored, _ := jobRepo.GetByID(ctx, "j1")
	assert.Equal(t, transcoding.Failed, stored.Status)
	assert.NotEmpty(t, stored.ErrorMessage)

	var found bool
	for _, evt := range bus.PublishedEvents() {
		if evt.EventName() == "transcode.failed" {
			found = true
		}
	}
	assert.True(t, found, "TranscodeFailed event should be published")
}

func newSvcWithCache() (*apptranscode.Service, *tcfakes.FakeCacheStore) {
	tc := tcfakes.NewFakeTranscoder()
	hw := tcfakes.NewFakeHWDetector()
	jobRepo := tcfakes.NewFakeTranscodeJobRepository()
	profileRepo := tcfakes.NewFakeTranscodeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	cache := tcfakes.NewFakeCacheStore()
	svc := apptranscode.NewService(tc, hw, jobRepo, profileRepo, bus, 2)
	apptranscode.WithCacheStore(cache)(svc)
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
	assert.ErrorContains(t, err, "not found")
}

func newSvc() *apptranscode.Service {
	tc := tcfakes.NewFakeTranscoder()
	hw := tcfakes.NewFakeHWDetector()
	jobRepo := tcfakes.NewFakeTranscodeJobRepository()
	profileRepo := tcfakes.NewFakeTranscodeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	return apptranscode.NewService(tc, hw, jobRepo, profileRepo, bus, 2)
}

func TestListTranscodeProfiles_Empty(t *testing.T) {
	svc := newSvc()
	profiles, err := svc.ListProfiles(context.Background())
	require.NoError(t, err)
	assert.Empty(t, profiles)
}

func TestListTranscodeProfiles_WithItems(t *testing.T) {
	tc := tcfakes.NewFakeTranscoder()
	hw := tcfakes.NewFakeHWDetector()
	jobRepo := tcfakes.NewFakeTranscodeJobRepository()
	profileRepo := tcfakes.NewFakeTranscodeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := apptranscode.NewService(tc, hw, jobRepo, profileRepo, bus, 2)

	require.NoError(t, profileRepo.Create(context.Background(), &transcoding.TranscodeProfile{
		ID: "p1", Name: "720p", VideoCodec: "h264", AudioCodec: "aac", Container: "mp4",
	}))
	require.NoError(t, profileRepo.Create(context.Background(), &transcoding.TranscodeProfile{
		ID: "p2", Name: "1080p", VideoCodec: "hevc", AudioCodec: "aac", Container: "mp4",
	}))

	profiles, err := svc.ListProfiles(context.Background())
	require.NoError(t, err)
	assert.Len(t, profiles, 2)
}

func TestCreateTranscodeProfile(t *testing.T) {
	svc := newSvc()
	p := &transcoding.TranscodeProfile{
		ID:           "p1",
		Name:         "480p",
		VideoCodec:   "h264",
		AudioCodec:   "aac",
		Container:    "mp4",
		MaxWidth:     854,
		MaxHeight:    480,
		VideoBitrate: 2000000,
	}
	require.NoError(t, svc.CreateProfile(context.Background(), p))

	profiles, err := svc.ListProfiles(context.Background())
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, "480p", profiles[0].Name)
	assert.Equal(t, "h264", profiles[0].VideoCodec)
}

func TestUpdateTranscodeProfile(t *testing.T) {
	svc := newSvc()
	p := &transcoding.TranscodeProfile{ID: "p1", Name: "720p", VideoCodec: "h264", AudioCodec: "aac", Container: "mp4"}
	require.NoError(t, svc.CreateProfile(context.Background(), p))

	p.Name = "720p-updated"
	p.VideoBitrate = 5000000
	require.NoError(t, svc.UpdateProfile(context.Background(), p))

	profiles, err := svc.ListProfiles(context.Background())
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, "720p-updated", profiles[0].Name)
	assert.Equal(t, int64(5000000), profiles[0].VideoBitrate)
}

func TestDeleteTranscodeProfile(t *testing.T) {
	svc := newSvc()
	p := &transcoding.TranscodeProfile{ID: "p1", Name: "720p", VideoCodec: "h264", AudioCodec: "aac", Container: "mp4"}
	require.NoError(t, svc.CreateProfile(context.Background(), p))

	require.NoError(t, svc.DeleteProfile(context.Background(), "p1"))

	profiles, err := svc.ListProfiles(context.Background())
	require.NoError(t, err)
	assert.Empty(t, profiles)
}

func TestGetTranscodeStatus(t *testing.T) {
	tc := tcfakes.NewFakeTranscoder()
	hw := tcfakes.NewFakeHWDetector()
	jobRepo := tcfakes.NewFakeTranscodeJobRepository()
	profileRepo := tcfakes.NewFakeTranscodeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := apptranscode.NewService(tc, hw, jobRepo, profileRepo, bus, 2)

	tc.SetProgress("j1", &transcoding.TranscodeProgress{
		JobID:   "j1",
		Percent: 75.0,
		FPS:     24.0,
		Speed:   2.0,
	})

	progress, err := svc.GetProgress(context.Background(), "j1")
	require.NoError(t, err)
	assert.Equal(t, "j1", progress.JobID)
	assert.InDelta(t, 75.0, progress.Percent, 0.001)
	assert.InDelta(t, 24.0, progress.FPS, 0.001)
	assert.InDelta(t, 2.0, progress.Speed, 0.001)
}

func TestGetTranscodeStatus_Default(t *testing.T) {
	svc := newSvc()
	progress, err := svc.GetProgress(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.Equal(t, "nonexistent", progress.JobID)
	assert.InDelta(t, 0.0, progress.Percent, 0.001)
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

func newProfileSvc() (*apptranscode.Service, *tcfakes.FakeTranscodeProfileRepository) {
	tc := tcfakes.NewFakeTranscoder()
	hw := tcfakes.NewFakeHWDetector()
	jobRepo := tcfakes.NewFakeTranscodeJobRepository()
	profileRepo := tcfakes.NewFakeTranscodeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := apptranscode.NewService(tc, hw, jobRepo, profileRepo, bus, 2)
	return svc, profileRepo
}

func TestCreateProfile(t *testing.T) {
	svc, _ := newProfileSvc()
	p := &transcoding.TranscodeProfile{
		ID:           "tp-1",
		Name:         "1080p",
		VideoCodec:   "h264",
		AudioCodec:   "aac",
		Container:    "mp4",
		MaxWidth:     1920,
		MaxHeight:    1080,
		VideoBitrate: 8000000,
	}
	require.NoError(t, svc.CreateProfile(context.Background(), p))

	profiles, err := svc.ListProfiles(context.Background())
	require.NoError(t, err)
	assert.Len(t, profiles, 1)
	assert.Equal(t, "1080p", profiles[0].Name)
}

func TestListProfiles_Empty(t *testing.T) {
	svc, _ := newProfileSvc()
	profiles, err := svc.ListProfiles(context.Background())
	require.NoError(t, err)
	assert.Empty(t, profiles)
}

func TestUpdateProfile(t *testing.T) {
	svc, _ := newProfileSvc()
	p := &transcoding.TranscodeProfile{ID: "tp-1", Name: "720p", VideoCodec: "h264", Container: "mp4"}
	require.NoError(t, svc.CreateProfile(context.Background(), p))

	p.Name = "720p-updated"
	require.NoError(t, svc.UpdateProfile(context.Background(), p))

	profiles, err := svc.ListProfiles(context.Background())
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, "720p-updated", profiles[0].Name)
}

func TestUpdateProfile_NotFound(t *testing.T) {
	svc, _ := newProfileSvc()
	err := svc.UpdateProfile(context.Background(), &transcoding.TranscodeProfile{ID: "missing", Name: "x", VideoCodec: "h264", Container: "mp4"})
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestDeleteProfile(t *testing.T) {
	svc, _ := newProfileSvc()
	p := &transcoding.TranscodeProfile{ID: "tp-1", Name: "720p", VideoCodec: "h264", Container: "mp4"}
	require.NoError(t, svc.CreateProfile(context.Background(), p))

	require.NoError(t, svc.DeleteProfile(context.Background(), "tp-1"))

	profiles, err := svc.ListProfiles(context.Background())
	require.NoError(t, err)
	assert.Empty(t, profiles)
}

func TestDeleteProfile_NotFound(t *testing.T) {
	svc, _ := newProfileSvc()
	err := svc.DeleteProfile(context.Background(), "missing")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func newFullSvc() (*apptranscode.Service, *tcfakes.FakeTranscoder, *tcfakes.FakeTranscodeJobRepository, *sharedfakes.FakeEventBus) {
	tc := tcfakes.NewFakeTranscoder()
	hw := tcfakes.NewFakeHWDetector()
	jobRepo := tcfakes.NewFakeTranscodeJobRepository()
	profileRepo := tcfakes.NewFakeTranscodeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := apptranscode.NewService(tc, hw, jobRepo, profileRepo, bus, 2)
	return svc, tc, jobRepo, bus
}

func startJob(t *testing.T, svc *apptranscode.Service, id, mediaID string) *transcoding.TranscodeJob {
	t.Helper()
	job := &transcoding.TranscodeJob{
		ID:          id,
		MediaItemID: shared.MediaItemID(mediaID),
		InputPath:   "/media/movie.mkv",
		InputCodec:  "h264",
		OutputCodec: "h265",
	}
	require.NoError(t, svc.StartTranscode(context.Background(), job))
	return job
}

func TestStopTranscode_HappyPath(t *testing.T) {
	svc, _, jobRepo, bus := newFullSvc()
	startJob(t, svc, "j1", "m1")

	err := svc.StopTranscode(context.Background(), "j1")
	require.NoError(t, err)

	job, err := jobRepo.GetByID(context.Background(), "j1")
	require.NoError(t, err)
	assert.Equal(t, transcoding.Cancelled, job.Status)
	assert.NotNil(t, job.EndedAt)

	events := bus.PublishedEvents()
	require.Len(t, events, 2)
	cancelled, ok := events[1].(transcoding.TranscodeCancelled)
	require.True(t, ok)
	assert.Equal(t, "j1", cancelled.JobID)
	assert.Equal(t, shared.MediaItemID("m1"), cancelled.MediaItemID)
}

func TestStopTranscode_NonExistentJob(t *testing.T) {
	svc, _, _, _ := newFullSvc()

	err := svc.StopTranscode(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestStopTranscode_EventPublished(t *testing.T) {
	svc, _, _, bus := newFullSvc()
	startJob(t, svc, "j1", "m1")

	require.NoError(t, svc.StopTranscode(context.Background(), "j1"))

	var found bool
	for _, evt := range bus.PublishedEvents() {
		if evt.EventName() == "transcode.cancelled" {
			c := evt.(transcoding.TranscodeCancelled)
			assert.Equal(t, "j1", c.JobID)
			assert.Equal(t, shared.MediaItemID("m1"), c.MediaItemID)
			assert.False(t, c.OccurredAt.IsZero())
			found = true
		}
	}
	assert.True(t, found, "TranscodeCancelled event not published")
}

func TestStopTranscode_ConcurrentStop(t *testing.T) {
	svc, _, _, _ := newFullSvc()
	startJob(t, svc, "j1", "m1")

	errs := make(chan error, 2)
	for range 2 {
		go func() {
			errs <- svc.StopTranscode(context.Background(), "j1")
		}()
	}

	var successes, failures int
	for range 2 {
		if err := <-errs; err != nil {
			failures++
		} else {
			successes++
		}
	}
	assert.GreaterOrEqual(t, successes, 1, "at least one stop should succeed")
}

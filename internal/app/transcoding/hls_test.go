package transcoding_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	apptranscode "github.com/gravity-zero/evey/internal/app/transcoding"
	"github.com/gravity-zero/evey/internal/domain/transcoding"
	"github.com/gravity-zero/evey/internal/domain/transcoding/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsImageSubtitle(t *testing.T) {
	imageCodes := []string{"pgssub", "PGSSUB", "hdmv_pgs_subtitle", "dvd_subtitle", "dvbsub", "xsub"}
	for _, c := range imageCodes {
		assert.True(t, apptranscode.IsImageSubtitle(c), c)
	}
	textCodes := []string{"subrip", "ass", "webvtt", "srt", ""}
	for _, c := range textCodes {
		assert.False(t, apptranscode.IsImageSubtitle(c), c)
	}
}

func newTestManager(t *testing.T, maxConcurrent int) (*apptranscode.HLSManager, *fakes.HLSProcess) {
	t.Helper()
	dir := t.TempDir()
	fake := &fakes.HLSProcess{}
	m := apptranscode.NewHLSManager(context.Background(), dir, maxConcurrent, fake)
	t.Cleanup(func() { m.CleanupAll() })
	return m, fake
}

func TestFilterProfiles_1080pSource(t *testing.T) {
	result := apptranscode.FilterProfiles(transcoding.DefaultHLSProfiles, 1920, 1080)
	assert.Len(t, result, 4)
}

func TestFilterProfiles_720pSource(t *testing.T) {
	result := apptranscode.FilterProfiles(transcoding.DefaultHLSProfiles, 1280, 720)
	assert.Len(t, result, 3)
	for _, p := range result {
		assert.LessOrEqual(t, p.Width, 1280)
		assert.LessOrEqual(t, p.Height, 720)
	}
}

func TestFilterProfiles_480pSource(t *testing.T) {
	result := apptranscode.FilterProfiles(transcoding.DefaultHLSProfiles, 854, 480)
	assert.Len(t, result, 2)
	assert.Equal(t, "360p", result[0].Name)
	assert.Equal(t, "480p", result[1].Name)
}

func TestFilterProfiles_LowRes(t *testing.T) {
	result := apptranscode.FilterProfiles(transcoding.DefaultHLSProfiles, 320, 240)
	assert.Len(t, result, 1, "should keep at least 1 profile")
	assert.Equal(t, "360p", result[0].Name)
}

func TestFilterProfiles_EmptyInput(t *testing.T) {
	result := apptranscode.FilterProfiles(nil, 1920, 1080)
	assert.Empty(t, result)
}

func TestNewHLSManager(t *testing.T) {
	m, _ := newTestManager(t, 3)
	assert.NotNil(t, m)
	_, ok := m.GetSession("nonexistent")
	assert.False(t, ok)
}

func TestGetOrCreate_MaxConcurrent_QueueTimeout(t *testing.T) {
	m, _ := newTestManager(t, 1)
	m.SetQueueTimeout(50 * time.Millisecond)

	_, err := m.GetOrCreate("user1", "media1", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)

	_, err = m.GetOrCreate("user1", "media2", "/b.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many concurrent sessions")
}

func TestGetOrCreate_MaxConcurrent_QueueSuccess(t *testing.T) {
	m, _ := newTestManager(t, 1)
	m.SetQueueTimeout(2 * time.Second)

	_, err := m.GetOrCreate("user1", "media1", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		_, err := m.GetOrCreate("user2", "media2", "/b.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
		done <- err
	}()

	time.Sleep(50 * time.Millisecond)
	m.RemoveSession("media1")

	select {
	case err := <-done:
		require.NoError(t, err, "queued session should succeed after slot freed")
	case <-time.After(3 * time.Second):
		t.Fatal("queued session did not complete in time")
	}
}

func TestGetOrCreate_ReturnsSameSession(t *testing.T) {
	m, _ := newTestManager(t, 5)

	s1, err := m.GetOrCreate("user1", "media1", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)

	s2, err := m.GetOrCreate("user1", "media1", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)
	assert.Equal(t, s1, s2)
}

func TestGetOrCreate_MaxUserStreams(t *testing.T) {
	m, _ := newTestManager(t, 10)

	_, err := m.GetOrCreate("user1", "media-u1a", "/a.mkv", 1920, 1080, 1, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)

	_, err = m.GetOrCreate("user1", "media-u1b", "/b.mkv", 1920, 1080, 1, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max streams per user")

	_, err = m.GetOrCreate("user2", "media-u2a", "/c.mkv", 1920, 1080, 1, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)
}

func TestSessionOutputDir(t *testing.T) {
	dir := t.TempDir()
	fake := &fakes.HLSProcess{}
	m := apptranscode.NewHLSManager(context.Background(), dir, 5, fake)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("user1", "my-media-id", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "my-media-id"), sess.OutputDir)
}

func TestSessionMarkAccess(t *testing.T) {
	m, _ := newTestManager(t, 5)

	sess, err := m.GetOrCreate("user1", "media-access", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)

	before := sess.LastAccess
	time.Sleep(5 * time.Millisecond)
	sess.MarkAccess()
	assert.True(t, sess.LastAccess.After(before))
}

func TestWaitReady_WithFake(t *testing.T) {
	m, _ := newTestManager(t, 5)

	sess, err := m.GetOrCreate("user1", "ready-test", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)

	err = sess.WaitReady(context.Background())
	assert.NoError(t, err)
}

func TestCleanupAll_RemovesOutputDirs(t *testing.T) {
	dir := t.TempDir()
	fake := &fakes.HLSProcess{}
	m := apptranscode.NewHLSManager(context.Background(), dir, 5, fake)

	sess, err := m.GetOrCreate("user1", "cleanup-test", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)

	outputDir := sess.OutputDir
	require.DirExists(t, outputDir)
	os.WriteFile(filepath.Join(outputDir, "test.ts"), []byte("data"), 0644)

	m.CleanupAll()
	_, err = os.Stat(outputDir)
	assert.True(t, os.IsNotExist(err))
}

func TestSessionCleanup(t *testing.T) {
	m, _ := newTestManager(t, 3)

	sess, err := m.GetOrCreate("user1", "test-media", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)
	require.NotNil(t, sess)

	_, ok := m.GetSession("test-media")
	assert.True(t, ok)

	m.CleanupAll()
	_, ok = m.GetSession("test-media")
	assert.False(t, ok)
}

func TestCleanupStale_10MinThreshold(t *testing.T) {
	m, _ := newTestManager(t, 5)

	sess, err := m.GetOrCreate("u", "stale-test", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)

	sess.LastAccess = time.Now().Add(-11 * time.Minute)

	_, ok := m.GetSession("stale-test")
	require.True(t, ok)

	m.CleanupStale(10 * time.Minute)

	_, ok = m.GetSession("stale-test")
	assert.False(t, ok, "session idle >10min should be cleaned up")
}

func TestCleanupStale_RecentSessionKept(t *testing.T) {
	m, _ := newTestManager(t, 5)

	_, err := m.GetOrCreate("u", "recent-test", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)

	m.CleanupStale(10 * time.Minute)

	_, ok := m.GetSession("recent-test")
	assert.True(t, ok, "recently accessed session should not be cleaned up")
}

func TestIsTranscodeComplete_Complete(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "master.m3u8"), []byte("#EXTM3U\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stream_0.m3u8"), []byte("#EXTM3U\n#EXT-X-ENDLIST\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stream_1.m3u8"), []byte("#EXTM3U\n#EXT-X-ENDLIST\n"), 0644))
	assert.True(t, apptranscode.IsTranscodeComplete(dir, 2))
}

func TestIsTranscodeComplete_Incomplete(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "master.m3u8"), []byte("#EXTM3U\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stream_0.m3u8"), []byte("#EXTM3U\n#EXT-X-ENDLIST\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stream_1.m3u8"), []byte("#EXTM3U\n"), 0644))
	assert.False(t, apptranscode.IsTranscodeComplete(dir, 2))
}

func TestIsTranscodeComplete_MissingFiles(t *testing.T) {
	dir := t.TempDir()
	assert.False(t, apptranscode.IsTranscodeComplete(dir, 2))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "master.m3u8"), []byte("#EXTM3U\n"), 0644))
	assert.False(t, apptranscode.IsTranscodeComplete(dir, 1))
}

func TestIsTranscodeComplete_SingleStream(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "master.m3u8"), []byte("#EXTM3U\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stream_0.m3u8"), []byte("#EXTM3U\n#EXT-X-ENDLIST\n"), 0644))
	assert.True(t, apptranscode.IsTranscodeComplete(dir, 1))
}

func TestIsSingleStream(t *testing.T) {
	m, _ := newTestManager(t, 10)

	copy, _ := m.GetOrCreate("u", "ss-copy", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	assert.True(t, copy.IsSingleStream())

	tc, _ := m.GetOrCreate("u", "ss-tc", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "transcode", "transcode")
	assert.True(t, tc.IsSingleStream())

	multi, _ := m.GetOrCreate("u", "ss-multi", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "", "")
	assert.False(t, multi.IsSingleStream())
}

func TestGetOrCreate_DelegatestoProcess(t *testing.T) {
	m, fake := newTestManager(t, 5)

	sess, err := m.GetOrCreate("u", "delegate-test", "/f.mkv", 1920, 1080, 0, 1, 2, "pgssub", nil, true, true, 42.5, "transcode", "transcode")
	require.NoError(t, err)
	require.NoError(t, sess.WaitReady(context.Background()))

	require.Len(t, fake.Started, 1)
	opts := fake.Started[0]
	assert.Equal(t, "/f.mkv", opts.FilePath)
	assert.Equal(t, "transcode", opts.VideoDecision)
	assert.Equal(t, "transcode", opts.AudioDecision)
	assert.Equal(t, 1, opts.AudioTrackIndex)
	assert.Equal(t, 2, opts.SubtitleTrackIndex)
	assert.Equal(t, "pgssub", opts.SubtitleCodec)
	assert.True(t, opts.HDRSupported)
	assert.True(t, opts.AudioNormalize)
	assert.InEpsilon(t, 42.5, opts.StartSec, 1e-9)
}

func TestActiveSessionCount(t *testing.T) {
	m, _ := newTestManager(t, 5)

	assert.Equal(t, 0, m.ActiveSessionCount())

	_, _ = m.GetOrCreate("u", "m1", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	assert.Equal(t, 1, m.ActiveSessionCount())

	_, _ = m.GetOrCreate("u", "m2", "/b.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	assert.Equal(t, 2, m.ActiveSessionCount())
}

func TestRestartFromPosition(t *testing.T) {
	m, fake := newTestManager(t, 5)

	s1, err := m.GetOrCreate("u", "restart-test", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)
	require.NoError(t, s1.WaitReady(context.Background()))

	s2, err := m.RestartFromPosition("u", "restart-test", "/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 120.0, "copy", "transcode")
	require.NoError(t, err)
	require.NoError(t, s2.WaitReady(context.Background()))

	assert.NotEqual(t, s1, s2)
	assert.Equal(t, 1, m.ActiveSessionCount())
	assert.Len(t, fake.Started, 2)
}

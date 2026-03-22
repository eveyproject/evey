package transcoding_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apptranscode "github.com/gravity-zero/evey/internal/app/transcoding"
	"github.com/gravity-zero/evey/internal/domain/library"
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

func TestBuildFFmpegArgs_NoBurnIn_MultiVariant(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	// Empty decisions → legacy multi-variant ABR
	sess, err := m.GetOrCreate("u", "m", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "", "")
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "[0:v]split=")
	assert.NotContains(t, joined, "overlay")
}

func TestBuildFFmpegArgs_ModeA_VideoCopy(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("u", "m-copy", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-c:v copy")
	assert.Contains(t, joined, "-c:a aac")
	assert.NotContains(t, joined, "filter_complex")
	assert.NotContains(t, joined, "split=")
	assert.NotContains(t, joined, "libx264")
}

func TestBuildFFmpegArgs_ModeA_AllCopy(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("u", "m-allcopy", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "copy")
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-c:v copy")
	assert.Contains(t, joined, "-c:a copy")
	assert.NotContains(t, joined, "filter_complex")
}

func TestBuildFFmpegArgs_ModeB_SingleTranscode(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("u", "m-transcode", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "transcode", "transcode")
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "libx264")
	assert.Contains(t, joined, "-c:a aac")
	assert.NotContains(t, joined, "split=")
	assert.NotContains(t, joined, "var_stream_map")
	assert.Contains(t, joined, "stream_0.m3u8")
}

func TestBuildFFmpegArgs_BurnIn(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	// Burn-in with multi-variant (empty decisions)
	sess, err := m.GetOrCreate("u", "m2", "/f.mkv", 1920, 1080, 0, 0, 2, "hdmv_pgs_subtitle", nil, false, false, 0, "", "")
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "[0:v][0:s:2]overlay[vb]")
	assert.Contains(t, joined, "[vb]split=")
	assert.NotContains(t, joined, "[0:v]split=")
}

func TestFilterProfiles_1080pSource(t *testing.T) {
	result := apptranscode.FilterProfiles(apptranscode.DefaultProfiles, 1920, 1080)
	assert.Len(t, result, 4)
}

func TestFilterProfiles_720pSource(t *testing.T) {
	result := apptranscode.FilterProfiles(apptranscode.DefaultProfiles, 1280, 720)
	assert.Len(t, result, 3)
	for _, p := range result {
		assert.LessOrEqual(t, p.Width, 1280)
		assert.LessOrEqual(t, p.Height, 720)
	}
}

func TestFilterProfiles_480pSource(t *testing.T) {
	result := apptranscode.FilterProfiles(apptranscode.DefaultProfiles, 854, 480)
	assert.Len(t, result, 2)
	assert.Equal(t, "360p", result[0].Name)
	assert.Equal(t, "480p", result[1].Name)
}

func TestFilterProfiles_LowRes(t *testing.T) {
	result := apptranscode.FilterProfiles(apptranscode.DefaultProfiles, 320, 240)
	assert.Len(t, result, 1, "should keep at least 1 profile")
	assert.Equal(t, "360p", result[0].Name)
}

func TestNewHLSManager(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 3)
	defer m.CleanupAll()

	assert.NotNil(t, m)
	_, ok := m.GetSession("nonexistent")
	assert.False(t, ok)
}

func TestSessionCleanup(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 3)
	defer m.CleanupAll()

	// GetOrCreate will fail to start ffmpeg (no real file) but the session is created
	sess, err := m.GetOrCreate("user1", "test-media", "/nonexistent/file.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "", "")
	require.NoError(t, err)
	require.NotNil(t, sess)

	_, ok := m.GetSession("test-media")
	assert.True(t, ok)

	outputDir := sess.OutputDir
	assert.DirExists(t, outputDir)

	m.CleanupAll()
	_, ok = m.GetSession("test-media")
	assert.False(t, ok)
}

func TestGetOrCreate_MaxConcurrent(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 1)
	defer m.CleanupAll()

	_, err := m.GetOrCreate("user1", "media1", "/nonexistent/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "", "")
	require.NoError(t, err)

	_, err = m.GetOrCreate("user1", "media2", "/nonexistent/b.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max concurrent")
}

func TestGetOrCreate_ReturnsSameSession(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	s1, err := m.GetOrCreate("user1", "media1", "/nonexistent/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "", "")
	require.NoError(t, err)

	s2, err := m.GetOrCreate("user1", "media1", "/nonexistent/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "", "")
	require.NoError(t, err)
	assert.Equal(t, s1, s2)
}

func TestSessionOutputDir(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("user1", "my-media-id", "/nonexistent/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "", "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "my-media-id"), sess.OutputDir)
}

func TestFilterProfiles_EmptyInput(t *testing.T) {
	result := apptranscode.FilterProfiles(nil, 1920, 1080)
	assert.Empty(t, result)
}

func TestSessionMarkAccess(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("user1", "media-access", "/nonexistent/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "", "")
	require.NoError(t, err)

	before := sess.LastAccess
	time.Sleep(5 * time.Millisecond)
	sess.MarkAccess()
	assert.True(t, sess.LastAccess.After(before))
}

func TestWaitReady_FailsOnBadFile(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("user1", "bad-file", "/nonexistent/file.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "", "")
	require.NoError(t, err)

	// ffmpeg will fail since the file doesn't exist; WaitReady returns error
	err = sess.WaitReady(t.Context())
	assert.Error(t, err)
}

func TestGetOrCreate_MaxUserStreams(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 10)
	defer m.CleanupAll()

	_, err := m.GetOrCreate("user1", "media-u1a", "/nonexistent/a.mkv", 1920, 1080, 1, 0, -1, "", nil, false, false, 0, "", "")
	require.NoError(t, err)

	_, err = m.GetOrCreate("user1", "media-u1b", "/nonexistent/b.mkv", 1920, 1080, 1, 0, -1, "", nil, false, false, 0, "", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max streams per user")

	// different user must succeed
	_, err = m.GetOrCreate("user2", "media-u2a", "/nonexistent/c.mkv", 1920, 1080, 1, 0, -1, "", nil, false, false, 0, "", "")
	require.NoError(t, err)
}

func TestCleanupAll_RemovesOutputDirs(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)

	sess, err := m.GetOrCreate("user1", "cleanup-test", "/nonexistent/a.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "", "")
	require.NoError(t, err)

	outputDir := sess.OutputDir
	require.DirExists(t, outputDir)
	os.WriteFile(filepath.Join(outputDir, "test.ts"), []byte("data"), 0644)

	m.CleanupAll()
	_, err = os.Stat(outputDir)
	assert.True(t, os.IsNotExist(err))
}

func TestBuildFFmpegArgs_HDRToneMapping(t *testing.T) {
	hdrType := "HDR10"
	w, h := 3840, 2160
	streams := []*library.MediaStream{
		{StreamType: "video", HDRType: &hdrType, Width: &w, Height: &h},
	}

	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	// HDR source, client does NOT support HDR → tone mapping must be injected
	sess, err := m.GetOrCreate("u", "hdr-media", "/f.mkv", w, h, 0, 0, -1, "", streams, false, false, 0, "", "")
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "zscale", "tone mapping filter expected")
	assert.Contains(t, joined, "tonemap=hable", "hable algorithm expected")
	assert.NotContains(t, joined, "[0:v]split=", "should not split directly from input when tone-mapping")
}

func TestBuildFFmpegArgs_HDRToneMapping_HDRClientSkips(t *testing.T) {
	hdrType := "HDR10"
	w, h := 3840, 2160
	streams := []*library.MediaStream{
		{StreamType: "video", HDRType: &hdrType, Width: &w, Height: &h},
	}

	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	// HDR source, client supports HDR → no tone mapping
	sess, err := m.GetOrCreate("u", "hdr-media-hdr-client", "/f.mkv", w, h, 0, 0, -1, "", streams, true, false, 0, "", "")
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	joined := strings.Join(args, " ")
	assert.NotContains(t, joined, "zscale", "no tone mapping when client supports HDR")
	assert.Contains(t, joined, "[0:v]split=")
}

func TestBuildFFmpegArgs_SDRNoToneMapping(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	// SDR source → no tone mapping regardless of hdrSupported
	sess, err := m.GetOrCreate("u", "sdr-media", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "", "")
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	joined := strings.Join(args, " ")
	assert.NotContains(t, joined, "zscale")
	assert.Contains(t, joined, "[0:v]split=")
}

func TestBuildFFmpegArgs_AudioNormalize(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("u", "audio-norm", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, true, 0, "", "")
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "loudnorm=I=-16:TP=-1.5:LRA=11:linear=true")
}

// --- Mode A: AudioTrackIndex != 0 ---

func TestBuildFFmpegArgs_ModeA_AudioTrackIndex(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("u", "m-copy-at2", "/f.mkv", 1920, 1080, 0, 2, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-map 0:a:2", "should map audio track index 2")
	assert.Contains(t, joined, "-c:v copy")
	assert.Contains(t, joined, "-c:a aac")
	assert.Contains(t, joined, "-ac 2")
}

// --- Mode B: audio copy (no -ac 2) ---

func TestBuildFFmpegArgs_ModeB_AudioCopy(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("u", "m-transcode-acopy", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "transcode", "copy")
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-c:v libx264")
	assert.Contains(t, joined, "-c:a copy")
	assert.NotContains(t, joined, "-ac 2", "audio copy must not force stereo downmix")
}

// --- Verify -ac 2 present when audio is transcoded ---

func TestBuildFFmpegArgs_ModeA_AudioTranscode_AC2(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("u", "m-ac2-a", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-ac 2")
}

func TestBuildFFmpegArgs_ModeB_AudioTranscode_AC2(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("u", "m-ac2-b", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "transcode", "transcode")
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-ac 2")
}

// --- Verify scale=WIDTH:-2,setsar=1 format in transcode modes ---

func TestBuildFFmpegArgs_ModeB_ScaleFormat(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("u", "m-scale-fmt", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "transcode", "transcode")
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	joined := strings.Join(args, " ")
	// scale uses WIDTH:-2 (not W:H), with setsar=1
	assert.Contains(t, joined, "scale=1920:-2,setsar=1")
}

func TestBuildFFmpegArgs_Legacy_ScaleFormat(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("u", "m-scale-legacy", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "", "")
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	joined := strings.Join(args, " ")
	// Each profile's scale filter uses WIDTH:-2,setsar=1
	assert.Contains(t, joined, "scale=640:-2,setsar=1")
	assert.Contains(t, joined, "scale=1920:-2,setsar=1")
}

// --- Verify -profile:v high -level:v 4.1 -pix_fmt yuv420p in transcode modes ---

func TestBuildFFmpegArgs_ModeB_TranscodeProfile(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("u", "m-profile", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "transcode", "transcode")
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-profile:v high")
	assert.Contains(t, joined, "-level:v 4.1")
	assert.Contains(t, joined, "-pix_fmt yuv420p")
}

// --- Verify -ss positioning (before -i) for all modes ---

func TestBuildFFmpegArgs_ModeA_SeekBeforeInput(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("u", "m-seek-a", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 42.5, "copy", "transcode")
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	ssIdx := -1
	iIdx := -1
	for i, a := range args {
		if a == "-ss" && ssIdx < 0 {
			ssIdx = i
		}
		if a == "-i" && iIdx < 0 {
			iIdx = i
		}
	}
	require.Greater(t, ssIdx, -1, "-ss must be present")
	require.Greater(t, iIdx, -1, "-i must be present")
	assert.Less(t, ssIdx, iIdx, "-ss must come before -i")
}

func TestBuildFFmpegArgs_ModeB_SeekBeforeInput(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("u", "m-seek-b", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 10.0, "transcode", "transcode")
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	ssIdx := -1
	iIdx := -1
	for i, a := range args {
		if a == "-ss" && ssIdx < 0 {
			ssIdx = i
		}
		if a == "-i" && iIdx < 0 {
			iIdx = i
		}
	}
	require.Greater(t, ssIdx, -1, "-ss must be present")
	require.Greater(t, iIdx, -1, "-i must be present")
	assert.Less(t, ssIdx, iIdx, "-ss must come before -i")
}

func TestBuildFFmpegArgs_Legacy_SeekBeforeInput(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("u", "m-seek-legacy", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 5.0, "", "")
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	ssIdx := -1
	iIdx := -1
	for i, a := range args {
		if a == "-ss" && ssIdx < 0 {
			ssIdx = i
		}
		if a == "-i" && iIdx < 0 {
			iIdx = i
		}
	}
	require.Greater(t, ssIdx, -1, "-ss must be present")
	require.Greater(t, iIdx, -1, "-i must be present")
	assert.Less(t, ssIdx, iIdx, "-ss must come before -i")
}

func TestBuildFFmpegArgs_NoSeekWhenZero(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("u", "m-noseek", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	joined := strings.Join(args, " ")
	assert.NotContains(t, joined, "-ss")
}

// --- Mode C (video=transcode, audio=transcode) full verification ---

func TestBuildFFmpegArgs_ModeC_FullTranscode(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("u", "m-full-tc", "/f.mkv", 1920, 1080, 0, 1, -1, "", nil, false, false, 0, "transcode", "transcode")
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "-c:v libx264")
	assert.Contains(t, joined, "-c:a aac")
	assert.Contains(t, joined, "-ac 2")
	assert.Contains(t, joined, "-map 0:a:1")
	assert.Contains(t, joined, "-profile:v high")
	assert.Contains(t, joined, "-pix_fmt yuv420p")
	assert.NotContains(t, joined, "split=")
	assert.NotContains(t, joined, "var_stream_map")
}

// --- Cleanup stale: 10 minute threshold ---

func TestCleanupStale_10MinThreshold(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("u", "stale-test", "/nonexistent/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "", "")
	require.NoError(t, err)

	// Manually set LastAccess to 11 minutes ago
	sess.LastAccess = time.Now().Add(-11 * time.Minute)

	// Session should still be found before cleanup
	_, ok := m.GetSession("stale-test")
	require.True(t, ok)

	// Trigger cleanup with 10 minute threshold (matches production code)
	m.CleanupStale(10 * time.Minute)

	_, ok = m.GetSession("stale-test")
	assert.False(t, ok, "session idle >10min should be cleaned up")
}

func TestCleanupStale_RecentSessionKept(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	_, err := m.GetOrCreate("u", "recent-test", "/nonexistent/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "", "")
	require.NoError(t, err)

	// Session just created → should survive 10min cleanup
	m.CleanupStale(10 * time.Minute)

	_, ok := m.GetSession("recent-test")
	assert.True(t, ok, "recently accessed session should not be cleaned up")
}

// --- isTranscodeComplete ---

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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stream_1.m3u8"), []byte("#EXTM3U\n"), 0644)) // no ENDLIST

	assert.False(t, apptranscode.IsTranscodeComplete(dir, 2))
}

func TestIsTranscodeComplete_MissingFiles(t *testing.T) {
	dir := t.TempDir()
	// no files at all
	assert.False(t, apptranscode.IsTranscodeComplete(dir, 2))

	// master exists but no stream files
	require.NoError(t, os.WriteFile(filepath.Join(dir, "master.m3u8"), []byte("#EXTM3U\n"), 0644))
	assert.False(t, apptranscode.IsTranscodeComplete(dir, 1))
}

func TestIsTranscodeComplete_SingleStream(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "master.m3u8"), []byte("#EXTM3U\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stream_0.m3u8"), []byte("#EXTM3U\n#EXT-X-ENDLIST\n"), 0644))

	assert.True(t, apptranscode.IsTranscodeComplete(dir, 1))
}

// --- IsSingleStream ---

func TestIsSingleStream(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 10)
	defer m.CleanupAll()

	copy, _ := m.GetOrCreate("u", "ss-copy", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "copy", "transcode")
	assert.True(t, copy.IsSingleStream())

	tc, _ := m.GetOrCreate("u", "ss-tc", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "transcode", "transcode")
	assert.True(t, tc.IsSingleStream())

	multi, _ := m.GetOrCreate("u", "ss-multi", "/f.mkv", 1920, 1080, 0, 0, -1, "", nil, false, false, 0, "", "")
	assert.False(t, multi.IsSingleStream())
}

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

func TestBuildFFmpegArgs_NoBurnIn(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("u", "m", "/f.mkv", 1920, 1080, 0, -1, "", nil, false, false)
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "[0:v]split=")
	assert.NotContains(t, joined, "overlay")
}

func TestBuildFFmpegArgs_BurnIn(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("u", "m2", "/f.mkv", 1920, 1080, 0, 2, "hdmv_pgs_subtitle", nil, false, false)
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
	sess, err := m.GetOrCreate("user1", "test-media", "/nonexistent/file.mkv", 1920, 1080, 0, -1, "", nil, false, false)
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

	_, err := m.GetOrCreate("user1", "media1", "/nonexistent/a.mkv", 1920, 1080, 0, -1, "", nil, false, false)
	require.NoError(t, err)

	_, err = m.GetOrCreate("user1", "media2", "/nonexistent/b.mkv", 1920, 1080, 0, -1, "", nil, false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max concurrent")
}

func TestGetOrCreate_ReturnsSameSession(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	s1, err := m.GetOrCreate("user1", "media1", "/nonexistent/a.mkv", 1920, 1080, 0, -1, "", nil, false, false)
	require.NoError(t, err)

	s2, err := m.GetOrCreate("user1", "media1", "/nonexistent/a.mkv", 1920, 1080, 0, -1, "", nil, false, false)
	require.NoError(t, err)
	assert.Equal(t, s1, s2)
}

func TestSessionOutputDir(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)
	defer m.CleanupAll()

	sess, err := m.GetOrCreate("user1", "my-media-id", "/nonexistent/a.mkv", 1920, 1080, 0, -1, "", nil, false, false)
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

	sess, err := m.GetOrCreate("user1", "media-access", "/nonexistent/a.mkv", 1920, 1080, 0, -1, "", nil, false, false)
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

	sess, err := m.GetOrCreate("user1", "bad-file", "/nonexistent/file.mkv", 1920, 1080, 0, -1, "", nil, false, false)
	require.NoError(t, err)

	// ffmpeg will fail since the file doesn't exist; WaitReady returns error
	err = sess.WaitReady(t.Context())
	assert.Error(t, err)
}

func TestGetOrCreate_MaxUserStreams(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 10)
	defer m.CleanupAll()

	_, err := m.GetOrCreate("user1", "media-u1a", "/nonexistent/a.mkv", 1920, 1080, 1, -1, "", nil, false, false)
	require.NoError(t, err)

	_, err = m.GetOrCreate("user1", "media-u1b", "/nonexistent/b.mkv", 1920, 1080, 1, -1, "", nil, false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max streams per user")

	// different user must succeed
	_, err = m.GetOrCreate("user2", "media-u2a", "/nonexistent/c.mkv", 1920, 1080, 1, -1, "", nil, false, false)
	require.NoError(t, err)
}

func TestCleanupAll_RemovesOutputDirs(t *testing.T) {
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(dir, 5)

	sess, err := m.GetOrCreate("user1", "cleanup-test", "/nonexistent/a.mkv", 1920, 1080, 0, -1, "", nil, false, false)
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
	sess, err := m.GetOrCreate("u", "hdr-media", "/f.mkv", w, h, 0, -1, "", streams, false, false)
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
	sess, err := m.GetOrCreate("u", "hdr-media-hdr-client", "/f.mkv", w, h, 0, -1, "", streams, true, false)
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
	sess, err := m.GetOrCreate("u", "sdr-media", "/f.mkv", 1920, 1080, 0, -1, "", nil, false, false)
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

	sess, err := m.GetOrCreate("u", "audio-norm", "/f.mkv", 1920, 1080, 0, -1, "", nil, false, true)
	require.NoError(t, err)

	args := sess.BuildFFmpegArgs("/f.mkv")
	joined := strings.Join(args, " ")
	assert.Contains(t, joined, "loudnorm=I=-16:TP=-1.5:LRA=11:linear=true")
}

package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/eveyproject/evey/internal/domain/music"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMusicScanner(t *testing.T) {
	s := NewMusicScanner()
	require.NotNil(t, s)
}

func TestMusicScanner_WalkAudioFiles(t *testing.T) {
	dir := t.TempDir()

	audioFiles := map[string]string{
		"song.mp3":   "audio/mpeg",
		"track.flac": "audio/flac",
		"clip.m4a":   "audio/mp4",
		"sound.ogg":  "audio/ogg",
		"voice.opus": "audio/ogg",
		"raw.wav":    "audio/wav",
		"tune.aac":   "audio/aac",
	}
	for name := range audioFiles {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("audio data"), 0644))
	}
	// Non-audio files
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("text"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "video.mkv"), []byte("video"), 0644))

	s := NewMusicScanner()
	var found []music.ScannedAudioFile
	err := s.WalkAudioFiles(context.Background(), dir, func(f music.ScannedAudioFile) error {
		found = append(found, f)
		return nil
	})
	require.NoError(t, err)
	assert.Len(t, found, len(audioFiles))

	mimeMap := map[string]string{}
	for _, f := range found {
		mimeMap[filepath.Base(f.Path)] = f.MimeType
	}
	for name, expectedMime := range audioFiles {
		assert.Equal(t, expectedMime, mimeMap[name], "mime mismatch for %s", name)
	}
}

func TestMusicScanner_WalkAudioFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	s := NewMusicScanner()
	var found []music.ScannedAudioFile
	err := s.WalkAudioFiles(context.Background(), dir, func(f music.ScannedAudioFile) error {
		found = append(found, f)
		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, found)
}

func TestMusicScanner_WalkAudioFiles_ContextCancelled(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "song.mp3"), []byte("data"), 0644))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s := NewMusicScanner()
	err := s.WalkAudioFiles(ctx, dir, func(f music.ScannedAudioFile) error {
		t.Fatal("callback should not be called")
		return nil
	})
	require.Error(t, err)
}

func TestMusicScanner_WalkAudioFiles_Subdirs(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "album")
	require.NoError(t, os.MkdirAll(subDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "track1.flac"), []byte("data"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "track2.mp3"), []byte("data"), 0644))

	s := NewMusicScanner()
	var found []music.ScannedAudioFile
	err := s.WalkAudioFiles(context.Background(), dir, func(f music.ScannedAudioFile) error {
		found = append(found, f)
		return nil
	})
	require.NoError(t, err)
	assert.Len(t, found, 2)
}

func TestMusicScanner_OpenFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.mp3")
	require.NoError(t, os.WriteFile(path, []byte("content"), 0644))

	s := NewMusicScanner()
	rc, err := s.OpenFile(path)
	require.NoError(t, err)
	defer rc.Close()
}

func TestMusicScanner_OpenFile_NotFound(t *testing.T) {
	s := NewMusicScanner()
	_, err := s.OpenFile("/tmp/nonexistent-music-file-xyz.mp3")
	require.Error(t, err)
}

// Prod 2026-08-24: /media/musique vanished from the kernel (dead mount) and
// the walk logged one WARN, returned nil, and the scan reported "completed,
// nothing new" - a missing library ROOT looked exactly like an empty library.
// The root failing is not a per-file hiccup to skip: it means the scan saw
// NOTHING, and the caller must know.
func TestWalkAudioFiles_MissingRootIsAnError(t *testing.T) {
	s := NewMusicScanner()
	err := s.WalkAudioFiles(context.Background(), "/nonexistent-evey-test-root", func(music.ScannedAudioFile) error { return nil })
	if err == nil {
		t.Fatal("a missing library root must fail the walk, not read as an empty library")
	}
}

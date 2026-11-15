package mkvgo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
	extwriter "github.com/gravity-zero/mkvgo/mkv/writer"
	"github.com/stretchr/testify/require"
)

// A dubbed film laid out the way real ones are: the dub first, its subtitles,
// the picture, then the original audio. The second audio track is track 5,
// not 2 - adding one to an audio index lands on a subtitle.
func TestAudioTrackNumbers_FollowTheFileNotTheAudioRank(t *testing.T) {
	p := filepath.Join(t.TempDir(), "dub.mkv")
	f, err := os.Create(p)
	require.NoError(t, err)
	mw := extwriter.NewMKVWriter(f)
	require.NoError(t, mw.WriteStart())
	tracks := []extmkv.Track{
		{ID: 1, Type: extmkv.AudioTrack, Codec: "ac3", Language: "fre", IsDefault: true},
		{ID: 2, Type: extmkv.SubtitleTrack, Codec: "srt", Language: "fre"},
		{ID: 3, Type: extmkv.SubtitleTrack, Codec: "srt", Language: "fre"},
		{ID: 4, Type: extmkv.VideoTrack, Codec: "h264", Language: "und", IsDefault: true},
		{ID: 5, Type: extmkv.AudioTrack, Codec: "ac3", Language: "eng"},
	}
	c := &extmkv.Container{Info: extmkv.SegmentInfo{TimecodeScale: 1_000_000}}
	require.NoError(t, mw.WriteMetadata(c, tracks, 1000))
	payload := make([]byte, 256)
	require.NoError(t, mw.WriteClusterWithCues(0, 1_000_000, []extmkv.Block{
		{TrackNumber: 4, Timecode: 0, Keyframe: true, Data: payload},
		{TrackNumber: 1, Timecode: 0, Keyframe: true, Data: payload},
		{TrackNumber: 5, Timecode: 0, Keyframe: true, Data: payload},
	}))
	require.NoError(t, mw.Finalize())
	require.NoError(t, f.Close())

	got, err := NewTrackRetimer().AudioTrackNumbers(context.Background(), p)

	require.NoError(t, err)
	require.Equal(t, []uint64{1, 5}, got)
}

func TestAudioTrackNumbers_RefusesANonMatroskaFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.mp4")
	require.NoError(t, os.WriteFile(p, []byte("not ebml"), 0o644))
	_, err := NewTrackRetimer().AudioTrackNumbers(context.Background(), p)
	require.Error(t, err)
}

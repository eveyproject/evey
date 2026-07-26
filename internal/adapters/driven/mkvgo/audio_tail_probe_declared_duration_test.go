package mkvgo

// The audio tail probe on real bytes, against the failure mode that made its
// prod numbers unbelievable: a container that DECLARES more than it contains.
// No ffmpeg needed - the writer produces the shapes directly.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
	extwriter "github.com/gravity-zero/mkvgo/mkv/writer"
	"github.com/stretchr/testify/require"
)

// genTailFixture writes an MKV declaring declaredMs, whose video blocks run to
// videoEndMs and audio blocks to audioEndMs (one cued cluster per second).
func genTailFixture(t *testing.T, name string, declaredMs, videoEndMs, audioEndMs int64) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	f, err := os.Create(p)
	require.NoError(t, err)
	defer f.Close()

	mw := extwriter.NewMKVWriter(f)
	require.NoError(t, mw.WriteStart())
	c := &extmkv.Container{Info: extmkv.SegmentInfo{TimecodeScale: 1_000_000}}
	tracks := []extmkv.Track{
		{ID: 1, UID: 11, Type: extmkv.VideoTrack, Codec: "h264", Language: "eng", IsDefault: true},
		{ID: 2, UID: 12, Type: extmkv.AudioTrack, Codec: "aac", Language: "eng", IsDefault: true},
	}
	require.NoError(t, mw.WriteMetadata(c, tracks, declaredMs))
	for ts := int64(0); ts <= max(videoEndMs, audioEndMs); ts += 1000 {
		var blocks []extmkv.Block
		if ts <= videoEndMs {
			blocks = append(blocks, extmkv.Block{TrackNumber: 1, Timecode: ts, Keyframe: true, Data: []byte("v")})
		}
		if ts <= audioEndMs {
			blocks = append(blocks, extmkv.Block{TrackNumber: 2, Timecode: ts, Keyframe: true, Data: []byte("a")})
		}
		require.NoError(t, mw.WriteClusterWithCues(ts, 1_000_000, blocks))
	}
	require.NoError(t, mw.Finalize())
	return p
}

// A container that over-declares its duration by 60 s while BOTH tracks stop
// together is not an audio defect - it is a wrong declaration. Measuring the
// audio against that declaration is what produced badges like "Audio -154s" on
// prod files whose picture and sound were perfectly aligned (2026-07-26).
func TestProbeAudioTailGapSec_OverDeclaredDuration_NotBlamedOnAudio(t *testing.T) {
	p := genTailFixture(t, "over_declared.mkv", 120_000, 60_000, 60_000)

	gap, err := NewAudioTailProber().ProbeAudioTailGapSec(context.Background(), p)
	require.NoError(t, err)
	require.Zero(t, gap,
		"video and audio end together: the container's declared duration is the liar, and the audio must not be accused for it")
}

// The control, so the fix cannot have simply blinded the probe: same declared
// duration, audio genuinely stopping 15 s before the video.
func TestProbeAudioTailGapSec_AudioShorterThanVideo_StillMeasured(t *testing.T) {
	p := genTailFixture(t, "short_audio.mkv", 120_000, 60_000, 45_000)

	gap, err := NewAudioTailProber().ProbeAudioTailGapSec(context.Background(), p)
	require.NoError(t, err)
	require.InDelta(t, 15.0, gap, 1.0, "the audio really does die 15 s before the picture")
}

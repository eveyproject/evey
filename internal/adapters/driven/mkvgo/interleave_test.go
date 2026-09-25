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

// genInterleavedMKV writes clusters at 1 s steps; chunked=true writes every
// video cluster first, then every audio cluster (the ffmpeg two-input shape),
// so cluster timestamps fall back to 0 mid-file.
func genInterleavedMKV(t *testing.T, chunked bool) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "interleave.mkv")
	f, err := os.Create(p)
	require.NoError(t, err)
	defer f.Close()

	mw := extwriter.NewMKVWriter(f)
	require.NoError(t, mw.WriteStart())
	c := &extmkv.Container{Info: extmkv.SegmentInfo{TimecodeScale: 1_000_000}}
	tracks := []extmkv.Track{
		{ID: 1, Type: extmkv.VideoTrack, Codec: "h264", Language: "eng", IsDefault: true},
		{ID: 2, Type: extmkv.AudioTrack, Codec: "aac", Language: "eng", IsDefault: true},
	}
	const seconds = int64(600)
	require.NoError(t, mw.WriteMetadata(c, tracks, seconds*1000))
	payload := make([]byte, 4096)
	video := func(ts int64) extmkv.Block {
		return extmkv.Block{TrackNumber: 1, Timecode: ts, Keyframe: true, Data: payload}
	}
	audio := func(ts int64) extmkv.Block {
		return extmkv.Block{TrackNumber: 2, Timecode: ts, Keyframe: true, Data: payload[:512]}
	}
	if chunked {
		for s := range seconds {
			require.NoError(t, mw.WriteClusterWithCues(s*1000, 1_000_000, []extmkv.Block{video(s * 1000)}))
		}
		for s := range seconds {
			require.NoError(t, mw.WriteClusterWithCues(s*1000, 1_000_000, []extmkv.Block{audio(s * 1000)}))
		}
	} else {
		for s := range seconds {
			require.NoError(t, mw.WriteClusterWithCues(s*1000, 1_000_000, []extmkv.Block{video(s * 1000), audio(s * 1000)}))
		}
	}
	require.NoError(t, mw.Finalize())
	return p
}

func TestProbeInterleaveBackstep_HealthyFileReadsZero(t *testing.T) {
	back, err := New().ProbeInterleaveBackstepSec(context.Background(), genInterleavedMKV(t, false))
	require.NoError(t, err)
	require.Zero(t, back)
}

func TestProbeInterleaveBackstep_ChunkedFileReadsBackMinutes(t *testing.T) {
	back, err := New().ProbeInterleaveBackstepSec(context.Background(), genInterleavedMKV(t, true))
	require.NoError(t, err)
	// Video clusters cover 0..600 s, then audio restarts at 0: the step back
	// at the boundary is hundreds of seconds, never a few.
	require.Greater(t, back, 100.0)
}

func TestProbeInterleaveBackstep_NonEBMLIsNoFinding(t *testing.T) {
	back, err := New().ProbeInterleaveBackstepSec(context.Background(), filepath.Join(t.TempDir(), "x.mp4"))
	require.NoError(t, err)
	require.Zero(t, back)
}

func TestMaxBackstepMs(t *testing.T) {
	require.Zero(t, maxBackstepMs(nil))
	require.Zero(t, maxBackstepMs([]int64{0, 5, 5, 9}))
	require.EqualValues(t, 400, maxBackstepMs([]int64{0, 500, 100, 900, 700}))
}

// genThinBlocksMKV writes blocks of blockSec seconds: the video clusters
// (4 KiB a second), then the SAME seconds of a thin audio track (64 B a
// second) - a stereo track's block is a sliver of the file, which the
// sampled probe steps over.
func genThinBlocksMKV(t *testing.T, seconds, blockSec int64) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "thin.mkv")
	f, err := os.Create(p)
	require.NoError(t, err)
	defer f.Close()
	mw := extwriter.NewMKVWriter(f)
	require.NoError(t, mw.WriteStart())
	c := &extmkv.Container{Info: extmkv.SegmentInfo{TimecodeScale: 1_000_000}}
	tracks := []extmkv.Track{
		{ID: 1, Type: extmkv.VideoTrack, Codec: "h264", Language: "eng", IsDefault: true},
		{ID: 2, Type: extmkv.AudioTrack, Codec: "aac", Language: "eng", IsDefault: true},
	}
	require.NoError(t, mw.WriteMetadata(c, tracks, seconds*1000))
	payload := make([]byte, 4096)
	for bs := int64(0); bs < seconds; bs += blockSec {
		for s := bs; s < bs+blockSec; s++ {
			require.NoError(t, mw.WriteClusterWithCues(s*1000, 1_000_000, []extmkv.Block{{TrackNumber: 1, Timecode: s * 1000, Keyframe: true, Data: payload}}))
		}
		for s := bs; s < bs+blockSec; s++ {
			require.NoError(t, mw.WriteClusterWithCues(s*1000, 1_000_000, []extmkv.Block{{TrackNumber: 2, Timecode: s * 1000, Keyframe: true, Data: payload[:64]}}))
		}
	}
	require.NoError(t, mw.Finalize())
	return p
}

// The health scan's probe walks every cluster: a thin block the sampling
// steps over (0 here) reads its full length.
func TestExactInterleaveProber_SeesThinBlocksTheSamplingMisses(t *testing.T) {
	path := genThinBlocksMKV(t, 720, 180)
	sampled, err := New().ProbeInterleaveBackstepSec(context.Background(), path)
	require.NoError(t, err)
	require.Less(t, sampled, 10.0, "the sampling misses the thin blocks (the reason for the exact probe)")

	exact, err := ExactInterleaveProber{New()}.ProbeInterleaveBackstepSec(context.Background(), path)
	require.NoError(t, err)
	require.InDelta(t, 179.0, exact, 1.0, "the last audio cluster's jump back over its block")

	healthy, err := ExactInterleaveProber{New()}.ProbeInterleaveBackstepSec(context.Background(), genInterleavedMKV(t, false))
	require.NoError(t, err)
	require.Zero(t, healthy)
	nonEBML, err := ExactInterleaveProber{New()}.ProbeInterleaveBackstepSec(context.Background(), filepath.Join(t.TempDir(), "x.mp4"))
	require.NoError(t, err)
	require.Zero(t, nonEBML)
}

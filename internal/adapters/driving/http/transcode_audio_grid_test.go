package httpapi

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// recordingAudioEnsurer keeps the fingerprint every segment is asked under:
// its grid is what the encoder cuts the rendition on (-hls_time, -ss).
type recordingAudioEnsurer struct {
	fps []apptranscoding.AudioFingerprint
}

func (r *recordingAudioEnsurer) EnsureAudioSegment(_ context.Context, fp apptranscoding.AudioFingerprint, _ int) error {
	r.fps = append(r.fps, fp)
	return errTest
}

func (r *recordingAudioEnsurer) EnsureAudioInit(_ context.Context, fp apptranscoding.AudioFingerprint) error {
	r.fps = append(r.fps, fp)
	return errTest
}

// playlistSegmentStarts reads where each segment of a media playlist begins,
// as a player that trusts the playlist places it: the running sum of EXTINF.
func playlistSegmentStarts(t *testing.T, body string) (starts []float64, total float64) {
	t.Helper()
	for line := range strings.SplitSeq(body, "\n") {
		v, ok := strings.CutPrefix(line, "#EXTINF:")
		if !ok {
			continue
		}
		d, err := strconv.ParseFloat(strings.TrimSuffix(v, ","), 64)
		require.NoError(t, err, line)
		starts = append(starts, total)
		total += d
	}
	return starts, total
}

// The audio rendition playlist must declare the grid its segments are cut on.
// A player in sequence mode (Shaka, the cast receiver's engine) places segment
// N at the sum of the EXTINF before it, not at its tfdt: a playlist declaring
// 3.000 over segments cut every 3.003 s put the audio 3 ms earlier per
// segment, 1.3 s early twenty-two minutes in, while the video playlist was
// right. Checked for every grid a source can get, frame-exact or legacy.
func TestAudioRenditionPlaylist_DeclaresTheGridItsSegmentsAreCutOn(t *testing.T) {
	fps := func(v float64) *float64 { return &v }
	cases := []struct {
		name string
		rate *float64
		mode string
	}{
		{"23.976", fps(24000.0 / 1001), ""},
		{"29.97", fps(30000.0 / 1001), ""},
		{"59.94", fps(60000.0 / 1001), ""},
		{"24", fps(24), ""},
		{"25", fps(25), ""},
		{"50", fps(50), ""},
		{"vfr", fps(24000.0 / 1001), "vfr"},
		{"unknown rate", nil, ""},
	}
	// Long enough for the drift to show, with a leftover last segment.
	const durMs = 3_514_428
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dur, _ := shared.NewDurationMs(durMs)
			item := &domainlibrary.MediaItem{
				ID:        "m1",
				LibraryID: "lib1",
				File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur},
			}
			streams := []*domainlibrary.MediaStream{
				{StreamType: "video", Codec: "h264", Framerate: tc.rate, FrameRateMode: tc.mode},
				{StreamType: "audio", Codec: "eac3", Language: "en"},
				{StreamType: "audio", Codec: "eac3", Language: "fr"},
			}
			ensurer := &recordingAudioEnsurer{}
			h := buildVODHandlerWithAudio(t, &testFakeStreamsGetter{item: item, streams: streams},
				allowAll(), testFakeParentalChecker{}, ensurer)

			w := doAudioDispatchRequest(h, "m1", "audio_1.m3u8")
			require.Equal(t, http.StatusOK, w.Code)
			starts, total := playlistSegmentStarts(t, w.Body.String())

			doAudioDispatchRequest(h, "m1", "audio_1_000.m4s")
			require.NotEmpty(t, ensurer.fps, "the segment must reach the orchestrator")
			grid := ensurer.fps[len(ensurer.fps)-1].Grid

			require.Equal(t, grid.SegmentCount(durMs), len(starts),
				"the playlist must list as many segments as the encoder cuts")
			for n, s := range starts {
				if math.Abs(s-grid.SegmentStartSeconds(n)) > 0.0005 {
					t.Fatalf("segment %d declared at %.3f s, cut at %.3f s (grid %s)",
						n, s, grid.SegmentStartSeconds(n), grid.HLSTime())
				}
			}
			// Every segment the playlist lists is one the encoder produces:
			// the declared total stops at the media's end, within a frame.
			assert.InDelta(t, float64(durMs)/1000, total, 0.05)
		})
	}
}

package transcoding

import (
	"fmt"
	"math"
	"net/url"
	"strings"

	"github.com/evey-project/evey/internal/domain/transcoding"
)

// ManifestSegmentSeconds is the target HLS segment length. MUST stay in sync with
// the ffmpeg `-force_key_frames expr:gte(t,n_forced*N)` cadence (hlsForceKeyframesArgs
// in adapters/driven/ffmpeg/hls_transcoder.go) so each segment lands on a keyframe.
const ManifestSegmentSeconds = 3.0

// ManifestInput is the minimal info needed to render an HLS master + variant playlists
// for a single playback decision. The same input must always produce the same manifest —
// callers therefore derive `MediaID + Profiles + StreamToken` from the playback decision
// up front and reuse it for the lifetime of the session.
type ManifestInput struct {
	// MediaID is the media item UUID. Used in segment URLs.
	MediaID string

	// DurationSec is the total media duration. The manifest is sliced into
	// ceil(Duration / ManifestSegmentSeconds) segments. Must be > 0 — callers
	// fall back on a "growing event" manifest path when duration is unknown.
	DurationSec float64

	// Profiles is the ordered list of ABR variants. profileIdx 0 ↔ Profiles[0],
	// and so on. Each gets its own sub-playlist URL (stream_{idx}.m3u8).
	Profiles []transcoding.HLSProfile

	// StreamToken is the per-session, short-lived (5-min sliding TTL) token that
	// the playback path stamps on every segment URL. Baked into the manifest so
	// hls.js can stream without negotiating per-segment auth.
	StreamToken string

	// AudioTrack picks the audio stream (0-indexed) from the source. Default 0.
	// Threaded into the segment URL so the orchestrator hashes it into the
	// decisionHash (different audio track ⇒ separate segment cache).
	AudioTrack int

	// SubtitleBurnInTrack is the source subtitle stream burnt onto the video, or
	// -1 to disable. Threaded into segment URLs (different sub ⇒ separate cache).
	SubtitleBurnInTrack int

	// VideoCodecs / AudioCodecs / Containers are the client capabilities, forwarded
	// to the ffmpeg adapter so it picks a codec the client can decode. Threaded
	// into segment URLs as query parameters.
	VideoCodecs []string
	AudioCodecs []string
	Containers  []string

	// UseFMP4 picks the segment container: fMP4 (`.m4s` + `stream_N_init.mp4`)
	// when true, MPEG-TS (`.ts`) when false. fMP4 is default in production —
	// segments are ~30% smaller and CMAF-style aligns with hls.js modern path.
	UseFMP4 bool
}

// Validate returns an error if the input can't produce a valid manifest.
// Empty MediaID, zero duration, or no profiles all fail loudly: a VOD manifest
// MUST have a known end (otherwise hls.js will treat it as live and re-introduce
// the drift-snap we just got rid of).
func (in *ManifestInput) Validate() error {
	if in.MediaID == "" {
		return fmt.Errorf("ManifestInput: MediaID required")
	}
	if in.DurationSec <= 0 {
		return fmt.Errorf("ManifestInput: DurationSec must be > 0 (got %v)", in.DurationSec)
	}
	if len(in.Profiles) == 0 {
		return fmt.Errorf("ManifestInput: at least one profile required")
	}
	return nil
}

// SegmentCount is the number of segments the manifest will list, given the
// duration and the fixed segment length. The last segment is shorter than
// ManifestSegmentSeconds whenever the duration isn't an exact multiple.
func (in *ManifestInput) SegmentCount() int {
	if in.DurationSec <= 0 {
		return 0
	}
	return int(math.Ceil(in.DurationSec / ManifestSegmentSeconds))
}

// ManifestGenerator renders HLS master + variant playlists from a decision
// upfront — no ffmpeg involved. The playlists are immutable for the lifetime
// of a (mediaID, decision) pair: hls.js fetches them once and trusts them.
// Segments are encoded on demand by the segment orchestrator when the player
// fetches them; the manifest's job is just to publish the contract.
type ManifestGenerator struct{}

func NewManifestGenerator() *ManifestGenerator {
	return &ManifestGenerator{}
}

// Master renders the master playlist (multi-variant). Format:
//
//	#EXTM3U
//	#EXT-X-VERSION:6
//	#EXT-X-STREAM-INF:BANDWIDTH=<bps>,RESOLUTION=<wxh>,CODECS="<codecs>"
//	stream_0.m3u8?token=…
//	…
//
// `#EXT-X-VERSION:6` is required for fMP4 (which needs `#EXT-X-MAP`); even when
// emitting TS we keep v6 so we don't have to fork the version per container.
func (g *ManifestGenerator) Master(in ManifestInput) (string, error) {
	if err := in.Validate(); err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	b.WriteString("#EXT-X-VERSION:6\n")

	for i, p := range in.Profiles {
		// BANDWIDTH is the playlist's peak bitrate in bps. We use video + audio
		// (both in kbps from the profile, so multiply by 1000). hls.js uses it
		// to drive ABR switches — if it's wrong the wrong rendition is picked.
		bps := (p.VideoBitrate + p.AudioBitrate) * 1000
		res := fmt.Sprintf("%dx%d", p.Width, p.Height)
		codecs := masterCodecsAttr()

		fmt.Fprintf(&b, "#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%s,CODECS=%q\n", bps, res, codecs)
		fmt.Fprintf(&b, "stream_%d.m3u8%s\n", i, querySuffix(in))
	}

	return b.String(), nil
}

// Variant renders one sub-playlist (one ABR rendition). profileIdx must be a
// valid index into in.Profiles. The variant is VOD (PLAYLIST-TYPE:VOD +
// ENDLIST), so hls.js treats it as a complete on-demand stream — no live-edge
// snapping, no re-fetch of the playlist while playing.
//
// Each segment is listed with #EXTINF duration. All segments are
// ManifestSegmentSeconds except the last, which is the leftover. When
// UseFMP4 is true, #EXT-X-MAP points at the init segment that carries the
// codec config (moov box) shared across .m4s segments.
func (g *ManifestGenerator) Variant(in ManifestInput, profileIdx int) (string, error) {
	if err := in.Validate(); err != nil {
		return "", err
	}
	if profileIdx < 0 || profileIdx >= len(in.Profiles) {
		return "", fmt.Errorf("Variant: profileIdx %d out of range [0,%d)", profileIdx, len(in.Profiles))
	}

	segCount := in.SegmentCount()
	if segCount == 0 {
		return "", fmt.Errorf("Variant: empty segment list (duration=%v)", in.DurationSec)
	}

	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	b.WriteString("#EXT-X-VERSION:6\n")
	b.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n")
	// TARGETDURATION is the largest #EXTINF rounded up. Since every segment is
	// at most ManifestSegmentSeconds, ceiling that is the safe upper bound and
	// matches what ffmpeg would emit naturally.
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n", int(math.Ceil(ManifestSegmentSeconds)))
	b.WriteString("#EXT-X-MEDIA-SEQUENCE:0\n")

	suffix := querySuffix(in)

	if in.UseFMP4 {
		fmt.Fprintf(&b, "#EXT-X-MAP:URI=\"stream_%d_init.mp4%s\"\n", profileIdx, suffix)
	}

	ext := "ts"
	if in.UseFMP4 {
		ext = "m4s"
	}

	for i := 0; i < segCount; i++ {
		segDur := ManifestSegmentSeconds
		if i == segCount-1 {
			// Last segment runs from i*segDur to end-of-media. Floats: clamp
			// nonnegative and round to 3 decimals to match ffmpeg's #EXTINF
			// formatting (avoids "3.000000" vs "3.0" diff churn in tests).
			leftover := in.DurationSec - float64(i)*ManifestSegmentSeconds
			if leftover > 0 && leftover < ManifestSegmentSeconds {
				segDur = math.Round(leftover*1000) / 1000
			}
		}
		fmt.Fprintf(&b, "#EXTINF:%.3f,\n", segDur)
		fmt.Fprintf(&b, "stream_%d_%03d.%s%s\n", profileIdx, i, ext, suffix)
	}

	b.WriteString("#EXT-X-ENDLIST\n")
	return b.String(), nil
}

// masterCodecsAttr returns a generic codecs string suitable for hls.js capability
// checks. The exact codec depends on the ffmpeg decision; we expose a permissive
// match (avc1.640028 = H.264 high@L4.0, mp4a.40.2 = AAC-LC) so hls.js never
// rejects the rendition based on the master playlist. The real codec is what
// the segment actually carries.
func masterCodecsAttr() string {
	return "avc1.640028,mp4a.40.2"
}

// querySuffix builds the `?…` to append to every URL in the playlists. The
// stream token + decision params are baked into each entry so hls.js doesn't
// need to negotiate auth per request.
func querySuffix(in ManifestInput) string {
	q := url.Values{}
	if in.StreamToken != "" {
		q.Set("token", in.StreamToken)
	}
	q.Set("audioTrack", fmt.Sprintf("%d", in.AudioTrack))
	if in.SubtitleBurnInTrack >= 0 {
		q.Set("subtitleTrack", fmt.Sprintf("%d", in.SubtitleBurnInTrack))
	}
	if len(in.VideoCodecs) > 0 {
		q.Set("videoCodecs", strings.Join(in.VideoCodecs, ","))
	}
	if len(in.AudioCodecs) > 0 {
		q.Set("audioCodecs", strings.Join(in.AudioCodecs, ","))
	}
	if len(in.Containers) > 0 {
		q.Set("containers", strings.Join(in.Containers, ","))
	}
	if encoded := q.Encode(); encoded != "" {
		return "?" + encoded
	}
	return ""
}

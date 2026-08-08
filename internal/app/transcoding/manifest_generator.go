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
// in adapters/driven/ffmpeg/hls_transcoder.go) so each segment lands on a keyframe,
// and with ffmpeg's hlsSegmentSeconds (-hls_time) so the advertised segment count
// matches what the encoder cuts. TestSegmentGridConstantsInSync enforces the latter.
const ManifestSegmentSeconds = 3.0

// ManifestInput is the minimal info needed to render an HLS master + variant playlists
// for a single playback decision. The same input must always produce the same manifest -
// callers therefore derive `MediaID + Profiles + StreamToken` from the playback decision
// up front and reuse it for the lifetime of the session.
type ManifestInput struct {
	// MediaID is the media item UUID. Used in segment URLs.
	MediaID string

	// DurationSec is the total media duration. The manifest is sliced into
	// ceil(Duration / ManifestSegmentSeconds) segments. Must be > 0 - callers
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
	// when true, MPEG-TS (`.ts`) when false. fMP4 is default in production -
	// segments are ~30% smaller and CMAF-style aligns with hls.js modern path.
	UseFMP4 bool

	// CopyVideoCodec is the SOURCE RFC-6381 codec string (avc1.PPCCLL /
	// hvc1.P.C.Lxxx.B0) advertised for the video-COPY rung (the rung whose
	// HLSProfile.Copy is true), built from the source profile + level via
	// CopyVideoCodecString. Empty when there is no copy rung; the master then
	// falls back to the codec-family default for every rung.
	CopyVideoCodec string

	// CopyVideoRange is the HLS VIDEO-RANGE attribute value ("PQ" or "HLG")
	// for the copy rung when the SOURCE is HDR and served raw, "" otherwise.
	// Only the copy rung carries it: transcoded rungs are tonemapped to SDR.
	// Apple's HLS authoring spec requires VIDEO-RANGE on HDR variants -
	// without it some Safari/tvOS versions render PQ as washed-out SDR or
	// refuse the rendition; hls.js ignores unknown attributes, so emitting it
	// is free for web.
	CopyVideoRange string

	// ChosenVideoCodec / ChosenAudioCodec are the codecs the ffmpeg decision
	// picked for this session (e.g. "h264", "hevc", "av1" / "aac", "ac3",
	// "eac3"). Used to emit a correct `CODECS=...` attribute on each
	// `#EXT-X-STREAM-INF` so Safari + AirPlay don't reject HEVC/AV1
	// renditions out of hand (they validate against the master before
	// pulling the segment). Empty falls back to h264+aac, which preserves
	// the pre-refactor behaviour.
	ChosenVideoCodec string
	ChosenAudioCodec string

	// ResumeOffsetSec, when > 0, emits `#EXT-X-START:TIME-OFFSET=<n>` in
	// each variant so Safari natif / AirPlay / tvOS resume at the right
	// position without a JS seek. NOT hashed into the decisionHash -
	// per-user resume must NOT shard the segment cache. Threaded via a
	// query param outside the cache key.
	ResumeOffsetSec float64

	// AudioRenditions, when non-empty, switches the master from muxed
	// audio+video variants to HLS-native audio renditions: video-only
	// variants link to a shared `#EXT-X-MEDIA TYPE=AUDIO,GROUP-ID="aud"`
	// group with one entry per source audio track. The player can then
	// flip audio language without re-fetching the video stream. The
	// first entry is marked DEFAULT=YES.
	//
	// Empty (the default) preserves the pre-feature muxed-pipeline
	// behaviour bit-for-bit - every existing client and test keeps
	// working without change.
	AudioRenditions []AudioRendition

	// ExternalAudio switches the master to VIDEO-ONLY: video variants with no
	// muxed audio, no `#EXT-X-MEDIA:TYPE=AUDIO` group, and a CODECS attribute
	// that lists only the video codec. Used by the browser wasm-audio path,
	// where the AC-3/E-AC-3 audio is decoded client-side (ac3go wasm) from a
	// rendition the player fetches OUT OF BAND (mkvgo's od_audio{k}.m3u8) - so
	// the audio must NOT ride the master at all, or MSE would reject the
	// undecodable ec-3 group. Mutually exclusive with AudioRenditions (that
	// path keeps the in-master AAC group for native players); when both are set
	// ExternalAudio wins and no group is emitted. Empty (false) is the
	// pre-feature behaviour, bit-for-bit.
	ExternalAudio bool

	// ExternalAudioURI, when ExternalAudio is set, is emitted in the master as a
	// custom `#EVEY-AUDIO-URI:"<uri>"` tag. It names the AC-3/E-AC-3 audio
	// rendition the wasm client fetches OUT OF BAND - either mkvgo's own
	// (od_audio{k}.m3u8) or the ffmpeg copy rendition (audio_{k}.m3u8?wasmAudio=1),
	// whichever the server chose. hls.js ignores the unknown tag (video-only
	// playback); the wasm engine reads it, staying agnostic of which backend
	// packaged the audio. Empty emits no tag.
	ExternalAudioURI string

	// ExternalAudioAACURI, when ExternalAudio is set, is emitted as a
	// `#EVEY-AUDIO-AAC-URI:"<uri>"` tag naming the AAC transcode rendition of
	// the SAME track (audio_{k}.m3u8, no wasmAudio param - with it the handler
	// serves the AC-3 copy). The bi-source relay client decodes it NATIVELY for
	// instant sound while the wasm decoder boots, then hands over to the AC-3
	// rendition at a segment boundary. Empty emits no tag (no relay offered).
	ExternalAudioAACURI string

	// WasmAudio bakes `wasmAudio=1` into every URI this manifest emits. It is a
	// decision input on the AUDIO rendition path (like the codec lists on the
	// video path): it flips buildAudioFingerprint from the AAC re-encode to an
	// AC-3/E-AC-3 stream COPY, and forks the segment cache. The audio playlist
	// MUST carry it so its init/segment requests resolve to the same passthrough
	// bytes the playlist was built for - dropping it would serve AAC segments
	// under an AC-3 init. Empty = the AAC rendition path, unchanged.
	WasmAudio bool

	// HDRCapable mirrors the request's `hdr=1` client capability into every
	// baked URL. The master decision (copy rung vs tonemap) reads this flag;
	// without re-baking it, the variant/segment requests re-derive the decision
	// WITHOUT it and produce tonemapped SDR h264 segments inside a variant the
	// master advertised as hvc1/PQ copy - codec mismatch, broken playback.
	HDRCapable bool

	// MaxAudioChannels is the client's declared channel CEILING (?maxAudioChannels).
	// A decision input on the audio-rendition path exactly like the codec lists,
	// and it must ride every baked URL: the rendition's init/segments derive
	// their channel count from the request, so a master negotiated at 2 whose
	// audio URIs carry nothing re-derives from the SOURCE and serves 5.1 anyway.
	// That is what made the cast receiver's stereo fallback a placebo - it
	// renegotiated the master, got the same 5.1 rendition back, and failed
	// identically (2026-08-08, Chromecast HD). 0 = no ceiling declared.
	MaxAudioChannels int

	// AudioOffsetMs is the user's audio resync (positive = audio later). A
	// decision input like the codec lists: it forces the audio decision to
	// transcode and forks the segment cache, so it must ride every baked URL.
	AudioOffsetMs int

	// PinnedVideoDecision ("copy"|"transcode") stamps the video decision the
	// FIRST playlist of this playback computed into every baked URL (vd=...),
	// so background enrichment (keyframe backfill flipping copy-eligibility)
	// cannot change what a LIVE session's URLs serve between two polls of the
	// EVENT playlist. Empty = legacy request, nothing stamped.
	PinnedVideoDecision string

	// PinnedAudioDecision ("copy"|"transcode") - same contract as
	// PinnedVideoDecision, for the AUDIO side: an admin toggle (loudnorm) or
	// background enrichment can flip audio copy-eligibility between two polls
	// of a live playlist, switching the served audio codec under the player.
	PinnedAudioDecision string

	// VideoPlaylistURI, when set, overrides the video variant's playlist URI
	// (the default is stream_<idx>.m3u8). It is honoured ONLY for a
	// single-variant input - the on-demand copy rung uses it to point the one
	// video variant at the mkvgo packager's playlist (od_playlist.m3u8) while
	// the audio renditions stay Evey's ffmpeg pipeline (the hybrid master:
	// stateless video from PlanHLS + transcoded AAC audio). Empty preserves the
	// stream_N.m3u8 default bit-for-bit.
	VideoPlaylistURI string

	// FingerprintHash seals the decision hash of the moment this manifest was
	// built into every URI it emits (fp=…). Variant and segment URLs name a
	// LADDER INDEX (stream_2), and the server re-derives the ladder on every
	// request: if it reshapes mid-play (a capability flap, an admission pin
	// gone cold), the same URL silently maps to a DIFFERENT rendition and the
	// player concatenates a stranger's bytes onto its init segment - the
	// 2026-07-15 rainbow screen. With the hash sealed in, a request whose fp no
	// longer matches is refused (410) and the player refetches a coherent
	// master instead. Empty emits no fp param (no check, pre-feature shape).
	FingerprintHash string

	// FallbackVariants are extra, self-contained variants appended AFTER the
	// main Profiles. The on-demand copy master uses one to advertise a low
	// transcoded rung a starving player can drop to: the copy path is otherwise
	// single-variant, so a throughput collapse has no ABR escape below the
	// source bitrate and the picture freezes instead of degrading. Each entry
	// belongs to ANOTHER plan (its own fingerprint, its own decision pins), so
	// its URI is emitted VERBATIM - the master's own querySuffix must not touch
	// it. Empty (the default) preserves the master bit-for-bit.
	FallbackVariants []FallbackVariant
}

// FallbackVariant is one FallbackVariants entry: the rung's advertised shape
// and its complete, already-suffixed playlist URI. VideoCodec/AudioCodec are
// the codecs that variant's segments actually carry (its plan's effective
// codecs, not this master's) - a transcoded rung is muxed audio+video, so both
// ride its CODECS attribute and no rendition group is referenced.
type FallbackVariant struct {
	Profile    transcoding.HLSProfile
	URI        string
	VideoCodec string
	AudioCodec string
}

// AudioRendition describes a single source audio track exposed as an
// HLS-native rendition. SourceTrackIdx is the 0-indexed audio stream
// in the source media (used to route segment URLs back to the right
// ffmpeg -map). Name is the user-facing label hls.js / Safari shows in
// the picker (e.g. "Français", "English Director's Commentary").
// LanguageCode is the RFC 5646 tag (e.g. "fr", "en-US"); empty when
// the source doesn't declare one.
type AudioRendition struct {
	SourceTrackIdx int
	Name           string
	LanguageCode   string
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
// upfront - no ffmpeg involved. The playlists are immutable for the lifetime
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
	// Declares to hls.js (and well-behaved players) that every segment starts
	// at a keyframe - true here because the ffmpeg adapter forces keyframes
	// at the segment boundary cadence. Lets the player switch renditions
	// (ABR) and back-seek within the buffer without re-pulling adjacent
	// segments to find an IDR. Without this tag the player has to assume
	// the worst and re-fetch, costing one whole segment of latency on every
	// switch.
	b.WriteString("#EXT-X-INDEPENDENT-SEGMENTS\n")

	// wasm-audio path: name the out-of-band AC-3/E-AC-3 rendition the client
	// decodes itself. A custom tag hls.js ignores (so it plays the master as
	// video-only) while the wasm engine reads it - keeping the client agnostic of
	// whether mkvgo or ffmpeg packaged the audio (see field doc).
	if in.ExternalAudio && in.ExternalAudioURI != "" {
		fmt.Fprintf(&b, "#EVEY-AUDIO-URI:%q\n", in.ExternalAudioURI)
	}
	if in.ExternalAudio && in.ExternalAudioAACURI != "" {
		fmt.Fprintf(&b, "#EVEY-AUDIO-AAC-URI:%q\n", in.ExternalAudioAACURI)
	}

	// When audio is served as a SEPARATE rendition it is always transcoded to AAC
	// (transcode_audio_handler hardcodes mp4a), so the variant CODECS must
	// advertise mp4a - not the decision's possibly-ac3/eac3 muxed codec. Otherwise
	// Safari validates the master against e.g. an "ec-3" claim on an AAC rendition
	// and refuses it. Muxed path (no renditions) keeps the decision codec.
	effectiveAudioCodec := in.ChosenAudioCodec
	if len(in.AudioRenditions) > 0 {
		effectiveAudioCodec = "aac"
	}
	// Video-only master (wasm-audio path): the CODECS attribute must list the
	// video codec ALONE - advertising any audio codec makes Safari/hls.js
	// validate the master against an audio track the video-only segments don't
	// carry and refuse the stream.
	var codecs string
	if in.ExternalAudio {
		codecs = videoCodecAttr(strings.ToLower(in.ChosenVideoCodec))
	} else {
		codecs = codecsAttr(in.ChosenVideoCodec, effectiveAudioCodec)
	}
	suffix := querySuffix(in)

	// Audio rendition path: emit one #EXT-X-MEDIA per source audio
	// track and tie every variant to the shared "aud" group. The
	// variants now carry video-only segments (the audio playlist holds
	// the audio segments). Pre-feature behaviour stays bit-for-bit
	// identical when in.AudioRenditions is empty - every existing
	// caller, test and client falls through to the muxed path below.
	audioGroup := ""
	if len(in.AudioRenditions) > 0 && !in.ExternalAudio {
		audioGroup = "aud"
		// Mark the REQUESTED audio track (in.AudioTrack, 0-based among audio
		// streams) as DEFAULT=YES so the player auto-selects it. Before this
		// the FIRST rendition was always DEFAULT, so switching language via
		// ?audioTrack= changed the segment URLs but never the DEFAULT flag -
		// the player kept the original language on BOTH web and mobile (root
		// cause found by diffing the served master, 2026-07-25). Fall back to
		// the first rendition when the request is out of range so a default
		// always exists. AUTOSELECT=YES on every rendition so a client can
		// still switch tracks itself.
		defaultRenditionIdx := 0
		for j, r := range in.AudioRenditions {
			if r.SourceTrackIdx == in.AudioTrack {
				defaultRenditionIdx = j
				break
			}
		}
		for i, r := range in.AudioRenditions {
			def := "NO"
			autoSel := "YES"
			if i == defaultRenditionIdx {
				def = "YES"
			}
			name := r.Name
			if name == "" {
				name = fmt.Sprintf("Track %d", r.SourceTrackIdx+1)
			}
			name = hlsAttrSafe(name)
			lang := ""
			if r.LanguageCode != "" {
				lang = fmt.Sprintf(`,LANGUAGE=%q`, hlsAttrSafe(r.LanguageCode))
			}
			fmt.Fprintf(&b,
				"#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=%q,NAME=%q,DEFAULT=%s,AUTOSELECT=%s%s,URI=\"audio_%d.m3u8%s\"\n",
				audioGroup, name, def, autoSel, lang, r.SourceTrackIdx, suffix,
			)
		}
	}

	for i, p := range in.Profiles {
		// BANDWIDTH is the playlist's peak bitrate in bps. We use video + audio
		// (both in kbps from the profile, so multiply by 1000). hls.js uses it
		// to drive ABR switches - if it's wrong the wrong rendition is picked.
		bps := (p.VideoBitrate + p.AudioBitrate) * 1000
		// AVERAGE-BANDWIDTH lets the player factor sustained throughput vs
		// the peak. We estimate it at 85% of peak (typical for VBR encodes
		// with light scenes amortising the high-motion peaks). Optional per
		// HLS spec but hls.js + Safari both use it when present.
		avg := bps * 85 / 100
		res := fmt.Sprintf("%dx%d", p.Width, p.Height)

		audioAttr := ""
		if audioGroup != "" {
			audioAttr = fmt.Sprintf(",AUDIO=%q", audioGroup)
		}
		// Per-rung CODECS: a video-COPY rung advertises the SOURCE's exact
		// avc1.PPCCLL (in.CopyVideoCodec, built from the source profile+level) so
		// Safari/hls.js don't reject a rendition whose real level exceeds the
		// conservative default; transcoded rungs keep the encoder codec.
		rungCodecs := codecs
		videoRangeAttr := ""
		if p.Copy && in.CopyVideoCodec != "" {
			// Video-only master carries no audio codec in CODECS (see above).
			if in.ExternalAudio {
				rungCodecs = in.CopyVideoCodec
			} else {
				rungCodecs = in.CopyVideoCodec + "," + audioCodecAttr(strings.ToLower(effectiveAudioCodec))
			}
			if in.CopyVideoRange != "" {
				videoRangeAttr = ",VIDEO-RANGE=" + in.CopyVideoRange
			}
		}
		fmt.Fprintf(&b,
			"#EXT-X-STREAM-INF:BANDWIDTH=%d,AVERAGE-BANDWIDTH=%d,RESOLUTION=%s,CODECS=%q%s%s\n",
			bps, avg, res, rungCodecs, videoRangeAttr, audioAttr,
		)
		// The video variant's playlist URI. Default stream_<idx>.m3u8; the
		// on-demand copy rung overrides it (single-variant only) so the video
		// rendition is served by the mkvgo packager while the audio group stays
		// Evey's ffmpeg renditions.
		if in.VideoPlaylistURI != "" && len(in.Profiles) == 1 {
			fmt.Fprintf(&b, "%s%s\n", in.VideoPlaylistURI, suffix)
		} else {
			fmt.Fprintf(&b, "stream_%d.m3u8%s\n", i, suffix)
		}
	}

	// Fallback rungs ride last so players that start on the first listed
	// variant keep starting on the main one. Muxed audio+video: no AUDIO
	// attribute (audio renditions only exist for copied video - the rendition
	// offset is the SOURCE's composition delay, wrong under an encoder), and
	// the URI is another plan's, emitted verbatim with its own baked suffix.
	for _, v := range in.FallbackVariants {
		bps := (v.Profile.VideoBitrate + v.Profile.AudioBitrate) * 1000
		fmt.Fprintf(&b,
			"#EXT-X-STREAM-INF:BANDWIDTH=%d,AVERAGE-BANDWIDTH=%d,RESOLUTION=%dx%d,CODECS=%q\n",
			bps, bps*85/100, v.Profile.Width, v.Profile.Height,
			codecsAttr(v.VideoCodec, v.AudioCodec),
		)
		b.WriteString(v.URI + "\n")
	}

	return b.String(), nil
}

// Variant renders one sub-playlist (one ABR rendition). profileIdx must be a
// valid index into in.Profiles. The variant is VOD (PLAYLIST-TYPE:VOD +
// ENDLIST), so hls.js treats it as a complete on-demand stream - no live-edge
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

	// Resume offset is portable across every conformant HLS player: Safari
	// native, AirPlay, tvOS all honour it without the JS layer having to
	// seek post-attach. Clamp to the media duration to stay valid; <= 0
	// emits nothing (pre-refactor behaviour).
	if in.ResumeOffsetSec > 0 {
		offset := in.ResumeOffsetSec
		if offset > in.DurationSec {
			offset = in.DurationSec
		}
		fmt.Fprintf(&b, "#EXT-X-START:TIME-OFFSET=%.3f,PRECISE=YES\n", offset)
	}

	suffix := querySuffix(in)

	if in.UseFMP4 {
		fmt.Fprintf(&b, "#EXT-X-MAP:URI=\"stream_%d_init.mp4%s\"\n", profileIdx, suffix)
	}

	ext := "ts"
	if in.UseFMP4 {
		ext = "m4s"
	}

	for i := range segCount {
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

// AudioPlaylist renders the audio-only sub-playlist for one source
// audio track. trackIdx is the 0-indexed audio stream in the source
// media (matches AudioRendition.SourceTrackIdx). The playlist mirrors
// the Variant() shape - same segment cadence, same VOD framing,
// EXT-X-MAP init - so the player keeps the audio and video timelines
// aligned through `#EXT-X-INDEPENDENT-SEGMENTS` declared on the master.
//
// Only valid when in.AudioRenditions is non-empty AND trackIdx matches
// one of the registered renditions; otherwise returns an error. Always
// emits fMP4 segments - the audio rendition path doesn't support TS.
func (g *ManifestGenerator) AudioPlaylist(in ManifestInput, trackIdx int) (string, error) {
	if err := in.Validate(); err != nil {
		return "", err
	}
	if len(in.AudioRenditions) == 0 {
		return "", fmt.Errorf("AudioPlaylist: no audio renditions registered")
	}
	known := false
	for _, r := range in.AudioRenditions {
		if r.SourceTrackIdx == trackIdx {
			known = true
			break
		}
	}
	if !known {
		return "", fmt.Errorf("AudioPlaylist: trackIdx %d not in AudioRenditions", trackIdx)
	}

	segCount := in.SegmentCount()
	if segCount == 0 {
		return "", fmt.Errorf("AudioPlaylist: empty segment list (duration=%v)", in.DurationSec)
	}

	suffix := querySuffix(in)

	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	b.WriteString("#EXT-X-VERSION:6\n")
	b.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n")
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n", int(math.Ceil(ManifestSegmentSeconds)))
	b.WriteString("#EXT-X-MEDIA-SEQUENCE:0\n")

	if in.ResumeOffsetSec > 0 {
		offset := in.ResumeOffsetSec
		if offset > in.DurationSec {
			offset = in.DurationSec
		}
		fmt.Fprintf(&b, "#EXT-X-START:TIME-OFFSET=%.3f,PRECISE=YES\n", offset)
	}

	fmt.Fprintf(&b, "#EXT-X-MAP:URI=\"audio_%d_init.mp4%s\"\n", trackIdx, suffix)

	for i := range segCount {
		segDur := ManifestSegmentSeconds
		if i == segCount-1 {
			leftover := in.DurationSec - float64(i)*ManifestSegmentSeconds
			if leftover > 0 && leftover < ManifestSegmentSeconds {
				segDur = math.Round(leftover*1000) / 1000
			}
		}
		fmt.Fprintf(&b, "#EXTINF:%.3f,\n", segDur)
		fmt.Fprintf(&b, "audio_%d_%03d.m4s%s\n", trackIdx, i, suffix)
	}

	b.WriteString("#EXT-X-ENDLIST\n")
	return b.String(), nil
}

// hlsAttrSafe strips characters RFC 8216 §4.2 forbids inside a quoted-string
// (", CR, LF) plus other control chars from a value (a track NAME/LANGUAGE comes
// from file metadata). %q would backslash-escape them, which hls.js's "([^"]*)"
// attribute parser then mis-reads - corrupting the rest of the EXT-X-MEDIA line
// and dropping the rendition. Removing them keeps the attribute list parseable.
func hlsAttrSafe(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '"' || r == '\r' || r == '\n' || r < 0x20 {
			return -1
		}
		return r
	}, s)
}

// codecsAttr returns the RFC 6381 codecs string for the master playlist
// `CODECS=` attribute, given the chosen video + audio codec from the ffmpeg
// decision. Safari validates this string up front and refuses to fetch
// segments when it doesn't match its decoder capability - historically we
// hardcoded `avc1.640028,mp4a.40.2` which silently locked Safari out of
// any HEVC / AV1 / VP9 rendition. The strings here are deliberately
// permissive within each codec family (no profile/level enumeration per
// rendition) - that's the right tradeoff: we don't yet probe per-rendition
// profile, and a too-narrow string is what bricks Safari.
//
// Audio falls back to AAC-LC when unknown. Empty inputs ⇒ pre-refactor
// behaviour (H.264 + AAC-LC).
func codecsAttr(videoCodec, audioCodec string) string {
	v := videoCodecAttr(strings.ToLower(videoCodec))
	a := audioCodecAttr(strings.ToLower(audioCodec))
	return v + "," + a
}

func videoCodecAttr(c string) string {
	switch c {
	case "hevc", "h265", "x265":
		// hvc1.1.6.L93.B0 = HEVC main profile, main tier, level 3.1 - covers
		// up to 1080p30. For 4K + main10 you'd want hvc1.2.4.L150.B0; we keep
		// the broader string because we don't have per-rendition profile/level
		// data wired through yet, and Safari accepts the permissive form.
		return "hvc1.1.6.L93.B0"
	case "av1", "av01":
		// av01.0.04M.08 = Main profile, level 3.0, Main tier, 8-bit. The transcode
		// output is always yuv420p (8-bit) - declaring 10-bit (.10) made MSE create
		// a 10-bit SourceBuffer the 8-bit segments never matched, so hls.js
		// downloaded everything and rendered nothing (perpetual buffering). 8-bit
		// also matches the client capability probe (av01.0.00M.08 in codec.ts).
		return "av01.0.04M.08"
	case "vp9":
		return "vp09.00.10.08"
	case "", "h264", "avc", "libx264":
		// avc1.640028 = High profile @ L4.0 (1080p baseline). Pre-refactor
		// default; keep for backward compat when ChosenVideoCodec is empty.
		return "avc1.640028"
	default:
		return "avc1.640028"
	}
}

// avcCodecString builds the EXACT RFC-6381 "avc1.PPCCLL" for an H.264 stream
// from its ffprobe profile name + level_idc (e.g. High + 40 → "avc1.640028").
// Used for the video-COPY rung so the master advertises the SOURCE's real
// profile/level instead of the conservative hardcoded default - a too-low
// advertised level makes Safari/hls.js refuse a higher-level copied rendition.
// PP=profile_idc, CC=constraint flags (00, correct for the High/Main 8-bit SDR
// copy scope), LL=level_idc (the ffprobe level int, 40 = L4.0 = 0x28).
func avcCodecString(profile string, level int) string {
	return fmt.Sprintf("avc1.%02x00%02x", avcProfileIDC(profile), level)
}

// avcProfileIDC maps an ffprobe H.264 profile name to its profile_idc byte.
func avcProfileIDC(profile string) int {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "high":
		return 0x64
	case "main":
		return 0x4d
	case "baseline", "constrained baseline":
		return 0x42
	default:
		return 0x64 // High - the safe default for the 8-bit SDR copy scope
	}
}

// hevcCodecString builds the EXACT RFC-6381 HEVC codec string for the
// video-COPY rung from the ffprobe profile name + level_idc, e.g.
// Main 10 + 153 → "hvc1.2.4.L153.B0" (Main 10, Main tier, level 5.1).
// Layout: hvc1.<profile_idc>.<compatibility flags>.L<level_idc>.<constraints>.
// The compatibility flags are the profile_idc bit reversed-and-hexed: profile
// 1 (Main) ⇒ 6 (Main 10 decoders also decode Main), profile 2 (Main 10) ⇒ 4.
// Main tier ("L") is assumed - High-tier consumer files are effectively
// nonexistent, and ffprobe doesn't surface the tier. "hvc1" (not "hev1") is
// the sample-entry Safari requires; the mux side tags the track accordingly
// (-tag:v hvc1).
func hevcCodecString(profile string, level int) string {
	idx := hevcProfileIdx(profile)
	if idx == 0 {
		return "" // unknown profile - the eligibility guard already rejected it
	}
	compat := 6 // Main
	if idx == 2 {
		compat = 4 // Main 10
	}
	return fmt.Sprintf("hvc1.%d.%d.L%d.B0", idx, compat, level)
}

// hevcProfileIdx maps an ffprobe HEVC profile name to its profile_idc, or 0
// when unknown (only Main / Main 10 are wired - the consumer-content pair).
func hevcProfileIdx(profile string) int {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "main":
		return 1
	case "main 10":
		return 2
	}
	return 0
}

// av1CodecString builds the RFC-6381 AV1 codec string from the ffprobe
// profile + seq_level_idx, e.g. Main + 8 + 10-bit → "av01.0.08M.10".
// Layout: av01.<profile>.<level><tier>.<bitDepth>. Main tier assumed (the
// consumer-content norm; ffprobe doesn't surface the tier).
func av1CodecString(profile string, level int, tenBit bool) string {
	idx := av1ProfileIdx(profile)
	if idx < 0 {
		return ""
	}
	depth := "08"
	if tenBit {
		depth = "10"
	}
	return fmt.Sprintf("av01.%d.%02dM.%s", idx, level, depth)
}

// av1ProfileIdx maps an ffprobe AV1 profile name to its profile index, or -1
// when unknown. Only Main (0) is wired - High (4:4:4) and Professional
// (12-bit/4:2:2) have no meaningful browser decode.
func av1ProfileIdx(profile string) int {
	if strings.EqualFold(strings.TrimSpace(profile), "main") {
		return 0
	}
	return -1
}

// vp9CodecString builds the RFC-6381 VP9 codec string from the ffprobe
// profile + level, e.g. Profile 0 + 41 → "vp09.00.41.08". Profile 2 is the
// 10-bit variant browsers decode; 1/3 (4:2:2/4:4:4) are not wired.
func vp9CodecString(profile string, level int, tenBit bool) string {
	idx := vp9ProfileIdx(profile)
	if idx < 0 {
		return ""
	}
	depth := "08"
	if tenBit || idx == 2 {
		depth = "10"
	}
	return fmt.Sprintf("vp09.%02d.%02d.%s", idx, level, depth)
}

// vp9ProfileIdx maps an ffprobe VP9 profile name ("Profile 0"…) to its index,
// or -1 when unknown/unwired (profiles 1 and 3 have no browser decode).
func vp9ProfileIdx(profile string) int {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "profile 0":
		return 0
	case "profile 2":
		return 2
	}
	return -1
}

func audioCodecAttr(c string) string {
	switch c {
	case "ac3", "ac-3":
		return "ac-3"
	case "eac3", "ec-3":
		return "ec-3"
	case "opus":
		return "opus"
	case "flac":
		return "flac"
	case "", "aac", "aac-lc", "mp4a":
		return "mp4a.40.2"
	default:
		return "mp4a.40.2"
	}
}

// QuerySuffixFor exposes the baked query suffix so the HTTP layer can
// post-process producer-owned playlists (the copy manifest emits bare URIs;
// they must carry the same auth + decision params as the generator's own).
func QuerySuffixFor(in ManifestInput) string { return querySuffix(in) }

// querySuffix builds the `?…` to append to every URL in the playlists. The
// stream token + decision params are baked into each entry so hls.js doesn't
// need to negotiate auth per request.
func querySuffix(in ManifestInput) string {
	q := url.Values{}
	if in.StreamToken != "" {
		q.Set("token", in.StreamToken)
	}
	if in.FingerprintHash != "" {
		// The rendition identity seal - see the field doc. Rides every URI the
		// manifest emits, exactly like the decision inputs below.
		q.Set("fp", in.FingerprintHash)
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
	if in.HDRCapable {
		// Decision input like the codec lists: dropping it flips the segment
		// decision from copy to tonemap-transcode mid-presentation (see field doc).
		q.Set("hdr", "1")
	}
	if in.MaxAudioChannels > 0 {
		// Audio-rendition decision input: without it the rendition falls back to
		// the source layout and a stereo-only sink gets 5.1 (see field doc).
		q.Set("maxAudioChannels", fmt.Sprintf("%d", in.MaxAudioChannels))
	}
	if in.AudioOffsetMs != 0 {
		q.Set("audioOffsetMs", fmt.Sprintf("%d", in.AudioOffsetMs))
	}
	if in.PinnedVideoDecision != "" {
		// Pins the playback's video decision across playlist polls (see field
		// doc) - the fix for the mid-play transcode→copy flip the keyframe
		// backfill caused on a live session (green frames on HW decoders).
		q.Set("vd", in.PinnedVideoDecision)
	}
	if in.PinnedAudioDecision != "" {
		q.Set("ad", in.PinnedAudioDecision)
	}
	if in.WasmAudio {
		// Audio-rendition decision input: keeps the passthrough (AC-3 copy)
		// segments/init aligned with the playlist that named them (see field doc).
		q.Set("wasmAudio", "1")
	}
	if encoded := q.Encode(); encoded != "" {
		return "?" + encoded
	}
	return ""
}

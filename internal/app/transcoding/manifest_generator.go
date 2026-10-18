package transcoding

import (
	"fmt"
	"math"
	"net/url"
	"slices"
	"strings"

	"github.com/eveyproject/evey/internal/domain/transcoding"
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
	// Grid.SegmentCount segments. Must be > 0 - callers fall back on a
	// "growing event" manifest path when duration is unknown.
	DurationSec float64

	// Grid is the segmentation the presentation is cut on (the fingerprint's):
	// segment count, every EXTINF and the DASH timeline come from it. The
	// zero value is the legacy 3 s clock.
	Grid transcoding.SegmentGrid

	// CopySegments is the keyframe plan of a copy rung (variable spans), the
	// DASH timeline of a presentation the ffmpeg copy fallback serves. Only
	// read by MPD; the copy rung's HLS playlist comes from the producer.
	CopySegments []CopySegment

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

	// FMP4AudioPassthrough is the client's audio-passthrough opt-in (today: mp3
	// and nothing else). A DECISION INPUT, so like the codec lists it MUST ride
	// every URI the manifest emits: the segment handler re-parses the query to
	// rebuild the plan, and dropping it there would flip the audio verdict from
	// copy to AAC re-encode halfway through a presentation - the master would
	// advertise mp4a.6B over segments carrying AAC-LC.
	FMP4AudioPassthrough []string

	// PixelFormats is the client's pixel-format declaration (?pixelFormats),
	// a video DECISION INPUT like the codec lists: a segment that lost it
	// would re-judge the source on the browser rule and flip copy/transcode
	// under the master. Empty for every client that declares nothing.
	PixelFormats []string

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

	// TranscodeVideoRange ("PQ"/"HLG") marks the transcoded rungs as HDR: the
	// HEVC Main10 passthrough. "" = SDR transcode, as always.
	TranscodeVideoRange string

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
	// SubtitleRenditions, when non-empty, declares a `#EXT-X-MEDIA
	// TYPE=SUBTITLES,GROUP-ID="subs"` per text track and ties every variant
	// to it. The URIs are the on-demand packager's windowed WebVTT playlists,
	// served next to the video segments - no full-file extraction ahead of
	// playback. X-EVEY-TRACK carries the subtitle-relative ordinal the
	// players' pickers count in.
	SubtitleRenditions []SubtitleRendition

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
	// ScreenHeight (?screenHeight), MaxHeight (?maxHeight), MaxH264Level
	// (?maxH264Level) and MaxH264Height (?maxH264Height) shape the ladder like
	// the codec lists do, so they ride every baked URL for the same reason.
	ScreenHeight  int
	MaxHeight     int
	MaxH264Level  int
	MaxH264Height int
	// NetworkType (?networkType) brings the profile's cellular bitrate cap into
	// the decision: a segment that lost it would re-decide a copy at the source
	// bitrate under a master that advertised a capped ladder. Only "cellular"
	// is baked; Wi-Fi (the default) leaves every URI as it was.
	NetworkType string

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

	// LineKbps (?maxKbps) and ABRFloor (?abr=floor) are the client's line
	// constraints (PlaybackRequest): plan inputs, so they ride every baked URL
	// like the pins - a child that lost them would resolve another plan.
	LineKbps int64
	ABRFloor bool

	// PinnedAudioDecision ("copy"|"transcode") - same contract as
	// PinnedVideoDecision, for the AUDIO side: an admin toggle (loudnorm) or
	// background enrichment can flip audio copy-eligibility between two polls
	// of a live playlist, switching the served audio codec under the player.
	PinnedAudioDecision string

	// PinnedVideoCodec (vc=) rides every baked URL of a plan whose output codec
	// was pinned (the copy master's fallback rung), so the segment-time
	// resolution lands on the same codec, hence the same fingerprint.
	PinnedVideoCodec string

	// VideoPlaylistURI, when set, overrides the video variant's playlist URI
	// (the default is stream_<idx>.m3u8). It is honoured ONLY for a
	// single-variant input - the on-demand copy rung uses it to point the one
	// video variant at the mkvgo packager's playlist (od_playlist.m3u8) while
	// the audio renditions stay Evey's ffmpeg pipeline (the hybrid master:
	// stateless video from PlanHLS + transcoded AAC audio). Empty preserves the
	// stream_N.m3u8 default bit-for-bit.
	VideoPlaylistURI string

	// FirstRungIndex is the ladder index of Profiles[0]: a plan fitted to one
	// rung advertises that rung alone, and its fragments keep their ladder name
	// (stream_<rung>_*). 0 = Profiles is the ladder from its first rung.
	FirstRungIndex int

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
// codecs, not this master's). A muxed rung carries both in CODECS; a VideoOnly
// one (its plan serves audio as renditions) plays this master's audio group.
type FallbackVariant struct {
	Profile    transcoding.HLSProfile
	URI        string
	VideoCodec string
	// VideoRange is the rung's HLS VIDEO-RANGE ("PQ"/"HLG" = HDR, "" = SDR).
	// Carried so the rung's ABR group (codec family + HDR) can be checked
	// against the copy rung it sits next to - a mismatch must never be an
	// auto-switch variant (see ABRGroup / copyFallbackVariant's gate).
	VideoRange string
	AudioCodec string
	// VideoOnly: the rung's segments carry no audio.
	VideoOnly bool
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
	// Codec the rendition's segments carry ("aac" when re-encoded, the source
	// codec when copied; "" reads as aac). The master's CODECS lists every
	// distinct one, so a client validating the attribute never meets an
	// unannounced sample entry.
	Codec string
	// SampleRate (Hz), Channels and BitrateKbps are what the fragments carry:
	// the source track's on a copy, the AAC encode's otherwise (48 kHz, the
	// resolved channel count, the scaled bitrate). 0 = unknown.
	SampleRate  int
	Channels    int
	BitrateKbps int64
	// Profile is the copied source track's profile as the prober reports it
	// ("LC", "HE-AAC", "HE-AACv2"); "" for a re-encode (AAC-LC) or unknown.
	Profile string
}

// codecAttr is the rendition's codecs attribute (see renditionCodecAttr).
func (r AudioRendition) codecAttr() string { return renditionCodecAttr(r.Codec, r.Profile) }

// SubtitleRendition is one text subtitle track served as segmented WebVTT.
// TrackIdx is the subtitle-relative ordinal (0-based among the source's
// subtitle streams, image tracks included) - the number the pickers and the
// /subtitle/{track} route count in. URI is the bare playlist name; the
// master's own query suffix is appended.
type SubtitleRendition struct {
	TrackIdx     int
	Name         string
	LanguageCode string
	URI          string
	Forced       bool
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
// duration and the grid. The last segment is shorter whenever the duration
// isn't an exact multiple.
func (in *ManifestInput) SegmentCount() int {
	return in.Grid.SegmentCount(in.DurationMs())
}

// DurationMs is DurationSec as the millisecond figure the grid arithmetic
// takes.
func (in *ManifestInput) DurationMs() int64 {
	return int64(math.Round(in.DurationSec * 1000))
}

// targetDuration is the EXT-X-TARGETDURATION: the longest EXTINF rounded up
// (3.003 s segments need 4).
func (in *ManifestInput) targetDuration() int {
	return int(math.Ceil(in.Grid.SegmentSeconds()))
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
	audioAttrs := audioCodecAttr(strings.ToLower(in.ChosenAudioCodec))
	if len(in.AudioRenditions) > 0 {
		// Every distinct codec the rendition group carries, in rendition
		// order: a copied AC-3 next to a re-encoded AAC is "ac-3,mp4a.40.2".
		var attrs []string
		for _, r := range in.AudioRenditions {
			a := r.codecAttr()
			if !slices.Contains(attrs, a) {
				attrs = append(attrs, a)
			}
		}
		audioAttrs = strings.Join(attrs, ",")
	}
	// Video-only master (wasm-audio path): the CODECS attribute must list the
	// video codec ALONE - advertising any audio codec makes Safari/hls.js
	// validate the master against an audio track the video-only segments don't
	// carry and refuse the stream.
	suffix := querySuffix(in)
	tenBit := in.TranscodeVideoRange != ""
	codecsForRung := func(p transcoding.HLSProfile) string {
		v := videoCodecAttrFor(strings.ToLower(in.ChosenVideoCodec), p, tenBit, in.Grid.FPS())
		if in.ExternalAudio {
			return v
		}
		return v + "," + audioAttrs
	}

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
		defaultRenditionIdx := defaultAudioRendition(in.AudioRenditions, in.AudioTrack)
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

	// Subtitle renditions: DEFAULT/AUTOSELECT off on purpose - the app picks
	// the track (profile preference, forced flag, the viewer's menu); a
	// player-side auto-selection would fight it.
	subsGroup := ""
	if len(in.SubtitleRenditions) > 0 {
		subsGroup = "subs"
		for _, r := range in.SubtitleRenditions {
			name := r.Name
			if name == "" {
				name = fmt.Sprintf("Subtitle %d", r.TrackIdx+1)
			}
			lang := ""
			if r.LanguageCode != "" {
				lang = fmt.Sprintf(`,LANGUAGE=%q`, hlsAttrSafe(r.LanguageCode))
			}
			forced := ""
			if r.Forced {
				forced = ",FORCED=YES"
			}
			fmt.Fprintf(&b,
				"#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID=%q,NAME=%q,DEFAULT=NO,AUTOSELECT=NO%s%s,X-EVEY-TRACK=\"%d\",URI=\"%s%s\"\n",
				subsGroup, hlsAttrSafe(name), lang, forced, r.TrackIdx, r.URI, suffix,
			)
		}
	}
	subsAttr := ""
	if subsGroup != "" {
		subsAttr = fmt.Sprintf(",SUBTITLES=%q", subsGroup)
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
		// The picture the rung is encoded at, not its box: a 4:3 source in the
		// 360p box is 480x360, a scope one in the 1080p box 1920x800.
		fw, fh := p.Frame()
		res := fmt.Sprintf("%dx%d", fw, fh)

		audioAttr := ""
		if audioGroup != "" {
			audioAttr = fmt.Sprintf(",AUDIO=%q", audioGroup)
		}
		// Per-rung CODECS: a video-COPY rung advertises the SOURCE's exact
		// avc1.PPCCLL (in.CopyVideoCodec, built from the source profile+level) so
		// Safari/hls.js don't reject a rendition whose real level exceeds the
		// conservative default; transcoded rungs keep the encoder codec.
		rungCodecs := codecsForRung(p)
		videoRangeAttr := ""
		if !p.Copy && in.TranscodeVideoRange != "" {
			videoRangeAttr = ",VIDEO-RANGE=" + in.TranscodeVideoRange
		}
		if p.Copy && in.CopyVideoCodec != "" {
			// Video-only master carries no audio codec in CODECS (see above).
			if in.ExternalAudio {
				rungCodecs = in.CopyVideoCodec
			} else {
				rungCodecs = in.CopyVideoCodec + "," + audioAttrs
			}
			if in.CopyVideoRange != "" {
				videoRangeAttr = ",VIDEO-RANGE=" + in.CopyVideoRange
			}
		}
		fmt.Fprintf(&b,
			"#EXT-X-STREAM-INF:BANDWIDTH=%d,AVERAGE-BANDWIDTH=%d,RESOLUTION=%s,CODECS=%q%s%s%s\n",
			bps, avg, res, rungCodecs, videoRangeAttr, audioAttr, subsAttr,
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
	// variant keep starting on the main one. The URI is another plan's,
	// emitted verbatim with its own baked suffix. A muxed rung names its own
	// audio codec; a video-only one plays this master's audio group (no track
	// switch on the ABR drop), and without a group it would be silent, so it is
	// not advertised.
	for _, v := range in.FallbackVariants {
		// The rung is encoded under the same contract as this master's (same
		// client, same source grid).
		vattr := videoCodecAttrFor(strings.ToLower(v.VideoCodec), v.Profile, v.VideoRange != "", in.Grid.FPS())
		codecs, audioAttr := vattr+","+audioCodecAttr(strings.ToLower(v.AudioCodec)), ""
		if v.VideoOnly {
			if audioGroup == "" {
				continue
			}
			codecs, audioAttr = vattr+","+audioAttrs, fmt.Sprintf(",AUDIO=%q", audioGroup)
		}
		bps := (v.Profile.VideoBitrate + v.Profile.AudioBitrate) * 1000
		fw, fh := v.Profile.Frame()
		fmt.Fprintf(&b,
			"#EXT-X-STREAM-INF:BANDWIDTH=%d,AVERAGE-BANDWIDTH=%d,RESOLUTION=%dx%d,CODECS=%q%s%s\n",
			bps, bps*85/100, fw, fh,
			codecs, audioAttr, subsAttr,
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
	// at most one grid segment, ceiling that is the safe upper bound and
	// matches what ffmpeg would emit naturally.
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n", in.targetDuration())
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

	durMs := in.DurationMs()
	for i := range segCount {
		// Every segment is one grid segment, the last one the leftover: the
		// exact frames on a frame grid, a float leftover on the legacy clock.
		fmt.Fprintf(&b, "#EXTINF:%.3f,\n", in.Grid.SegmentDurationSeconds(i, durMs))
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
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n", in.targetDuration())
	b.WriteString("#EXT-X-MEDIA-SEQUENCE:0\n")

	if in.ResumeOffsetSec > 0 {
		offset := in.ResumeOffsetSec
		if offset > in.DurationSec {
			offset = in.DurationSec
		}
		fmt.Fprintf(&b, "#EXT-X-START:TIME-OFFSET=%.3f,PRECISE=YES\n", offset)
	}

	fmt.Fprintf(&b, "#EXT-X-MAP:URI=\"audio_%d_init.mp4%s\"\n", trackIdx, suffix)

	durMs := in.DurationMs()
	for i := range segCount {
		// The rendition is cut on the video's grid: the same cadence, declared
		// the same way (the muxer closes an audio segment on the first frame
		// past each boundary, so this is the declared, not the measured, span).
		fmt.Fprintf(&b, "#EXTINF:%.3f,\n", in.Grid.SegmentDurationSeconds(i, durMs))
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

// videoCodecAttrFor is the RFC 6381 codecs value of a transcoded rung: the
// profile and the level the encoder is told to write for it (the rung's
// CodecLevel, see the domain's codec_string.go), so the manifests declare
// what the init segment holds. A rung the plan did not level (0) is levelled
// here the way the encoder levels it on its own: at the software ceiling of
// the static policy, the higher one. fps is the grid's (0 when unknown).
func videoCodecAttrFor(c string, p transcoding.HLSProfile, main10 bool, fps float64) string {
	level := p.CodecLevel
	if level == 0 {
		level = transcoding.CodecLevel(c, p, fps, transcoding.StaticQualityPolicy{}.TargetFor(p, transcoding.EncodeRoute{}))
	}
	switch c {
	case "hevc", "h265", "x265":
		return transcoding.HEVCCodecString(level, main10)
	case "av1", "av01":
		// av01.0.04M.08 = Main profile, level 3.0, Main tier, 8-bit. The transcode
		// output is always yuv420p (8-bit) - declaring 10-bit (.10) made MSE create
		// a 10-bit SourceBuffer the 8-bit segments never matched, so hls.js
		// downloaded everything and rendered nothing (perpetual buffering). 8-bit
		// also matches the client capability probe (av01.0.00M.08 in codec.ts).
		return "av01.0.04M.08"
	case "vp9":
		return "vp09.00.10.08"
	default:
		return transcoding.H264CodecString(level)
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
	// An MP3 passthrough rung (opt-in clients only, see fmp4PassthroughOptIn)
	// carries an `mp4a` sample entry whose esds declares
	// objectTypeIndication 0x6B, MPEG-1 Audio - read off the bytes ffmpeg
	// actually writes for `-c:a copy` into fMP4 (2026-09-06). The default
	// below would have advertised mp4a.40.2 (AAC-LC) over MP3 samples, which
	// is precisely the manifest-vs-bytes disagreement AudioCopyEligible exists
	// to prevent.
	case "mp3":
		return "mp4a.6B"
	case "", "aac", "aac-lc", "mp4a":
		return "mp4a.40.2"
	default:
		return "mp4a.40.2"
	}
}

// aacObjectTypes maps the AAC profile names (ffprobe's, and the mkvgo
// prober's read of the AudioSpecificConfig) to their MPEG-4 audio object
// type: the RFC 6381 "mp4a.40.<AOT>" of a copied track.
var aacObjectTypes = map[string]int{
	"main": 1, "lc": 2, "ssr": 3, "ltp": 4, "he-aac": 5, "ld": 23, "he-aacv2": 29, "eld": 39,
}

// AACObjectType is the audio object type of an AAC profile name, false when
// the profile is unknown or absent.
func AACObjectType(profile string) (int, bool) {
	aot, ok := aacObjectTypes[strings.ToLower(strings.TrimSpace(profile))]
	return aot, ok
}

// renditionCodecAttr is audioCodecAttr refined by the copied AAC track's
// profile: an HE-AAC copy is mp4a.40.5, HE-AACv2 mp4a.40.29. A rendition
// with no profile is a re-encode (AAC-LC, mp4a.40.2): an AAC of unknown
// profile is never copied (AACCopyDeclarable).
func renditionCodecAttr(codec, profile string) string {
	codec = strings.ToLower(codec)
	if aot, ok := AACObjectType(profile); ok && (codec == "aac" || codec == "") {
		return fmt.Sprintf("mp4a.40.%d", aot)
	}
	return audioCodecAttr(codec)
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
	if len(in.FMP4AudioPassthrough) > 0 {
		// Decision input like the codec lists: without it the segment request
		// re-derives a re-encode under a master that promised a passthrough.
		q.Set("fmp4AudioPassthrough", strings.Join(in.FMP4AudioPassthrough, ","))
	}
	if len(in.PixelFormats) > 0 {
		q.Set("pixelFormats", strings.Join(in.PixelFormats, ","))
	}
	if in.HDRCapable {
		// Decision input like the codec lists: dropping it flips the segment
		// decision from copy to tonemap-transcode mid-presentation (see field doc).
		q.Set("hdr", "1")
	}
	if in.ScreenHeight > 0 {
		q.Set("screenHeight", fmt.Sprintf("%d", in.ScreenHeight))
	}
	if in.MaxHeight > 0 {
		q.Set("maxHeight", fmt.Sprintf("%d", in.MaxHeight))
	}
	if in.MaxH264Level > 0 {
		q.Set("maxH264Level", fmt.Sprintf("%d", in.MaxH264Level))
	}
	if in.MaxH264Height > 0 {
		q.Set("maxH264Height", fmt.Sprintf("%d", in.MaxH264Height))
	}
	if in.NetworkType == "cellular" {
		q.Set("networkType", in.NetworkType)
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
	if in.LineKbps > 0 {
		q.Set("maxKbps", fmt.Sprintf("%d", in.LineKbps))
	}
	if in.ABRFloor {
		q.Set("abr", "floor")
	}
	if in.PinnedVideoCodec != "" {
		q.Set("vc", in.PinnedVideoCodec)
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

// defaultAudioRendition is the index of the rendition the viewer asked for
// (?audioTrack=, 0-based among the source's audio streams), the first when
// the request names none of them: the one HLS marks DEFAULT=YES and DASH
// Role main.
func defaultAudioRendition(renditions []AudioRendition, audioTrack int) int {
	for j, r := range renditions {
		if r.SourceTrackIdx == audioTrack {
			return j
		}
	}
	return 0
}

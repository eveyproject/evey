package transcoding

// resolver_fingerprint.go - the cache-key object as a PROJECTION of the
// resolved route and the shaped ladder (ADR-0030, 5).

import (
	"context"
	"strings"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// audioChannelsFor is the channel count of the track the VIDEO segments carry:
// the selected track's, or 0 (unknown, guard on) when the audio is served as
// its own rendition (videoAudioTrack -1).
func audioChannelsFor(snap SourceSnapshot, videoAudioTrack, selected int) int {
	if videoAudioTrack != selected {
		return 0
	}
	return snap.AudioChannels
}

// fingerprintFor PROJECTS the resolved route and the shaped ladder onto the
// cache-key object: it decides nothing about codec, backend or pipeline.
func (pr *PlanResolver) fingerprintFor(ctx context.Context, req PlaybackRequest, decision transcoding.PlaybackDecision, route ResolvedRoute, profiles []transcoding.HLSProfile, pinKey string) DecisionFingerprint {
	streams := req.Streams
	audioTrack := req.AudioTrack
	subBurnIn := req.SubtitleBurnInTrack

	// When the audio-rendition feature is on, the audio is served as a SEPARATE
	// HLS rendition (audio_T.m3u8 / audio_T_*.m4s) and the video segments MUST be
	// video-only. Otherwise stream_0 keeps muxing the audio track while the master
	// also advertises the separate audio group with DEFAULT=YES - hls.js then
	// appends audio twice (muxed-in-stream_0 + audio_0), which throws
	// bufferAppendError on any seek. Signalling "no audio" with track -1 reuses the
	// tested video-only ladder (hasAudio) and forks the cache (atrack=-1 in Hash())
	// so muxed and video-only segments for the same media never alias.
	videoAudioTrack := audioTrack
	// Same video-only requirement for the wasm-audio path: the AC-3/E-AC-3 audio
	// is served as a SEPARATE rendition (mkvgo od_audio* or ffmpeg audio_*) and
	// decoded client-side, so the ffmpeg video segments must carry no audio track
	// (atrack=-1 forks the cache off the muxed ladder). Harmless when the video is
	// served by mkvgo instead (this ffmpeg fingerprint is then unused).
	//
	// GATED ON THE TRACK CODEC, like the master and the audio handler: since
	// wasm ON, the cast receiver rides ?wasmAudio=1 on EVERY cast, and for a
	// plain-AAC track the server refuses the wasm master and serves the NORMAL
	// muxed one - stripping the audio here anyway shipped audio-free segments
	// under a master advertising audio (CAF 104/905, prod 2026-07-29).
	// Same predicate as the master handler and the decision: the codec alone is
	// not the condition, the channel gate is part of it.
	wasmAudioApplies := req.WasmAudio && WasmAudioApplies(streams, audioTrack, req.Prefs.MaxAudioChannels)
	snap := snapshot(req)
	renditionAudio := pr.AudioRenditionEnabled && !snap.Telecine &&
		(decision.VideoDecision == "copy" || (decision.VideoDecision == "transcode" && pr.TranscodeAudioRendition))
	if renditionAudio || wasmAudioApplies {
		videoAudioTrack = -1
	}

	fpProfiles := make([]ProfileFingerprint, len(profiles))
	for i, p := range profiles {
		// Single, reflection-guarded projection (TestProfileFingerprintOf_CopiesEveryField).
		fpProfiles[i] = ProfileFingerprintOf(p)
	}
	// Each rung's rate control, from the policy the ladder was shaped under.
	encode := route.Encode()
	quality := make([]transcoding.QualityTarget, len(profiles))
	for i, p := range profiles {
		quality[i] = pr.QualityPolicy().TargetFor(p, encode)
	}
	// The route's projections: the codec the segments carry (capability
	// gate, spill, codec fallback already applied), the frozen backend, the
	// pipeline shape. A spilled stream reads as software here because its
	// route IS software - LadderClassOf(fp) then bills the CPU pool.
	hwUsable := route.HW()
	topology := route.Topology
	normalize := pr.AudioNormalize != nil && pr.AudioNormalize(ctx)
	srcAudio := snap.AudioCodec
	audioCopy := AudioCopyEligible(decision, normalize, len(fpProfiles), srcAudio, req.Client) &&
		AACCopyDeclarable(srcAudio, snap.AudioProfile)
	fp := DecisionFingerprint{
		MediaID:           req.MediaID,
		SourcePath:        req.SourcePath,
		SourceFingerprint: req.SourceFingerprint,
		VideoCodec:        route.Codec,
		// The SOURCE codec (excluded from Hash - derivable from MediaID): the
		// copy builder tags the fMP4 sample entry from it (-tag:v hvc1).
		SourceVideoCodec: snap.Codec,
		// The decode workload admission prices (excluded from Hash, as the codec).
		SourceVideoHeight: snap.Height,
		SourceFPS:         snap.FPS,
		SourceDurationMs:  req.SourceDurationMs,
		// The segmentation the whole presentation is cut on (hashed when exact).
		Grid: SourceGrid(req.Streams),
		// Legacy-codec corrections (B3): idet content verdict + anamorphic SAR,
		// both hashed conditionally so progressive/square titles keep their
		// cache. No-ops on modern progressive sources.
		DeinterlaceAll: snap.DeinterlaceAll,
		Telecine:       snap.Telecine,
		SourceSAR:      snap.SAR,
		// The codec the segments actually carry (source codec on passthrough).
		// The builder only reads it on the transcode branch; here it keeps the
		// hash truthful per served bytes.
		AudioCodec:          ServedAudioCodec(audioCopy, srcAudio, decision),
		AudioTrack:          videoAudioTrack,
		SubtitleBurnInTrack: subBurnIn,
		SubtitleBurnInCodec: snap.SubtitleCodec,
		// HDR->SDR tonemap is needed whenever the decision flagged it (HDR source +
		// non-HDR client, or Dolby Vision). Baked into the fingerprint so it (a)
		// enters the cache hash - tonemapped vs not are different bytes - and (b)
		// reaches the orchestrator/builder, which has no source streams to detect it.
		HDRTonemap:       topology.ToneMap != transcoding.ToneMapNone,
		HDRPassthrough:   topology.Output == transcoding.OutputHDRPassthrough,
		HDRTransfer:      strings.ToLower(snap.HDRRange),
		AudioNormalize:   normalize,
		AudioEncoder:     pr.audioEncoderID(),
		MaxAudioChannels: decision.MaxAudioChannels,
		// Gates the fold guard on a real fold; wasm (-1) reads as unknown, guard on.
		SourceAudioChannels: audioChannelsFor(snap, videoAudioTrack, audioTrack),
		// Audio passthrough eligibility - see AudioCopyEligible (shared with
		// the manifest input so the master CODECS always matches the bytes).
		AudioCopy: audioCopy,
		// User audio resync (ms) - hashed conditionally (non-zero only) and
		// threaded to the builder as a net input shift with the source-delay
		// cancel. Decide already forced audio transcode when non-zero.
		AudioOffsetMs:    req.AudioOffsetMs,
		ForceSidecarSubs: decision.SubtitleDecision == "external",
		UseFMP4:          pr.UseFMP4,
		HWAccel:          route.Backend.Name,
		HWDevice:         route.Backend.Device,
		HWFeatures:       route.Backend.Features,
		// Decoupled route (software decode + hwupload), only when the HW path is
		// actually taken. Excluded from Hash() like HWAccel.
		HWSoftwareDecode: topology.SoftwareDecode(),
		// GPU tone-mapping for this request (only when the HW path is actually
		// taken). Excluded from Hash() like HWAccel - a HW- vs software-tonemapped
		// segment is interchangeable in the addressable cache.
		HWTonemap: topology.ToneMap == transcoding.ToneMapGPU,
		// GPU subtitle compositing, same contract as HWTonemap: only when the HW
		// path is actually taken, and excluded from Hash() - the same subtitle on
		// the same frames, whoever blended it.
		HWOverlay:      route.Overlay && hwUsable,
		HWDeinterlace:  route.Deinterlace && hwUsable,
		Profiles:       fpProfiles,
		QualityTargets: quality,
	}
	// Tell the admission shaper which session this pin shaped: while that
	// session holds a limiter slot, the pinned ceiling must not move (see
	// VideoAdmissionShaper - the cache-hash invariant, live edition).
	if pr.AdmissionBind != nil {
		pr.AdmissionBind(pinKey, fp.Hash())
	}
	return fp
}

// IsCopy reports whether this request should be served as a video-COPY
// (remux) rung rather than transcoded. Three conditions, all required: the
// copy path is wired (CopyProducer non-nil), the decision did NOT require a
// video transcode, and the source is copy-eligible (SourceCopyVideoCodec).
// When false, the request routes through the selector exactly as before.
func (pr *PlanResolver) IsCopy(decision transcoding.PlaybackDecision, streams []*library.MediaStream, sourceBytes int64) bool {
	if pr.CopyProducer == nil || !VideoCopyEligible(decision, streams) {
		return false
	}
	// Disk guard: a copy session keeps a WINDOW of segments on disk around the
	// viewer (software_producer_window.go), not the film: its working set is
	// that window's share of the source. If the cache filesystem can't fit it
	// with headroom, fall back to the transcode ladder rather than risk ENOSPC
	// on a small box. Sized on the source it would have refused a 70 GB film on
	// a box with 50 GB free and sent it to the transcode, which keeps every
	// segment of the session - the heavier of the two. Fail-OPEN when the size
	// is unknown or statfs fails. (master- and segment-time both reach this
	// with the same inputs; a free-space flip between them is a rare, bounded
	// 503-retry, not a cache aliasing - the copy hash never collides with a
	// transcode hash.)
	if need := copyWorkingSetBytes(sourceBytes, streams, pr.copyWindowSegs()); need > 0 && pr.FreeBytes != nil {
		if free, err := pr.FreeBytes(); err == nil && free < need+need/10 {
			return false
		}
	}
	return true
}

// copyWindowSegs is how many segments a copy session holds at most, from the
// copy producer's own window; 0 (the whole source) when it has none.
func (pr *PlanResolver) copyWindowSegs() int {
	if w, ok := pr.CopyProducer.(interface{ CopyWindowSegs() int }); ok {
		return w.CopyWindowSegs()
	}
	return 0
}

// copyWorkingSetBytes is the disk a windowed copy session needs: the bytes
// of its window, one segment per keyframe. A segment's bytes follow its
// length in time, not its rank, so the window is sized on the LONGEST run of
// windowSegs consecutive keyframe spans (a stretch of long, static GOPs puts
// far more of the film in the same number of segments than the mean says),
// at twice the source's mean rate for the bitrate peaks, never more than the
// source. The whole source when the window or the keyframe index is unknown;
// 0 when the size is.
func copyWorkingSetBytes(sourceBytes int64, streams []*library.MediaStream, windowSegs int) int64 {
	if sourceBytes <= 0 {
		return 0
	}
	var kf []int64
	for _, s := range streams {
		if s != nil && s.StreamType == library.StreamTypeVideo {
			kf = s.Keyframes
			break
		}
	}
	if windowSegs <= 0 || len(kf) <= windowSegs {
		return sourceBytes
	}
	// The film's span, the last segment counted at the mean gap.
	total := kf[len(kf)-1] - kf[0]
	total += total / int64(len(kf)-1)
	var longest int64
	for i := 0; i+windowSegs < len(kf); i++ {
		longest = max(longest, kf[i+windowSegs]-kf[i])
	}
	// The window reaching the end holds the last segment too.
	longest = max(longest, kf[0]+total-kf[len(kf)-windowSegs])
	if total <= 0 {
		return sourceBytes
	}
	const peakFactor = 2
	need := float64(sourceBytes) * float64(longest) / float64(total) * peakFactor
	return min(sourceBytes, int64(need))
}

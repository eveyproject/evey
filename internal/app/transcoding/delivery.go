package transcoding

import (
	"context"
	"strings"

	"github.com/eveyproject/evey/internal/domain/library"
	domainplayback "github.com/eveyproject/evey/internal/domain/playback"
	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// delivery.go - HOW a plan is served, decided once with the plan.
//
// A request is delivered by one of two engines per track: mkvgo packages the
// container verbatim (no process, no re-encode, stateless), ffmpeg remuxes
// (the copy producer) or re-encodes (the transcode ladder). The audio may
// also be decoded by the client itself (the browser's wasm AC-3 decoder).
// Which engine carries which track used to be decided by the HTTP adapter
// in a chain of fall-throughs (serveMaster), so the plan never knew mkvgo
// existed and the admin's "serving path" was rebuilt by hand at every site.
// Here it is a projection of the request, the decision and the route, like
// every other facet of the plan.
//
// The strategy is what the plan DECIDED. What happened at runtime - a
// packager plan that failed to build, a hybrid master the audio pipeline
// could not serve - is not a new decision: the adapter falls back on the
// ffmpeg path, says so (a counter, a WARN), and the plan stays as resolved.

// Engine carries one track of the presentation.
type Engine string

const (
	// EngineMkvgo packages the source verbatim on demand (mp4.PlanHLS).
	EngineMkvgo Engine = "mkvgo"
	// EngineFFmpeg spawns ffmpeg: the copy producer or the transcode ladder.
	EngineFFmpeg Engine = "ffmpeg"
	// EngineClient: the client decodes the track itself (wasm AC-3/E-AC-3);
	// the master is video-only and the audio rides an out-of-band rendition.
	EngineClient Engine = "client"
)

// DeliveryStrategy names the engine behind each track. Two axes, deliberately
// not one enum: "mkvgo video + ffmpeg audio" (the hybrid master) is a value,
// not a fifth kind.
type DeliveryStrategy struct {
	Video Engine
	Audio Engine
	// AudioTrimmed: the packager carries the SELECTED audio track only, either
	// because the operator asked for a lighter master (the track-trim flag) or
	// because that track is the one the packager can carry verbatim while
	// another track of the file is not (trimming is what unlocks the copy).
	AudioTrimmed bool
}

// ServingPath is the strategy as the admin dashboard reads it, with the
// route's verdicts (copy / transcode, backend) and the fingerprint's audio
// copy folded in. The one place a ServingPath is built from a plan.
//
// The packager never transcodes: video on it IS a copy, whatever the route
// says. Route.Copy is ffmpeg's copy-rung verdict (a wired copy producer, cache
// room), which the packager needs neither of - so a plan could carry the
// video on the packager while its route, priced for the ffmpeg path that
// serves the plan if the packager cannot build the source, said "transcode".
// Read from the route alone, the planned path came out as transcode/mkvgo,
// disagreed with the copy/mkvgo the adapter served, and tripped the
// invariant counter that must stay at zero (measured 2026-09-26).
func (d DeliveryStrategy) ServingPath(route ResolvedRoute, fp DecisionFingerprint) domainplayback.ServingPath {
	p := domainplayback.ServingPath{VideoDecision: "transcode", VideoEngine: string(d.Video), AudioEngine: string(d.Audio)}
	if route.Copy || d.Video == EngineMkvgo {
		p.VideoDecision = "copy"
	} else {
		p.VideoBackend = "software"
		if route.Backend.Name != "" {
			p.VideoBackend = route.Backend.Name
		}
	}
	switch d.Audio {
	case EngineClient:
		p.AudioDecision = "wasm"
	case EngineMkvgo:
		p.AudioDecision = "copy"
	default:
		// The MUXED gate only. An ffmpeg audio RENDITION also copies a
		// carriable track, on a ladder of any height, but whether it does
		// depends on the request's own client declaration - which this
		// projection does not carry. checkDelivery compares engines and the
		// VIDEO decision, never this field, so the gap is inert here; the
		// session is told the truth by audioServedAsCopy on the serving side.
		// Do not "simplify" the two into this one without bringing the client
		// profile along, or the report goes back to calling a copy a transcode.
		p.AudioDecision = "transcode"
		if fp.AudioCopy {
			p.AudioDecision = "copy"
		}
	}
	return p
}

// resolveDelivery is the one derivation of the delivery strategy.
//
// The video goes to the packager when the request is a video copy the source
// carries verbatim (VideoCopyEligible, the packager's own gate - not IsCopy,
// which also demands a wired ffmpeg copy producer and cache room the
// stateless packager never needs) and the packager is wired. The audio
// follows: the client's wasm decoder when the request asks for it and the
// selected track qualifies; the packager when every track (or the selected
// one, trimmed) is CMAF-carriable and no loudnorm re-encodes; else ffmpeg's
// renditions beside the packager's video (the hybrid master) when that
// pipeline is wired - or, without it, the whole presentation on ffmpeg.
func (pr *PlanResolver) resolveDelivery(ctx context.Context, req PlaybackRequest, decision transcoding.PlaybackDecision, route ResolvedRoute) DeliveryStrategy {
	d := DeliveryStrategy{Video: EngineFFmpeg, Audio: EngineFFmpeg}
	if !pr.OnDemandPackager {
		return d
	}
	if pr.wasmAudioServes(req) {
		// The wasm master takes the packager's video whenever the video is
		// copy-eligible, ffmpeg's otherwise; the audio is the client's.
		d.Audio = EngineClient
		if VideoCopyEligible(decision, req.Streams) {
			d.Video = EngineMkvgo
		}
		return d
	}
	if !VideoCopyEligible(decision, req.Streams) {
		return d
	}
	d.Video = EngineMkvgo
	d.AudioTrimmed = pr.OnDemandTrackTrim
	switch {
	case pr.audioAllCarriable(ctx, decision, req.Streams):
		d.Audio = EngineMkvgo
	case pr.audioTrimUnlocksCopy(ctx, decision, req):
		d.Audio, d.AudioTrimmed = EngineMkvgo, true
	case pr.AudioRenditionEnabled && pr.AudioRenditionPipeline:
		d.Audio = EngineFFmpeg // the hybrid master
	default:
		// No audio pipeline to pair with the packager: the whole
		// presentation goes to ffmpeg, as it always did.
		d.Video, d.AudioTrimmed = EngineFFmpeg, false
	}
	return d
}

// audioAllCarriable: the decision copies the audio, no loudnorm re-encodes,
// and EVERY audio track is CMAF-carriable - the packager demuxes them all
// into renditions, so one it cannot carry would strand a client selecting it.
func (pr *PlanResolver) audioAllCarriable(ctx context.Context, decision transcoding.PlaybackDecision, streams []*library.MediaStream) bool {
	if decision.AudioDecision != "copy" || pr.audioNormalizes(ctx) {
		return false
	}
	for _, s := range streams {
		if s != nil && s.StreamType == library.StreamTypeAudio && !audioTrackPackagedVerbatim(s) {
			return false
		}
	}
	return true // no audio track is fine too: the packager packages video-only
}

// audioTrackPackagedVerbatim: the packager carries the track as stored, so
// its codec must be CMAF-carriable and its rate one the decision would copy
// (evaluateAudio's maxCopiedAudioSampleRate); a 96 kHz track beside the
// selected one was served at 96 kHz as a rendition.
func audioTrackPackagedVerbatim(s *library.MediaStream) bool {
	if s.SampleRate != nil && *s.SampleRate > maxCopiedAudioSampleRate {
		return false
	}
	return AudioCopyCodecAllowed(strings.ToLower(s.Codec))
}

// audioTrimUnlocksCopy: the SELECTED track is carriable verbatim while some
// other track of the file is not - packaging that one track alone is what
// keeps the presentation on the packager instead of re-encoding three AC-3
// tracks to AAC for a viewer who plays none of them.
func (pr *PlanResolver) audioTrimUnlocksCopy(ctx context.Context, decision transcoding.PlaybackDecision, req PlaybackRequest) bool {
	if decision.AudioDecision != "copy" || pr.audioNormalizes(ctx) {
		return false
	}
	return AudioCopyCodecAllowed(strings.ToLower(SourceAudioCodec(req.Streams, req.AudioTrack)))
}

func (pr *PlanResolver) audioNormalizes(ctx context.Context) bool {
	return pr.AudioNormalize != nil && pr.AudioNormalize(ctx)
}

// wasmAudioServes: the request asked for the browser's wasm audio decoder
// (WasmAudio, already gated by the cast policy at marshalling) and the
// selected track is one it decodes, within the channel ceiling.
func (pr *PlanResolver) wasmAudioServes(req PlaybackRequest) bool {
	if !req.WasmAudio || !WasmAudioCodecSupported(SourceAudioCodec(req.Streams, req.AudioTrack)) {
		return false
	}
	return WasmAudioChannelsOK(SourceAudioChannels(req.Streams, req.AudioTrack), req.Prefs.MaxAudioChannels)
}

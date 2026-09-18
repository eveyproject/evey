package transcoding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/evey-project/evey/internal/domain/library"
	domainplayback "github.com/evey-project/evey/internal/domain/playback"
	"github.com/evey-project/evey/internal/domain/transcoding"
)

// deliveryStreams is a copyable H.264 source (keyframes, profile, level) with
// the audio tracks given, so the decision's copy verdicts hold on the video.
func deliveryStreams(audio ...string) []*library.MediaStream {
	streams := ladderStreams(1080)
	streams[0].Keyframes = []int64{0, 6000, 12000}
	streams[0].Profile = "High"
	streams[0].Level = new(40)
	for i, codec := range audio {
		streams = append(streams, &library.MediaStream{StreamType: library.StreamTypeAudio, Codec: codec, Channels: new(6), IsDefault: i == 0})
	}
	return streams
}

func deliveryResolver(packager, trim, audioPipeline bool) *PlanResolver {
	return &PlanResolver{
		OnDemandPackager: packager, OnDemandTrackTrim: trim,
		AudioRenditionEnabled: audioPipeline, AudioRenditionPipeline: audioPipeline,
	}
}

var (
	copyBoth  = transcoding.PlaybackDecision{VideoDecision: "copy", AudioDecision: "copy"}
	copyVideo = transcoding.PlaybackDecision{VideoDecision: "copy", AudioDecision: "transcode", AudioCodec: "aac"}
)

// The strategy is a projection of (request, decision, host wiring): the
// packager carries a copy it can package verbatim, the audio follows the
// tracks it can carry, ffmpeg takes the rest - exactly the gates the HTTP
// adapter's fall-through chain applied, now decided once with the plan.
func TestResolveDelivery_Table(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name     string
		pr       *PlanResolver
		req      PlaybackRequest
		decision transcoding.PlaybackDecision
		want     DeliveryStrategy
	}{
		{"no packager wired: ffmpeg carries everything", deliveryResolver(false, false, true),
			PlaybackRequest{Streams: deliveryStreams("aac")}, copyBoth, DeliveryStrategy{Video: EngineFFmpeg, Audio: EngineFFmpeg}},
		{"video not copy-eligible: ffmpeg transcode", deliveryResolver(true, false, true),
			PlaybackRequest{Streams: deliveryStreams("aac")}, transcoding.PlaybackDecision{VideoDecision: "transcode", VideoCodec: "h264", AudioDecision: "copy"},
			DeliveryStrategy{Video: EngineFFmpeg, Audio: EngineFFmpeg}},
		{"copy + every track carriable: the whole presentation on the packager", deliveryResolver(true, false, true),
			PlaybackRequest{Streams: deliveryStreams("aac", "ac3")}, copyBoth, DeliveryStrategy{Video: EngineMkvgo, Audio: EngineMkvgo}},
		{"copy, no audio track: video-only on the packager", deliveryResolver(true, false, true),
			PlaybackRequest{Streams: deliveryStreams()}, copyBoth, DeliveryStrategy{Video: EngineMkvgo, Audio: EngineMkvgo}},
		{"the operator trims to the selected track", deliveryResolver(true, true, true),
			PlaybackRequest{Streams: deliveryStreams("aac", "ac3")}, copyBoth, DeliveryStrategy{Video: EngineMkvgo, Audio: EngineMkvgo, AudioTrimmed: true}},
		{"one track the packager cannot carry, the selected one it can: trimmed copy", deliveryResolver(true, false, true),
			PlaybackRequest{Streams: deliveryStreams("ac3", "truehd"), AudioTrack: 0}, copyBoth, DeliveryStrategy{Video: EngineMkvgo, Audio: EngineMkvgo, AudioTrimmed: true}},
		{"the selected track is the uncarriable one: hybrid master", deliveryResolver(true, false, true),
			PlaybackRequest{Streams: deliveryStreams("ac3", "truehd"), AudioTrack: 1}, copyVideo, DeliveryStrategy{Video: EngineMkvgo, Audio: EngineFFmpeg}},
		{"audio re-encoded, rendition pipeline wired: hybrid master", deliveryResolver(true, false, true),
			PlaybackRequest{Streams: deliveryStreams("dts")}, copyVideo, DeliveryStrategy{Video: EngineMkvgo, Audio: EngineFFmpeg}},
		{"audio re-encoded, no rendition pipeline: the whole presentation falls to ffmpeg", deliveryResolver(true, false, false),
			PlaybackRequest{Streams: deliveryStreams("dts")}, copyVideo, DeliveryStrategy{Video: EngineFFmpeg, Audio: EngineFFmpeg}},
		{"wasm audio on a copyable video: packager video, client audio", deliveryResolver(true, false, true),
			PlaybackRequest{Streams: deliveryStreams("ac3"), WasmAudio: true, Prefs: DefaultTranscodingPreferences()}, copyBoth, DeliveryStrategy{Video: EngineMkvgo, Audio: EngineClient}},
		{"wasm audio on a transcoded video: ffmpeg video, client audio", deliveryResolver(true, false, true),
			PlaybackRequest{Streams: deliveryStreams("eac3"), WasmAudio: true, Prefs: DefaultTranscodingPreferences()}, transcoding.PlaybackDecision{VideoDecision: "transcode", VideoCodec: "h264", AudioDecision: "copy"},
			DeliveryStrategy{Video: EngineFFmpeg, Audio: EngineClient}},
		{"wasm asked on an AAC track: nothing for the decoder, the ordinary paths", deliveryResolver(true, false, true),
			PlaybackRequest{Streams: deliveryStreams("aac"), WasmAudio: true, Prefs: DefaultTranscodingPreferences()}, copyBoth, DeliveryStrategy{Video: EngineMkvgo, Audio: EngineMkvgo}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			route := ResolvedRoute{Copy: tc.decision.VideoDecision == "copy"}
			assert.Equal(t, tc.want, tc.pr.resolveDelivery(ctx, tc.req, tc.decision, route))
		})
	}
}

// Loudnorm re-encodes every track: the audio leaves the packager (hybrid
// master), trimmed or not.
func TestResolveDelivery_LoudnormLeavesThePackagerAudio(t *testing.T) {
	pr := deliveryResolver(true, false, true)
	pr.AudioNormalize = func(context.Context) bool { return true }
	got := pr.resolveDelivery(context.Background(), PlaybackRequest{Streams: deliveryStreams("aac")}, copyBoth, ResolvedRoute{Copy: true})
	assert.Equal(t, DeliveryStrategy{Video: EngineMkvgo, Audio: EngineFFmpeg}, got)
}

// The admin's serving path is a projection of the strategy, the route and
// the fingerprint - the values the adapter used to build by hand per site.
func TestDeliveryStrategy_ServingPath(t *testing.T) {
	cases := []struct {
		name     string
		delivery DeliveryStrategy
		route    ResolvedRoute
		fp       DecisionFingerprint
		want     domainplayback.ServingPath
	}{
		{"packager presentation", DeliveryStrategy{Video: EngineMkvgo, Audio: EngineMkvgo}, ResolvedRoute{Copy: true}, DecisionFingerprint{},
			domainplayback.ServingPath{VideoDecision: "copy", VideoEngine: "mkvgo", AudioDecision: "copy", AudioEngine: "mkvgo"}},
		{"hybrid", DeliveryStrategy{Video: EngineMkvgo, Audio: EngineFFmpeg}, ResolvedRoute{Copy: true}, DecisionFingerprint{},
			domainplayback.ServingPath{VideoDecision: "copy", VideoEngine: "mkvgo", AudioDecision: "transcode", AudioEngine: "ffmpeg"}},
		{"ffmpeg copy with copied audio", DeliveryStrategy{Video: EngineFFmpeg, Audio: EngineFFmpeg}, ResolvedRoute{Copy: true}, DecisionFingerprint{AudioCopy: true},
			domainplayback.ServingPath{VideoDecision: "copy", VideoEngine: "ffmpeg", AudioDecision: "copy", AudioEngine: "ffmpeg"}},
		{"GPU transcode", DeliveryStrategy{Video: EngineFFmpeg, Audio: EngineFFmpeg}, ResolvedRoute{Backend: BackendSelection{Name: "qsv"}}, DecisionFingerprint{},
			domainplayback.ServingPath{VideoDecision: "transcode", VideoEngine: "ffmpeg", VideoBackend: "qsv", AudioDecision: "transcode", AudioEngine: "ffmpeg"}},
		{"software transcode", DeliveryStrategy{Video: EngineFFmpeg, Audio: EngineFFmpeg}, ResolvedRoute{}, DecisionFingerprint{},
			domainplayback.ServingPath{VideoDecision: "transcode", VideoEngine: "ffmpeg", VideoBackend: "software", AudioDecision: "transcode", AudioEngine: "ffmpeg"}},
		{"wasm audio", DeliveryStrategy{Video: EngineMkvgo, Audio: EngineClient}, ResolvedRoute{Copy: true}, DecisionFingerprint{},
			domainplayback.ServingPath{VideoDecision: "copy", VideoEngine: "mkvgo", AudioDecision: "wasm", AudioEngine: "client"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.delivery.ServingPath(tc.route, tc.fp))
		})
	}
}

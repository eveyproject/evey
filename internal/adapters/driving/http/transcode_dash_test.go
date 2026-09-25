package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/client_golang/prometheus/testutil"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domainplayback "github.com/evey-project/evey/internal/domain/playback"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/metrics"
)

// withAudioRenditions wires the separate audio renditions DASH needs (a
// muxed presentation is not offered): the flag and the probes the transcode
// path requires of the host.
func withAudioRenditions(h *hlsVODServe) {
	h.audioRenditionEnabled = true
	h.frameRateProbe = func(context.Context, string) float64 { return 25 }
	h.encoderReorderProbe = func(context.Context, string, string, string, []string) (int, bool) { return 2, true }
}

// manifest.mpd on the ffmpeg path is declared from the plan: the transcode
// ladder's timeline in the grid's ticks, over the same fragments and with
// the same auth suffix the HLS playlists address, before any fragment
// exists. The packager path keeps serving its own manifest.
func TestServeManifestMPD_FFmpegPathDeclaresThePlan(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur}}
	streams := copyEligibleAV()
	fps := 25.0
	streams[0].Framerate = &fps
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})
	withAudioRenditions(h)

	// A pinned transcode: the plan's video engine is ffmpeg.
	w := doVODRequestQ(h, "m1", "manifest.mpd", "videoCodecs=h264&audioCodecs=aac&containers=mp4&vd=transcode")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "application/dash+xml", w.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
	body := w.Body.String()
	assert.True(t, strings.HasPrefix(body, `<?xml version="1.0" encoding="UTF-8"?>`+"\n<MPD "), body[:80])
	assert.Contains(t, body, `timescale="12800"`, "25 fps: 512 ticks a frame")
	assert.Contains(t, body, `<S t="0" d="38400" r="18"/>`, "19 full segments of 75 frames")
	assert.Contains(t, body, `<S d="38400"/>`, "and a full last one: 60 s is 20 segments")
	assert.Contains(t, body, `media="stream_0_$Number%03d$.m4s?`)
	assert.Contains(t, body, `.m4s?ad=copy&amp;audioCodecs=aac&amp;audioTrack=0&amp;`, "the same query suffix as the HLS playlists, XML-escaped")
	assert.Contains(t, body, `vd=transcode`, "the decision inputs ride every URI")

	// The HLS master of the same plan lists the same segment count.
	m := doVODRequestQ(h, "m1", "stream_0.m3u8", "videoCodecs=h264&audioCodecs=aac&containers=mp4&vd=transcode")
	require.Equal(t, http.StatusOK, m.Code)
	assert.Equal(t, 20, strings.Count(m.Body.String(), "#EXTINF:"))
}

// A source on the legacy clock grid (no frame rate known) has no exact
// timeline: DASH is not offered, HLS is unchanged.
func TestServeManifestMPD_LegacyGridIsNotOffered(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur}}
	streams := copyEligibleAV() // no Framerate
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})
	w := doVODRequestQ(h, "m1", "manifest.mpd", "videoCodecs=h264&audioCodecs=aac&containers=mp4&vd=transcode")
	assert.Equal(t, http.StatusNotFound, w.Code)
	m := doVODRequestQ(h, "m1", "master.m3u8", "videoCodecs=h264&audioCodecs=aac&containers=mp4&vd=transcode")
	assert.Equal(t, http.StatusOK, m.Code)
}

// When the plan's video engine is the packager, manifest.mpd is still the
// packager's manifest - byte for byte what it was.
func TestServeManifestMPD_PackagerPathUnchanged(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := onDemandItem()
	item.ID, item.LibraryID = "m1", "lib1"
	item.File.Duration = dur
	streams := copyEligibleAV()
	fps := 25.0
	streams[0].Framerate = &fps
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})
	h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{dash: true, resources: map[string][]byte{
		"master.m3u8":  []byte("#EXTM3U\n"),
		"manifest.mpd": []byte("<MPD packager=\"mkvgo\"/>"),
	}}}
	w := doVODRequestQ(h, "m1", "manifest.mpd", "videoCodecs=h264,hevc&audioCodecs=aac&containers=mp4")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `packager="mkvgo"`)
}

// A source the packager cannot build (an MPEG-TS: mkvgo reads no transport
// stream) is served by the ffmpeg path for the same plan, as its HLS master
// is - DASH alone used to answer 404 there.
func TestServeManifestMPD_UnbuildablePackagerPlanFallsBackToFFmpeg(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := onDemandItem()
	item.ID, item.LibraryID = "m1", "lib1"
	item.File.Duration = dur
	streams := copyEligibleAV()
	fps := 25.0
	streams[0].Framerate = &fps
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})
	withAudioRenditions(h)
	h.onDemand = &fakeOnDemandPackager{err: errors.New("mp4: no moov box found")}
	w := doVODRequestQ(h, "m1", "manifest.mpd", "videoCodecs=h264,hevc&audioCodecs=aac&containers=mp4")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "application/dash+xml", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), `media="stream_0_$Number%03d$.m4s?`, "the ffmpeg path's fragments")
}

// The same transcode with its audio muxed (no separate renditions wired) is
// not offered as DASH: HLS keeps serving it.
func TestServeManifestMPD_MuxedAudioIsNotOffered(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur}}
	streams := copyEligibleAV()
	fps := 25.0
	streams[0].Framerate = &fps
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})
	w := doVODRequestQ(h, "m1", "manifest.mpd", "videoCodecs=h264&audioCodecs=aac&containers=mp4&vd=transcode")
	assert.Equal(t, http.StatusNotFound, w.Code)
	m := doVODRequestQ(h, "m1", "master.m3u8", "videoCodecs=h264&audioCodecs=aac&containers=mp4&vd=transcode")
	assert.Equal(t, http.StatusOK, m.Code)
}

// A copied HE-AAC rendition is announced by its object type; the same track
// re-encoded is AAC-LC, whatever the source was.
func TestServeManifestMPD_CopiedHEAACAnnouncedAsSuch(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur}}
	streams := copyEligibleAV()
	fps := 25.0
	streams[0].Framerate = &fps
	streams[1].Codec, streams[1].Profile = "aac", "HE-AAC"
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})
	withAudioRenditions(h)

	copied := doVODRequestQ(h, "m1", "manifest.mpd", "videoCodecs=h264&audioCodecs=aac&containers=mp4&vd=transcode")
	require.Equal(t, http.StatusOK, copied.Code, copied.Body.String())
	assert.Contains(t, copied.Body.String(), `codecs="mp4a.40.5"`)

	reencoded := doVODRequestQ(h, "m1", "manifest.mpd", "videoCodecs=h264&audioCodecs=aac&containers=mp4&vd=transcode&audioOffsetMs=200")
	require.Equal(t, http.StatusOK, reencoded.Code, reencoded.Body.String())
	assert.Contains(t, reencoded.Body.String(), `codecs="mp4a.40.2"`)
	assert.NotContains(t, reencoded.Body.String(), `mp4a.40.5`)
}

// manifest.mpd is observed like the HLS master: the path it served is
// recorded on the play session and held against the plan, the fallback of a
// source the packager cannot build is counted; the packager's manifest is
// served from the one plan the request resolved (built once).
func TestServeManifestMPD_RecordsWhatItServed(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := onDemandItem()
	item.ID, item.LibraryID = "m1", "lib1"
	item.File.Duration = dur
	streams := copyEligibleAV()
	fps := 25.0
	streams[0].Framerate = &fps
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})
	withAudioRenditions(h)
	var recorded []domainplayback.ServingPath
	h.servingRecord = func(_ context.Context, _, _ string, p domainplayback.ServingPath) { recorded = append(recorded, p) }

	// The packager's presentation: built once, recorded as the packager's.
	pkg := &fakeOnDemandPackager{plan: &fakeOnDemandPlan{dash: true, resources: map[string][]byte{"manifest.mpd": []byte("<MPD/>")}}}
	h.onDemand = pkg
	w := doVODRequestQ(h, "m1", "manifest.mpd", "videoCodecs=h264,hevc&audioCodecs=aac&containers=mp4")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 1, pkg.calls, "one plan, built once")
	require.Len(t, recorded, 1)
	assert.Equal(t, "mkvgo", recorded[0].VideoEngine)
	assert.Equal(t, "mkvgo", recorded[0].AudioEngine)

	// Unbuildable: the ffmpeg path serves it, the fallback is counted.
	fallbacks := func() int {
		return int(testutil.ToFloat64(metrics.HLSVODDeliveryFallbacks.WithLabelValues(fallbackPackagerUnbuildable)))
	}
	before := fallbacks()
	h.onDemand = &fakeOnDemandPackager{err: errors.New("mp4: no moov box found")}
	w = doVODRequestQ(h, "m1", "manifest.mpd", "videoCodecs=h264,hevc&audioCodecs=aac&containers=mp4")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "ffmpeg", w.Header().Get("X-Evey-Served-By"))
	require.Len(t, recorded, 2)
	assert.Equal(t, "ffmpeg", recorded[1].VideoEngine)
	assert.Equal(t, before+1, fallbacks())
}

// A hybrid plan (the packager's video, Evey's AAC rendition for a track the
// packager cannot carry) is offered as DASH: the packager's video set beside
// Evey's audio, the source's own audio dropped. A packager manifest with no
// video to stand beside is not offered.
func TestServeManifestMPD_HybridPlan(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := onDemandItem()
	item.ID, item.LibraryID = "m1", "lib1"
	item.File.Duration = dur
	streams := copyEligibleAV()
	fps := 25.0
	streams[0].Framerate = &fps
	streams[1].Codec = "dts"
	packager := `<?xml version="1.0" encoding="UTF-8"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" mediaPresentationDuration="PT60.000S">
  <Period>
    <AdaptationSet mimeType="video/mp4" contentType="video">
      <Representation id="v" bandwidth="8000000" width="1920" height="1080" codecs="avc1.640028">
        <SegmentTemplate initialization="od_init.mp4" media="od_seg$Number%05d$.m4s" startNumber="1" timescale="1000"/>
      </Representation>
    </AdaptationSet>
    <AdaptationSet mimeType="audio/mp4" contentType="audio" lang="eng">
      <Representation id="a1" bandwidth="0" codecs="dtsc"><SegmentTemplate media="od_seg_a1_$Number%05d$.m4s"/></Representation>
    </AdaptationSet>
  </Period>
</MPD>
`
	serve := func(mpd string) *httptest.ResponseRecorder {
		h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})
		withAudioRenditions(h)
		h.audio = &audioServe{}
		h.onDemand = &fakeOnDemandPackager{plan: &fakeOnDemandPlan{dash: true, resources: map[string][]byte{"manifest.mpd": []byte(mpd)}}}
		return doVODRequestQ(h, "m1", "manifest.mpd", "videoCodecs=h264,hevc&audioCodecs=aac&containers=mp4")
	}

	w := serve(packager)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := w.Body.String()
	assert.Equal(t, "application/dash+xml", w.Header().Get("Content-Type"))
	assert.Contains(t, body, `media="od_seg$Number%05d$.m4s?v=`, "the packager's video, its URIs suffixed as it serves them")
	assert.NotContains(t, body, "od_seg_a1_", "the source's DTS is not what the plan serves")
	assert.Contains(t, body, `media="audio_0_$Number%03d$.m4s?`, "Evey's rendition, as the hybrid HLS master names it")
	assert.Contains(t, body, `codecs="mp4a.40.2"`)

	w = serve("<MPD/>")
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

// An AAC whose profile nobody read is not declared an object type it may not
// have: the rendition is re-encoded to AAC-LC (48 kHz), where a known LC is
// copied at its own 44.1 kHz. The muxed plan follows the same rule.
func TestServeManifestMPD_UnknownAACProfileIsReencoded(t *testing.T) {
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", File: &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur}}
	rate := 44100
	for profile, copied := range map[string]bool{"": false, "LC": true} {
		streams := copyEligibleAV()
		fps := 25.0
		streams[0].Framerate = &fps
		streams[1].Codec, streams[1].Profile, streams[1].SampleRate = "aac", profile, &rate
		h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})
		withAudioRenditions(h)
		w := doVODRequestQ(h, "m1", "manifest.mpd", "videoCodecs=h264&audioCodecs=aac&containers=mp4&vd=transcode")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		want := `codecs="mp4a.40.2" audioSamplingRate="48000"`
		if copied {
			want = `codecs="mp4a.40.2" audioSamplingRate="44100"`
		}
		assert.Contains(t, w.Body.String(), want, "profile %q", profile)
	}

	// The muxed plan (no separate renditions) copies a known AAC only.
	for profile, copied := range map[string]bool{"": false, "LC": true} {
		streams := copyEligibleAV()
		streams[1].Codec, streams[1].Profile = "aac", profile
		h := newCopyTestHandler(t)
		rp := planFor(t, h, onDemandReq(), onDemandItem(), streams)
		assert.Equal(t, copied, rp.FP.AudioCopy, "profile %q", profile)
	}
}

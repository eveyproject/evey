package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
)

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

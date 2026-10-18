package httpapi

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// cellularCapHandler is a copy-capable handler whose profile caps cellular
// playback at capKbps, over a 10 Mb/s H.264 source the client decodes as is.
func cellularCapHandler(t *testing.T, capKbps string) (*hlsVODServe, *domainlibrary.MediaItem, []*domainlibrary.MediaStream) {
	t.Helper()
	dur, _ := shared.NewDurationMs(60_000)
	item := &domainlibrary.MediaItem{
		ID:        "m1",
		LibraryID: "lib1",
		File:      &domainlibrary.MediaFile{Path: "/test.mkv", Duration: dur, ContainerFormat: "matroska"},
	}
	streams := copyEligibleStreams()
	bps := int64(10_000_000)
	streams[0].Bitrate = &bps
	h, _ := buildVODHandler(t, &testFakeStreamsGetter{item: item, streams: streams}, allowAll(), testFakeParentalChecker{})
	copyProd := apptranscoding.NewSoftwareProducer(h.cache, nil, nil, apptranscoding.SoftwareProducerOptions{}, nil)
	copyProd.SetCopyMode(apptranscoding.NewCopyPlanner(nil, nil), nil)
	h.copyProducer = copyProd
	h.profilePrefs = &fakeProfilePrefsGetter{prefs: map[string]string{"max_bitrate_cellular_kbps": capKbps}}
	return h, item, streams
}

// The profile's cellular cap reaches the HLS master. GetStreamInfo applied it
// (network_type), but the master the client then opened never learnt the
// network: it re-decided as on Wi-Fi and served the copy rung at the source
// bitrate to a phone on mobile data. ?networkType=cellular now folds the cap
// into the master's decision exactly as it does into the stream info, so the
// copy is refused (bitrate_exceeded) and every transcoded rung stays under it.
// The symmetric case pins the premise: the same request on Wi-Fi still copies.
func TestResolvePlan_CellularCapRefusesTheCopyRung(t *testing.T) {
	h, item, streams := cellularCapHandler(t, "2000")

	wifi := withHTTPClaims(httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/master.m3u8?networkType=wifi", nil), testClaims(false))
	rp, err := h.resolvePlan(wifi, "m1", item, streams)
	require.NoError(t, err)
	require.True(t, rp.IsCopy, "premise: on Wi-Fi the 10 Mb/s source is copied, reasons %v", rp.Decision.TranscodeReasons)

	cell := withHTTPClaims(httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/master.m3u8?networkType=cellular", nil), testClaims(false))
	rp, err = h.resolvePlan(cell, "m1", item, streams)
	require.NoError(t, err)
	assert.False(t, rp.IsCopy, "a cellular client must not get the source bitrate over the copy rung")
	assert.True(t, slices.Contains(rp.Decision.TranscodeReasons, "bitrate_exceeded"), "reasons %v", rp.Decision.TranscodeReasons)
	for _, p := range rp.Profiles {
		assert.LessOrEqualf(t, p.VideoBitrate, int64(2000), "rung %s above the cellular cap", p.Name)
	}
}

// The network is a decision input, so it rides every child URI: a segment
// re-derives the master's decision from its own query, and a segment that
// lost it would decide a copy under a master that advertised a capped ladder.
func TestChildURIsCarryTheNetworkType(t *testing.T) {
	master := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/master.m3u8?networkType=cellular", nil)
	in := apptranscoding.ManifestInput{MediaID: "m1", StreamToken: "tok", SubtitleBurnInTrack: -1}
	bakeClientInputs(master, &in)
	suffix := apptranscoding.QuerySuffixFor(in)
	q, err := url.ParseQuery(strings.TrimPrefix(suffix, "?"))
	require.NoError(t, err)
	assert.Equal(t, "cellular", q.Get("networkType"))

	// Wi-Fi is the default: it adds nothing, so every existing URI is unchanged.
	wifi := httptest.NewRequest(http.MethodGet, "/api/v1/media/m1/hls/master.m3u8?networkType=wifi", nil)
	in = apptranscoding.ManifestInput{MediaID: "m1", StreamToken: "tok", SubtitleBurnInTrack: -1}
	bakeClientInputs(wifi, &in)
	assert.NotContains(t, apptranscoding.QuerySuffixFor(in), "networkType")
}

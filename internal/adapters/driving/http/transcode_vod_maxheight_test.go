package httpapi

import (
	"net/http"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

// ?maxHeight= is what the mobile/TV quality menu and the cast receiver's
// device profile send: the master must stop its ladder there and bake the
// value into every child URL. It was parsed by GetStreamInfo only, so the
// quality menu changed the stream-info verdict and nothing in the stream.
func TestHLSVOD_Master_HonoursMaxHeight(t *testing.T) {
	w, hgt := 3840, 2160
	v := &domainlibrary.MediaStream{StreamType: domainlibrary.StreamTypeVideo, Codec: "hevc", Width: &w, Height: &hgt, Profile: "Main"}
	h, _ := buildVODHandler(t,
		&testFakeStreamsGetter{item: testMediaItemWith1Minute(), streams: []*domainlibrary.MediaStream{v}},
		allowAll(), testFakeParentalChecker{block: false})
	h.capability = func() domaintranscoding.EncodeCapability { return domaintranscoding.EncodeCapability{SoftwareMaxHeight: 2160} }

	rec := doVODRequestQ(h, "m1", "master.m3u8", "videoCodecs=h264&maxHeight=480")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	heights := regexp.MustCompile(`RESOLUTION=\d+x(\d+)`).FindAllStringSubmatch(body, -1)
	require.NotEmpty(t, heights)
	for _, m := range heights {
		n, _ := strconv.Atoi(m[1])
		assert.LessOrEqual(t, n, 480, "rung above the declared ceiling: %s", m[0])
	}
	assert.Contains(t, body, "maxHeight=480", "the ceiling must ride the child URLs")

	unbounded := doVODRequestQ(h, "m1", "master.m3u8", "videoCodecs=h264").Body.String()
	assert.Contains(t, unbounded, "RESOLUTION=1920x1080", "without the ceiling the ladder keeps its taller rungs")
}

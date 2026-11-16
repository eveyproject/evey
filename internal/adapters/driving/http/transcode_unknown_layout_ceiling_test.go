package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainuser "github.com/eveyproject/evey/internal/domain/user"
)

// A track whose channel count the prober never measured, on a profile capped
// in channels: the master resolves the decision from the request as the
// client sent it, its variant URIs carry the ceiling folded with the profile
// cap, and the variant must resolve the fingerprint the master sealed.
func TestUnknownLayout_ProfileCap_VariantResolvesTheSealedFingerprint(t *testing.T) {
	six := 6
	cases := map[string]struct {
		clientCeiling string
		channels      *int
	}{
		"client declares no ceiling":         {"", nil},
		"client ceiling above the profile's": {"6", nil},
		"control: measured 5.1 layout":       {"", &six},
	}
	for name, tc := range cases {
		clientCeiling := tc.clientCeiling
		t.Run(name, func(t *testing.T) {
			h := lineHandler(t)
			h.profilePrefs = &fakeProfilePrefsGetter{prefs: map[string]string{"max_audio_channels": "2"}}
			item := lineItem(t, 4_000_000)
			streams := copyEligibleAV()
			streams[1].Channels = tc.channels

			query := "videoCodecs=h264&audioCodecs=aac&containers=mp4&maxHeight=1080"
			if clientCeiling != "" {
				query += "&maxAudioChannels=" + clientCeiling
			}
			req := profileReq("/api/v1/media/m1/hls/master.m3u8?" + query)
			r, rp := h.planAsServed(req, "m1", item, streams, planFor(t, h, req, item, streams))
			if tc.channels == nil {
				require.Contains(t, rp.Decision.TranscodeReasons, "audio_layout_unknown", "premise: the unmeasured layout is re-encoded")
			}

			var variant string
			for line := range strings.SplitSeq(masterOf(t, h, r, item, streams, rp), "\n") {
				if strings.HasPrefix(line, "stream_0.m3u8") {
					variant = line
				}
			}
			require.NotEmpty(t, variant)
			require.Contains(t, variant, "fp="+rp.FPHash)

			vp := planFor(t, h, profileReq("/api/v1/media/m1/hls/"+variant), item, streams)
			assert.Equal(t, rp.FPHash, vp.FPHash, "the variant must resolve to the sealed fingerprint, else 410 stale_rendition")
			assert.LessOrEqual(t, rp.Decision.MaxAudioChannels, 2, "the profile caps the served layout at stereo")
		})
	}
}

func profileReq(target string) *http.Request {
	req := httptest.NewRequest("GET", target, nil)
	return req.WithContext(context.WithValue(req.Context(), httpClaimsCtxKey{},
		&domainuser.Claims{UserID: "u1", ProfileID: "p1"}))
}

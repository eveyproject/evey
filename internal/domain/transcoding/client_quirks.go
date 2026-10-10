package transcoding

import (
	"regexp"
	"strconv"
	"strings"
)

// silentEAC3Builds matches the app builds whose own E-AC-3 decoder (ac3go in
// the player) hands a two-channel output pure silence: its stereo downmix of
// E-AC-3 was never computed (ac3go before 0756b36). Mobile 0.0.1-beta.68/69
// and TV 0.1.9/0.1.10 declare eac3 whatever the device decodes, so a 5.1
// E-AC-3 track was direct-played or copied to a phone that played the
// picture without a sound (2026-09-27). The API calls carry EveyMobile/ or
// EveyTV/, the player's segment and file requests Evey/ with the same version.
var silentEAC3Builds = regexp.MustCompile(`\bEvey(?:Mobile|TV)?/(?:0\.0\.1-beta\.(?:68|69)|0\.1\.(?:9|10))\b`)

// mobileBuild captures the beta number of a phone/tablet build's user agent
// (EveyMobile/ on the API calls, Evey/ on the player's own requests; the TV
// app's versions are 0.1.x and never match).
var mobileBuild = regexp.MustCompile(`\bEvey(?:Mobile)?/0\.0\.1-beta\.(\d+)\b`)

// firstMobileBuildWithSinkGatedDTS is the mobile build (f5a080f9a) from which
// a phone declares dts/truehd only on a plugged-in HDMI sink's say-so. Before
// it, the app declared them on isDirectPlaybackSupported, a signal Media3
// reads on a TV only: a phone with no DTS decoder was served a DTS-only film
// in direct play and played it silent (2026-10-06) or died on
// MediaCodecAudioRenderer with a DTS-HD MA track (2026-10-10). Those builds
// stay installed until the Play Store gets round to them.
const firstMobileBuildWithSinkGatedDTS = 76

// WithoutBrokenPlayerCodecs removes from a client's declared audio codecs the
// ones its build is known to render as silence, so the server transcodes them
// instead. Every reading of the declaration (stream info, master, rendition
// fingerprint, audio playlist) goes through it, so they all agree on the
// decision. A no-op for every other client; the slice is not modified.
func WithoutBrokenPlayerCodecs(userAgent string, codecs []string) []string {
	drop := map[string]bool{}
	if silentEAC3Builds.MatchString(userAgent) {
		drop["eac3"] = true
	}
	if m := mobileBuild.FindStringSubmatch(userAgent); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n < firstMobileBuildWithSinkGatedDTS {
			drop["dts"] = true
			drop["truehd"] = true
		}
	}
	if len(drop) == 0 {
		return codecs
	}
	out := make([]string, 0, len(codecs))
	for _, c := range codecs {
		if drop[strings.ToLower(strings.TrimSpace(c))] {
			continue
		}
		out = append(out, c)
	}
	return out
}

package transcoding

import (
	"regexp"
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

// WithoutBrokenPlayerCodecs removes from a client's declared audio codecs the
// ones its build is known to render as silence, so the server transcodes them
// instead. Every reading of the declaration (stream info, master, rendition
// fingerprint, audio playlist) goes through it, so they all agree on the
// decision. A no-op for every other client; the slice is not modified.
func WithoutBrokenPlayerCodecs(userAgent string, codecs []string) []string {
	if !silentEAC3Builds.MatchString(userAgent) {
		return codecs
	}
	out := make([]string, 0, len(codecs))
	for _, c := range codecs {
		if strings.EqualFold(strings.TrimSpace(c), "eac3") {
			continue
		}
		out = append(out, c)
	}
	return out
}

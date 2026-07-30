package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// decision_test.go already covers the ordinary arbitration of the two bitrate
// caps (wifi vs cellular, cellular-only, larger-cellular-ignored). What it does
// NOT cover is the exact boundary between them and the parsing of the stored
// values - both flagged by mutation testing, the parsing as entirely
// unexecuted. That is what lives here.

// ⚠️ The cap arbitration itself is NOT retested here. decision.go:151
// (`MaxBitrateCellularKbps < MaxBitrateKbps`) shows up as a surviving boundary
// mutant, but it is EQUIVALENT: at equality both branches return the same
// number, so no test can distinguish `<` from `<=`. The non-boundary cases are
// already covered in decision_test.go. Chasing that mutant is wasted effort -
// see the equivalent-mutant catalogue in the memory backlog.

// ParseTranscodingPreferences reads a profile's stored key/value strings, so
// every value is untrusted text. A non-positive number must leave the cap UNSET
// rather than be stored literally: a MaxBitrateKbps of -1 or 0 reads as "no
// cap" downstream, so a profile that tried to set a floor would silently get
// unlimited bitrate instead.
//
// These lines were entirely UNEXECUTED before this test (gremlins: NOT COVERED
// at decision.go:189 and :194). The `n > 0` guard itself is another equivalent
// mutant - the rejected value and the field's zero value are both 0 - but the
// garbage cases below are real, and an executed line is one a future
// non-equivalent mutant can be caught on.
func TestParseTranscodingPreferences_BitrateCapsRejectNonPositiveAndGarbage(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{name: "a plain number", in: "4500", want: 4500},
		{name: "surrounding whitespace", in: "  4500  ", want: 4500},
		{name: "zero is not a cap", in: "0"},
		{name: "negative is rejected", in: "-1"},
		{name: "not a number", in: "fast"},
		{name: "empty", in: ""},
		{name: "a float is not an int", in: "4500.5"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseTranscodingPreferences(map[string]string{
				"max_bitrate_kbps":          tc.in,
				"max_bitrate_cellular_kbps": tc.in,
			})
			assert.Equal(t, tc.want, got.MaxBitrateKbps,
				"a rejected value must leave the cap unset, never store the parsed number")
			assert.Equal(t, tc.want, got.MaxBitrateCellularKbps)
		})
	}
}

// The two caps are independent keys: setting one must not move the other, or a
// user capping only their mobile data would find their wifi capped too.
func TestParseTranscodingPreferences_BitrateCapsAreIndependent(t *testing.T) {
	onlyCellular := ParseTranscodingPreferences(map[string]string{"max_bitrate_cellular_kbps": "1500"})
	assert.Equal(t, 1500, onlyCellular.MaxBitrateCellularKbps)
	assert.Zero(t, onlyCellular.MaxBitrateKbps, "the general cap must stay unset")

	onlyGeneral := ParseTranscodingPreferences(map[string]string{"max_bitrate_kbps": "8000"})
	assert.Equal(t, 8000, onlyGeneral.MaxBitrateKbps)
	assert.Zero(t, onlyGeneral.MaxBitrateCellularKbps, "the cellular cap must stay unset")
}

// A nil map is the common case - a profile that never customised anything - and
// must yield exactly the client defaults. SubtitleBurnInTrack in particular:
// the zero value 0 means "the first subtitle track is selected", which would
// re-introduce burn-on-existence for every un-customised profile.
func TestParseTranscodingPreferences_NilYieldsTheDefaultsIncludingTheTrackSentinels(t *testing.T) {
	defaults := DefaultTranscodingPreferences()
	assert.Equal(t, defaults, ParseTranscodingPreferences(nil))

	assert.Equal(t, -1, defaults.SubtitleBurnInTrack,
		"0 would mean 'burn the first subtitle track' on every un-customised profile")
	assert.Equal(t, -1, defaults.AudioTrack,
		"0 would pin the first audio track instead of letting the decision choose")
}

package cmcd

import "testing"

// CMCD arrives in a header or query string the player controls, and it is
// parsed on every segment request - the hottest untrusted-input path in the
// server. Parse() does its own comma splitting, quote tracking and key/value
// slicing by index, which is the shape of code that walks off the end of a
// string when a quote is left open or a separator lands in the wrong place.
//
// Nothing here asserts what the payload should contain: on malformed input
// there is no right answer. The contract under test is that a client cannot
// make the parser panic by sending a deliberately awkward header.
func FuzzParse(f *testing.F) {
	f.Add("")
	f.Add("br=1200,bl=15000,d=4000")
	f.Add(`sid="some-session",cid="some-content"`)
	f.Add(`ot=v,su,rtp=15000`)
	// The awkward ones: unbalanced quote, trailing separator, empty key, a
	// separator inside a quoted value, and a numeric field that is not numeric.
	f.Add(`sid="unterminated`)
	f.Add(",,,")
	f.Add("=")
	f.Add(`sid="a,b,c",br=1`)
	f.Add("br=not-a-number")
	f.Add(`"`)

	f.Fuzz(func(t *testing.T, s string) {
		// Returning at all is the assertion.
		_ = Parse(s)
	})
}

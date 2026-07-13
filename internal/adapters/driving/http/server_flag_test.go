package httpapi

import "testing"

// Operator flags used to be compared against the literal strings "on"/"off",
// while the admin panel persists every other boolean with strconv.FormatBool
// ("true"/"false"). The two never met:
//   - transcoding.hls_ondemand = "false"  did NOT disable it ("false" != "off")
//   - transcoding.hls_ondemand_concat = "true" did NOT enable it ("true" != "on")
//
// An operator could believe a path was off while it ran, or on while it was
// ignored. Accept the usual spellings, and keep the unset value on the default.
func TestFlagEnabled(t *testing.T) {
	for _, tc := range []struct {
		raw       string
		defaultOn bool
		want      bool
	}{
		// unset → the code's default
		{"", true, true},
		{"", false, false},
		// the panel's own format (strconv.FormatBool) must work
		{"true", false, true},
		{"false", true, false},
		// the historical spelling must keep working
		{"on", false, true},
		{"off", true, false},
		// common operator spellings
		{"1", false, true},
		{"0", true, false},
		{"yes", false, true},
		{"no", true, false},
		{" TRUE ", false, true}, // whitespace + case
		{"Off", true, false},
		// garbage must not silently flip a path: fall back to the default
		{"maybe", true, true},
		{"maybe", false, false},
	} {
		if got := flagEnabled(tc.raw, tc.defaultOn); got != tc.want {
			t.Errorf("flagEnabled(%q, default=%v) = %v, want %v", tc.raw, tc.defaultOn, got, tc.want)
		}
	}
}

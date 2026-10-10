package transcoding

import (
	"slices"
	"testing"
)

// A phone build before beta.76 declared dts/truehd on a signal Media3 reads
// on a TV only (2026-10-06 silent film, 2026-10-10 fatal renderer error on a
// DTS-HD MA track): the server drops them so those installs get the track
// transcoded until the Play Store updates them. From beta.76 the app only
// declares them on a plugged-in HDMI sink's say-so, and is believed.
func TestWithoutBrokenPlayerCodecs_PhoneDTSBeforeSinkGate(t *testing.T) {
	declared := []string{"aac", "ac3", "eac3", "dts", "truehd", "opus"}
	withoutDTS := []string{"aac", "ac3", "eac3", "opus"}
	cases := []struct {
		ua   string
		want []string
	}{
		// The user agents of the 10-10 session: API calls, then the player's file requests.
		{"EveyMobile/0.0.1-beta.74 (Android 13; Phone)", withoutDTS},
		{"Evey/0.0.1-beta.74 (Linux;Android 13) AndroidXMedia3/1.8.0", withoutDTS},
		{"EveyMobile/0.0.1-beta.75 (Android 14; Tablet)", withoutDTS},
		{"EveyMobile/0.0.1-beta.9 (Android 14; Tablet)", withoutDTS},
		// The first build with the HDMI gate, and every later one.
		{"EveyMobile/0.0.1-beta.76 (Android 13; Phone)", declared},
		{"EveyMobile/0.0.1-beta.79 (Android 13; Phone)", declared},
		{"Evey/0.0.1-beta.100 (Linux;Android 13) AndroidXMedia3/1.8.0", declared},
		// A TV declares on direct playback, which Media3 honours there.
		{"EveyTV/0.1.21 (Android TV 14; Chromecast HD)", declared},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:156.0) Gecko/20100101 Firefox/156.0", declared},
		{"", declared},
	}
	for _, tc := range cases {
		got := WithoutBrokenPlayerCodecs(tc.ua, declared)
		if !slices.Equal(got, tc.want) {
			t.Errorf("%q: got %v, want %v", tc.ua, got, tc.want)
		}
	}
	if !slices.Equal(declared, []string{"aac", "ac3", "eac3", "dts", "truehd", "opus"}) {
		t.Fatalf("the declared slice was modified: %v", declared)
	}

	// Both quirks at once: a beta.69 phone loses eac3 AND dts/truehd. The
	// codec names are matched whatever their case or spacing.
	got := WithoutBrokenPlayerCodecs("EveyMobile/0.0.1-beta.69 (Android 16; Phone)", []string{"aac", " DTS", "eac3", "TrueHD"})
	if !slices.Equal(got, []string{"aac"}) {
		t.Fatalf("beta.69: got %v, want [aac]", got)
	}
}

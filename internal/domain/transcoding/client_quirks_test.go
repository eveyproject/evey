package transcoding

import (
	"slices"
	"testing"
)

func TestWithoutBrokenPlayerCodecs(t *testing.T) {
	declared := []string{"aac", "mp3", "ac3", "eac3", "opus"}
	withoutEAC3 := []string{"aac", "mp3", "ac3", "opus"}
	cases := []struct {
		ua   string
		want []string
	}{
		// The user agents seen in the prod logs of 2026-09-26/27.
		{"EveyMobile/0.0.1-beta.69 (Android 16; Phone Pro)", withoutEAC3},
		{"EveyMobile/0.0.1-beta.68 (Android 13; Phone)", withoutEAC3},
		{"Evey/0.0.1-beta.69 (Linux;Android 14) AndroidXMedia3/1.8.0", withoutEAC3},
		{"EveyTV/0.1.10 (Android TV 14; Chromecast HD)", withoutEAC3},
		{"EveyTV/0.1.9 (Android TV 12; SHIELD)", withoutEAC3},
		// Builds before the ac3go renderer and after its fix keep what they declare.
		{"EveyMobile/0.0.1-beta.67 (Android 16; Phone Pro)", declared},
		{"EveyMobile/0.0.1-beta.70 (Android 16; Phone Pro)", declared},
		{"Evey/0.0.1-beta.690 (Linux;Android 14) AndroidXMedia3/1.8.0", declared},
		{"EveyTV/0.1.8 (Android TV 14; Chromecast HD)", declared},
		{"EveyTV/0.1.11 (Android TV 14; Chromecast HD)", declared},
		{"EveyTV/0.1.100 (Android TV 14; Chromecast HD)", declared},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:156.0) Gecko/20100101 Firefox/156.0", declared},
		{"", declared},
	}
	for _, tc := range cases {
		got := WithoutBrokenPlayerCodecs(tc.ua, declared)
		if !slices.Equal(got, tc.want) {
			t.Errorf("%q: got %v, want %v", tc.ua, got, tc.want)
		}
	}
	if !slices.Equal(declared, []string{"aac", "mp3", "ac3", "eac3", "opus"}) {
		t.Fatalf("the declared slice was modified: %v", declared)
	}
}

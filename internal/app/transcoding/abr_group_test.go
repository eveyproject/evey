package transcoding

import "testing"

func TestABRGroup_SameAs(t *testing.T) {
	cases := []struct {
		name           string
		aCodec, aRange string
		bCodec, bRange string
		want           bool
	}{
		// The good cases the fallback rung is meant to keep: same family, same
		// range -> the player may auto-switch (ABR drop) with no decoder swap.
		{"h264 SDR == h264 SDR (normal ABR drop)", "h264", "", "h264", "", true},
		{"hevc SDR == hevc SDR (normal ABR drop)", "hevc", "", "hevc", "", true},
		{"codec spellings fold (avc1/libx264)", "avc1", "", "libx264", "", true},
		{"hevc HDR PQ == hevc HDR PQ", "hevc", "PQ", "hevc", "PQ", true},

		// The freeze cases: a decoder swap on an auto-drop. Never the same group.
		{"h264 copy vs hevc fallback (family)", "h264", "", "hevc", "", false},
		{"av1 copy vs h264 fallback (family)", "av1", "", "h264", "", false},
		{"hevc HDR copy vs hevc SDR fallback (range/bitdepth)", "hevc", "PQ", "hevc", "", false},
		{"hevc HLG copy vs hevc SDR fallback (range)", "hevc", "HLG", "hevc", "", false},

		// An unrecognised codec is never compatible - the safe default is an
		// explicit handoff, not a silent ABR switch.
		{"unknown codec never matches", "", "", "h264", "", false},
		{"RFC-6381 string folds to unknown (do not pass it)", "avc1.640028", "", "h264", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := NewABRGroup(c.aCodec, c.aRange)
			b := NewABRGroup(c.bCodec, c.bRange)
			if got := a.SameAs(b); got != c.want {
				t.Fatalf("NewABRGroup(%q,%q).SameAs(NewABRGroup(%q,%q)) = %v, want %v (a=%+v b=%+v)",
					c.aCodec, c.aRange, c.bCodec, c.bRange, got, c.want, a, b)
			}
			// SameAs must be symmetric.
			if got := b.SameAs(a); got != c.want {
				t.Fatalf("asymmetric: b.SameAs(a) = %v, want %v", got, c.want)
			}
		})
	}
}

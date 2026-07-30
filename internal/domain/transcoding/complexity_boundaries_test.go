package transcoding

import "testing"

// Every threshold in Classify, tested at the exact value and one step either
// side.
//
// The existing Classify tests sample the MIDDLE of each range, which is why
// mutation testing found 26 surviving mutants here: every `>=` could become
// `>`, every `<` could become `<=`, and the suite stayed green. These are not
// cosmetic comparisons - the class picks the encoding ladder, and the code's own
// comment says misclassifying a 4K master as animation means a 3.2 Mbps top rung
// instead of 15 Mbps. A master exactly 2160 tall sat on one of those edges.
//
// Cases are ordered the way Classify evaluates them, and each one sets the other
// fields so no earlier rule can claim it.
func TestClassify_Boundaries(t *testing.T) {
	tests := []struct {
		name string
		m    ComplexityMetrics
		want ContentClass
	}{
		// Rule 1: SourceHeight >= 2160, then SourceBitrateKbps >= 15000.
		{"just below 4K falls through to the normal rules",
			ComplexityMetrics{SourceHeight: 2159, SourceBitrateKbps: 20000, MotionMean: 2, ScenecutsPerMinute: 2, FramesSampled: 120},
			ClassFilmLowMotion},
		{"exactly 4K with a high bitrate is high-motion",
			ComplexityMetrics{SourceHeight: 2160, SourceBitrateKbps: 20000, MotionMean: 2, ScenecutsPerMinute: 2, FramesSampled: 120},
			ClassFilmHighMotion},
		{"4K exactly at the bitrate threshold is high-motion",
			ComplexityMetrics{SourceHeight: 2160, SourceBitrateKbps: 15000, MotionMean: 2, ScenecutsPerMinute: 2, FramesSampled: 120},
			ClassFilmHighMotion},
		{"4K one kbps below the threshold is normal",
			ComplexityMetrics{SourceHeight: 2160, SourceBitrateKbps: 14999, MotionMean: 2, ScenecutsPerMinute: 2, FramesSampled: 120},
			ClassFilmNormal},

		// Rule 2: ScenecutsPerMinute > 20 AND MotionMean > 4.
		{"sustained cuts and motion is sport",
			ComplexityMetrics{SourceHeight: 1080, ScenecutsPerMinute: 21, MotionMean: 4.1, SourceBitrateKbps: 8000, FramesSampled: 120},
			ClassSport},
		{"cuts exactly at 20 is not sport",
			ComplexityMetrics{SourceHeight: 1080, ScenecutsPerMinute: 20, MotionMean: 4.1, SourceBitrateKbps: 8000, FramesSampled: 120},
			ClassFilmHighMotion},
		{"motion exactly at 4 is not sport",
			ComplexityMetrics{SourceHeight: 1080, ScenecutsPerMinute: 21, MotionMean: 4, SourceBitrateKbps: 8000, FramesSampled: 120},
			ClassFilmHighMotion},

		// Rule 3: MotionMean < 1.5 AND ScenecutsPerMinute < 6 AND bitrate < 3000.
		{"all three animation signals just inside",
			ComplexityMetrics{SourceHeight: 1080, MotionMean: 1.4, ScenecutsPerMinute: 5, SourceBitrateKbps: 2999, FramesSampled: 120},
			ClassAnimation},
		{"motion exactly at 1.5 is not animation",
			ComplexityMetrics{SourceHeight: 1080, MotionMean: 1.5, ScenecutsPerMinute: 5, SourceBitrateKbps: 2999, FramesSampled: 120},
			ClassFilmLowMotion},
		{"cuts exactly at 6 is not animation",
			ComplexityMetrics{SourceHeight: 1080, MotionMean: 1.4, ScenecutsPerMinute: 6, SourceBitrateKbps: 2999, FramesSampled: 120},
			ClassFilmLowMotion},
		{"bitrate exactly at 3000 is not animation",
			ComplexityMetrics{SourceHeight: 1080, MotionMean: 1.4, ScenecutsPerMinute: 5, SourceBitrateKbps: 3000, FramesSampled: 120},
			ClassFilmLowMotion},

		// Rule 4: MotionMean < 3 AND ScenecutsPerMinute < 8.
		{"just inside low motion",
			ComplexityMetrics{SourceHeight: 1080, MotionMean: 2.9, ScenecutsPerMinute: 7, SourceBitrateKbps: 8000, FramesSampled: 120},
			ClassFilmLowMotion},
		{"motion exactly at 3 is not low motion",
			ComplexityMetrics{SourceHeight: 1080, MotionMean: 3, ScenecutsPerMinute: 7, SourceBitrateKbps: 8000, FramesSampled: 120},
			ClassFilmNormal},
		{"cuts exactly at 8 is not low motion",
			ComplexityMetrics{SourceHeight: 1080, MotionMean: 2.9, ScenecutsPerMinute: 8, SourceBitrateKbps: 8000, FramesSampled: 120},
			ClassFilmNormal},

		// Rule 5: MotionMean > 5 OR ScenecutsPerMinute > 15.
		{"motion just above 5 is high motion",
			ComplexityMetrics{SourceHeight: 1080, MotionMean: 5.1, ScenecutsPerMinute: 0, SourceBitrateKbps: 8000, FramesSampled: 120},
			ClassFilmHighMotion},
		{"motion exactly at 5 is not high motion",
			ComplexityMetrics{SourceHeight: 1080, MotionMean: 5, ScenecutsPerMinute: 0, SourceBitrateKbps: 8000, FramesSampled: 120},
			ClassFilmNormal},
		{"cuts just above 15 is high motion",
			ComplexityMetrics{SourceHeight: 1080, MotionMean: 0, ScenecutsPerMinute: 16, SourceBitrateKbps: 8000, FramesSampled: 120},
			ClassFilmHighMotion},
		{"cuts exactly at 15 is not high motion",
			ComplexityMetrics{SourceHeight: 1080, MotionMean: 0, ScenecutsPerMinute: 15, SourceBitrateKbps: 8000, FramesSampled: 120},
			ClassFilmNormal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.m); got != tt.want {
				t.Errorf("Classify(%+v) = %v, want %v", tt.m, got, tt.want)
			}
		})
	}
}

// The sample-free fallback has thresholds of its own, and they were the reason
// the first draft of the table above was wrong: with FramesSampled left at zero,
// Classify never reaches the motion rules at all. It decides on resolution and
// bitrate alone - which is the safer choice when ffmpeg gave us nothing, but it
// means these three edges also pick an encoding ladder.
func TestClassify_SampleFreeFallbackBoundaries(t *testing.T) {
	tests := []struct {
		name string
		m    ComplexityMetrics
		want ContentClass
	}{
		{"HD, very low bitrate, no samples: animation",
			ComplexityMetrics{SourceHeight: 1080, SourceBitrateKbps: 2499}, ClassAnimation},
		{"bitrate exactly at 2500 is not animation",
			ComplexityMetrics{SourceHeight: 1080, SourceBitrateKbps: 2500}, ClassFilmNormal},
		{"a zero bitrate tells us nothing, so not animation",
			ComplexityMetrics{SourceHeight: 1080, SourceBitrateKbps: 0}, ClassFilmNormal},
		{"below HD is never bucketed by this fallback",
			ComplexityMetrics{SourceHeight: 1079, SourceBitrateKbps: 2000}, ClassFilmNormal},
		{"HD at exactly 7000 is high motion",
			ComplexityMetrics{SourceHeight: 1080, SourceBitrateKbps: 7000}, ClassFilmHighMotion},
		{"one kbps below 7000 is normal",
			ComplexityMetrics{SourceHeight: 1080, SourceBitrateKbps: 6999}, ClassFilmNormal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.m); got != tt.want {
				t.Errorf("Classify(%+v) = %v, want %v", tt.m, got, tt.want)
			}
		})
	}
}

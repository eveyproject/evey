package transcoding

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sw builds one software-ladder sample. Passing is set to what the CALLER claims
// the bench recorded - Derive must RE-derive it from FPS vs the policy margin, not
// trust this flag (except when SourceRate is unknown), so tests deliberately pass
// a stale Passing to prove recomputation.
func sample(ladder string, height int, fps float64, passing bool) BenchRungSample {
	return BenchRungSample{Ladder: ladder, Height: height, FPS: fps, Passing: passing}
}

func TestDerive_SoftwareCeiling_TallestPassing(t *testing.T) {
	rep := BenchReport{
		SourceRate: 24,
		Samples: []BenchRungSample{
			sample(LadderSoftware, 720, 100, true),
			sample(LadderSoftware, 1080, 50, true),
			sample(LadderSoftware, 1440, 10, false), // 10 < 28.8 required
		},
	}
	got := Derive(rep, DefaultDerivePolicy())
	assert.Equal(t, 1080, got.SoftwareMaxHeight)
}

func TestDerive_SoftwareCeiling_FloorWhenNonePass(t *testing.T) {
	rep := BenchReport{
		SourceRate: 24,
		Samples:    []BenchRungSample{sample(LadderSoftware, 720, 5, false)},
	}
	got := Derive(rep, DefaultDerivePolicy())
	assert.Equal(t, 720, got.SoftwareMaxHeight, "floor to the universal 720p when no rung sustains realtime")
}

func TestDerive_PerLadderCeilings(t *testing.T) {
	rep := BenchReport{
		SourceRate: 24,
		HWBackend:  "nvenc",
		Samples: []BenchRungSample{
			sample(LadderSoftware, 720, 100, true),
			sample(LadderSoftware, 1080, 100, true),
			sample(LadderSoftwareHDR, 720, 100, true),
			sample(LadderSoftwareHDR, 1080, 10, false),
			sample(LadderSoftwareAV1, 720, 100, true), // 100 >= 24*1.2*2.0 = 57.6
			sample(LadderHW, 720, 400, true),
			sample(LadderHW, 1080, 400, true),
			sample(LadderHW, 1440, 400, true),
			sample(LadderHW, 2160, 400, true),
		},
	}
	got := Derive(rep, DefaultDerivePolicy())
	assert.Equal(t, 1080, got.SoftwareMaxHeight)
	assert.Equal(t, 720, got.SoftwareHDRMaxHeight)
	assert.Equal(t, 720, got.SoftwareAV1MaxHeight)
	assert.Equal(t, 2160, got.HWMaxHeight)
}

func TestDerive_HWCeiling_GatedOnBackend(t *testing.T) {
	// hw samples pass, but no live HW backend (e.g. the persisted backend was
	// revalidated dead and stripped): the hw ceiling must be 0, never derived
	// from stale hw samples that no backend can serve.
	rep := BenchReport{
		SourceRate: 24,
		HWBackend:  "", // stripped
		Samples: []BenchRungSample{
			sample(LadderSoftware, 720, 100, true),
			sample(LadderHW, 2160, 400, true),
		},
	}
	got := Derive(rep, DefaultDerivePolicy())
	assert.Zero(t, got.HWMaxHeight, "no backend => no hw ceiling regardless of hw samples")
}

func TestDerive_RecomputesPassingFromSourceRate(t *testing.T) {
	// Same evidence, two policies: a stricter margin re-derives a LOWER ceiling
	// from the SAME bench samples, without re-benching. This is the whole point
	// of splitting proof from policy. Every sample carries Passing=true (what the
	// 1.2 bench recorded) - Derive must ignore it and recompute.
	rep := BenchReport{
		SourceRate: 24,
		Samples: []BenchRungSample{
			sample(LadderSoftware, 720, 100, true),
			sample(LadderSoftware, 1080, 50, true),
			sample(LadderSoftware, 1440, 30, true),
		},
	}
	lax := DefaultDerivePolicy() // margin 1.2 -> required 28.8 -> 1440 (30) passes
	assert.Equal(t, 1440, Derive(rep, lax).SoftwareMaxHeight)

	strict := DefaultDerivePolicy()
	strict.Margin = 1.3 // required 31.2 -> 1440 (30) fails, 1080 (50) still passes
	assert.Equal(t, 1080, Derive(rep, strict).SoftwareMaxHeight)
}

func TestDerive_SourceRateZero_TrustsStoredPassing(t *testing.T) {
	// Rows persisted before SourceRate existed carry no rate: Derive cannot
	// recompute, so it trusts the Passing flag the bench stored, reproducing the
	// old ceiling exactly.
	rep := BenchReport{
		SourceRate: 0,
		Samples: []BenchRungSample{
			sample(LadderSoftware, 720, 0, true),
			sample(LadderSoftware, 1080, 0, true),
			sample(LadderSoftware, 1440, 0, false),
		},
	}
	got := Derive(rep, DefaultDerivePolicy())
	assert.Equal(t, 1080, got.SoftwareMaxHeight)
}

func TestDerive_AV1_UsesMultiplierMargin(t *testing.T) {
	// AV1 must clear margin*multiplier (2.4x realtime by default). A rung that
	// only scrapes the base margin is NOT offered an AV1 software ceiling.
	rep := BenchReport{
		SourceRate: 24,
		Samples: []BenchRungSample{
			sample(LadderSoftwareAV1, 720, 40, true), // 40 >= 28.8 (base) but < 57.6 (av1)
		},
	}
	got := Derive(rep, DefaultDerivePolicy())
	assert.Zero(t, got.SoftwareAV1MaxHeight, "AV1 floor is 0 (hardware-only) when no rung clears the AV1 margin")
}

func TestDerive_StampsPolicyVersionAndProof(t *testing.T) {
	now := time.Now().UTC()
	rep := BenchReport{
		SourceRate:          24,
		HWBackend:           "vaapi",
		HWDevice:            "/dev/dri/renderD128",
		HWCodecs:            []string{"h264", "hevc"},
		HWDecodeCodecs:      []string{"h264"},
		HW10BitTranscode:    false,
		HWTonemap:           true,
		HWFeatures:          []string{"forced_idr"},
		GPUClass:            "integrated",
		DetectedBackends:    []string{"vaapi"},
		CPUSignature:        "abc123",
		BenchmarkedAt:       now,
		BenchmarkDurationMs: 22000,
		Samples:             []BenchRungSample{sample(LadderSoftware, 720, 100, true), sample(LadderHW, 1080, 400, true)},
	}
	got := Derive(rep, DefaultDerivePolicy())

	assert.Equal(t, PolicyVersion, got.PolicyVersion)
	assert.Equal(t, 24, got.SourceRate)
	// Proof passes through verbatim - the derived profile still carries the
	// evidence the admin view renders.
	assert.Equal(t, "vaapi", got.HWBackend)
	assert.Equal(t, "/dev/dri/renderD128", got.HWDevice)
	assert.Equal(t, []string{"h264", "hevc"}, got.HWCodecs)
	assert.Equal(t, []string{"h264"}, got.HWDecodeCodecs)
	assert.True(t, got.HWTonemap)
	assert.Equal(t, []string{"forced_idr"}, got.HWFeatures)
	assert.Equal(t, "integrated", got.GPUClass)
	assert.Equal(t, []string{"vaapi"}, got.DetectedBackends)
	assert.Equal(t, "abc123", got.CPUSignature)
	assert.Equal(t, now, got.BenchmarkedAt)
	assert.Equal(t, int64(22000), got.BenchmarkDurationMs)
	assert.Len(t, got.BenchSamples, 2)
}

func TestDerive_DefaultPolicy_FillsZeroValues(t *testing.T) {
	// A caller passing DerivePolicy{} (all zero) must get the production defaults,
	// so an unconfigured call is byte-identical to DefaultDerivePolicy().
	rep := BenchReport{
		SourceRate: 24,
		Samples: []BenchRungSample{
			sample(LadderSoftware, 720, 100, true),
			sample(LadderSoftware, 1080, 30, true), // 30 >= 28.8 with default margin
		},
	}
	assert.Equal(t, Derive(rep, DefaultDerivePolicy()), Derive(rep, DerivePolicy{}))
}

func TestEncodeCapability_Report_Roundtrip(t *testing.T) {
	// Report() extracts the proof; Derive re-projects it. For a capability whose
	// ceilings already match its samples under the default policy, the round-trip
	// is a fixed point (proves Report<->Derive are consistent projections).
	cap := EncodeCapability{
		SoftwareMaxHeight:    1080,
		SoftwareHDRMaxHeight: 720,
		HWMaxHeight:          1080,
		HWBackend:            "nvenc",
		CPUSignature:         "sig",
		SourceRate:           24,
		BenchSamples: []BenchRungSample{
			sample(LadderSoftware, 720, 100, true),
			sample(LadderSoftware, 1080, 100, true),
			sample(LadderSoftware, 1440, 10, false),
			sample(LadderSoftwareHDR, 720, 100, true),
			sample(LadderSoftwareHDR, 1080, 10, false),
			sample(LadderHW, 720, 400, true),
			sample(LadderHW, 1080, 400, true),
			sample(LadderHW, 1440, 10, false),
		},
	}
	got := Derive(cap.Report(), DefaultDerivePolicy())
	assert.Equal(t, cap.SoftwareMaxHeight, got.SoftwareMaxHeight)
	assert.Equal(t, cap.SoftwareHDRMaxHeight, got.SoftwareHDRMaxHeight)
	assert.Equal(t, cap.HWMaxHeight, got.HWMaxHeight)
	require.Equal(t, cap.HWBackend, got.HWBackend)
}

// withDefaults fills any zero-or-negative field from the default policy. Every
// one of those guards survived mutation testing, because the tests only ever
// passed a fully-populated policy or an entirely empty one - never a policy with
// ONE field set, which is the case the function exists for.
func TestDerivePolicyWithDefaults_FillsEachFieldIndependently(t *testing.T) {
	d := DefaultDerivePolicy()

	t.Run("a zero field takes the default", func(t *testing.T) {
		got := DerivePolicy{}.withDefaults()
		if got.Margin != d.Margin || got.AV1MarginMultiplier != d.AV1MarginMultiplier ||
			got.SoftwareFloor != d.SoftwareFloor || got.SoftwareHDRFloor != d.SoftwareHDRFloor {
			t.Fatalf("empty policy did not take the defaults: %+v", got)
		}
	})

	t.Run("a negative field is treated as unset", func(t *testing.T) {
		// The guard is `<= 0`, not `== 0`: a negative margin is nonsense and
		// must fall back rather than be honoured.
		got := DerivePolicy{Margin: -1, AV1MarginMultiplier: -1, SoftwareFloor: -1, SoftwareHDRFloor: -1}.withDefaults()
		if got.Margin != d.Margin || got.AV1MarginMultiplier != d.AV1MarginMultiplier ||
			got.SoftwareFloor != d.SoftwareFloor || got.SoftwareHDRFloor != d.SoftwareHDRFloor {
			t.Fatalf("negative fields were not replaced: %+v", got)
		}
	})

	// One field set at a time: proves each guard is independent, i.e. that
	// setting Margin does not accidentally also keep a zero SoftwareFloor.
	t.Run("a set field is preserved while the others default", func(t *testing.T) {
		got := DerivePolicy{Margin: d.Margin + 0.5}.withDefaults()
		if got.Margin != d.Margin+0.5 {
			t.Errorf("Margin = %v, want the caller's value", got.Margin)
		}
		if got.SoftwareFloor != d.SoftwareFloor {
			t.Errorf("SoftwareFloor = %v, want the default", got.SoftwareFloor)
		}

		got = DerivePolicy{SoftwareFloor: 999}.withDefaults()
		if got.SoftwareFloor != 999 {
			t.Errorf("SoftwareFloor = %v, want the caller's value", got.SoftwareFloor)
		}
		if got.Margin != d.Margin {
			t.Errorf("Margin = %v, want the default", got.Margin)
		}

		got = DerivePolicy{AV1MarginMultiplier: 3}.withDefaults()
		if got.AV1MarginMultiplier != 3 || got.Margin != d.Margin {
			t.Errorf("AV1MarginMultiplier guard is not independent: %+v", got)
		}

		got = DerivePolicy{SoftwareHDRFloor: 42}.withDefaults()
		if got.SoftwareHDRFloor != 42 || got.SoftwareFloor != d.SoftwareFloor {
			t.Errorf("SoftwareHDRFloor guard is not independent: %+v", got)
		}
	})
}

func TestDerive_HWBridgeCeiling(t *testing.T) {
	rep := BenchReport{
		SourceRate: 24,
		HWBackend:  "nvenc",
		Samples: []BenchRungSample{
			sample(LadderHWBridge, 720, 200, true),
			sample(LadderHWBridge, 1080, 90, true),
			sample(LadderHWBridge, 1440, 20, false),
		},
	}
	assert.Equal(t, 1080, Derive(rep, DefaultDerivePolicy()).HWBridgeMaxHeight)
	rep.HWBackend = ""
	assert.Equal(t, 0, Derive(rep, DefaultDerivePolicy()).HWBridgeMaxHeight, "no live backend: no bridge ceiling")
	assert.Equal(t, 0, Derive(BenchReport{SourceRate: 24, HWBackend: "nvenc"}, DefaultDerivePolicy()).HWBridgeMaxHeight, "unmeasured: 0, the software HDR cap applies")
}

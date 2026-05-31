package transcoding

import (
	"slices"
	"testing"
)

func TestFailureClass_IsHWCapability(t *testing.T) {
	hw := []FailureClass{
		FailureHWUnsupportedOption, FailureHWEncoderMissing,
		FailureHWInit, FailureHWDeviceMemory,
	}
	notHW := []FailureClass{
		FailureNone, FailureTransient, FailureCancelled, FailureUnknown,
	}
	for _, c := range hw {
		if !c.IsHWCapability() {
			t.Errorf("%q should be a HW-capability class", c)
		}
	}
	for _, c := range notHW {
		if c.IsHWCapability() {
			t.Errorf("%q should NOT be a HW-capability class", c)
		}
	}
}

func TestFailureClass_Deterministic(t *testing.T) {
	det := []FailureClass{FailureHWUnsupportedOption, FailureHWEncoderMissing, FailureHWInit}
	nondet := []FailureClass{FailureHWDeviceMemory, FailureTransient, FailureCancelled, FailureUnknown, FailureNone}
	for _, c := range det {
		if !c.Deterministic() {
			t.Errorf("%q should be deterministic (persist demotion)", c)
		}
	}
	for _, c := range nondet {
		if c.Deterministic() {
			t.Errorf("%q should NOT be deterministic", c)
		}
	}
}

func fullCap() EncodeCapability {
	return EncodeCapability{
		SoftwareMaxHeight: 1080,
		HWMaxHeight:       2160,
		HWBackend:         "nvenc",
		HWCodecs:          []string{"h264", "hevc"},
		HWDevice:          "",
		HWFeatures:        []string{"forced_idr", "temporal_aq", "rc_lookahead"},
	}
}

func TestPlanDemotion(t *testing.T) {
	type tc struct {
		name        string
		class       FailureClass
		codec       string
		wantTier    RetryTier
		wantPersist bool
		wantChanged bool
		// assertions on the demoted capability:
		check func(t *testing.T, d EncodeCapability)
	}
	cases := []tc{
		{
			name: "unsupported option drops tuning, keeps HW+codec",
			class: FailureHWUnsupportedOption, codec: "h264",
			wantTier: TierHWMinimal, wantPersist: true, wantChanged: true,
			check: func(t *testing.T, d EncodeCapability) {
				if len(d.HWFeatures) != 0 {
					t.Errorf("HWFeatures should be cleared, got %v", d.HWFeatures)
				}
				if d.HWBackend != "nvenc" || d.HWMaxHeight != 2160 {
					t.Errorf("HW backend/ceiling must survive, got %q/%d", d.HWBackend, d.HWMaxHeight)
				}
				if !slices.Contains(d.HWCodecs, "h264") {
					t.Errorf("codec must survive, got %v", d.HWCodecs)
				}
			},
		},
		{
			name: "encoder missing removes that codec, keeps others + HW",
			class: FailureHWEncoderMissing, codec: "hevc",
			wantTier: TierSoftware, wantPersist: true, wantChanged: true,
			check: func(t *testing.T, d EncodeCapability) {
				if slices.Contains(d.HWCodecs, "hevc") {
					t.Errorf("hevc should be removed, got %v", d.HWCodecs)
				}
				if !slices.Contains(d.HWCodecs, "h264") {
					t.Errorf("h264 should survive, got %v", d.HWCodecs)
				}
				if d.HWBackend != "nvenc" {
					t.Errorf("backend should survive while a codec remains, got %q", d.HWBackend)
				}
			},
		},
		{
			name: "encoder missing on the LAST codec disables HW entirely",
			class: FailureHWEncoderMissing, codec: "h264",
			wantTier: TierSoftware, wantPersist: true, wantChanged: true,
			check: func(t *testing.T, d EncodeCapability) {
				// fullCap has h264+hevc; removing only h264 here keeps hevc, so
				// HW must NOT be disabled. (Guards against over-eager disable.)
				if d.HWBackend != "nvenc" {
					t.Errorf("hevc remains → backend should survive, got %q", d.HWBackend)
				}
			},
		},
		{
			name: "HW init failure disables HW for the whole host",
			class: FailureHWInit, codec: "h264",
			wantTier: TierSoftware, wantPersist: true, wantChanged: true,
			check: func(t *testing.T, d EncodeCapability) {
				if d.HWBackend != "" || d.HWMaxHeight != 0 || len(d.HWCodecs) != 0 || len(d.HWFeatures) != 0 {
					t.Errorf("HW must be fully disabled, got %+v", d)
				}
				if d.SoftwareMaxHeight != 1080 {
					t.Errorf("software ceiling must survive, got %d", d.SoftwareMaxHeight)
				}
			},
		},
		{
			name: "device OOM falls back for this job only, no persist",
			class: FailureHWDeviceMemory, codec: "h264",
			wantTier: TierSoftware, wantPersist: false, wantChanged: false,
			check: func(t *testing.T, d EncodeCapability) {
				if d.HWBackend != "nvenc" || d.HWMaxHeight != 2160 {
					t.Errorf("capability must be untouched on transient OOM, got %+v", d)
				}
			},
		},
		{
			name: "transient retries same tier, no change",
			class: FailureTransient, codec: "h264",
			wantTier: TierHWFull, wantPersist: false, wantChanged: false,
		},
		{
			name: "cancelled is a no-op",
			class: FailureCancelled, codec: "h264",
			wantTier: TierHWFull, wantPersist: false, wantChanged: false,
		},
		{
			name: "unknown falls back to software without persisting",
			class: FailureUnknown, codec: "h264",
			wantTier: TierSoftware, wantPersist: false, wantChanged: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := PlanDemotion(fullCap(), c.class, c.codec)
			if d.RetryTier != c.wantTier {
				t.Errorf("tier = %q, want %q", d.RetryTier, c.wantTier)
			}
			if d.Persist != c.wantPersist {
				t.Errorf("persist = %v, want %v", d.Persist, c.wantPersist)
			}
			if d.Changed != c.wantChanged {
				t.Errorf("changed = %v, want %v", d.Changed, c.wantChanged)
			}
			if c.check != nil {
				c.check(t, d.Capability)
			}
		})
	}
}

func TestPlanDemotion_DoesNotMutateInput(t *testing.T) {
	in := fullCap()
	_ = PlanDemotion(in, FailureHWInit, "h264")
	if in.HWBackend != "nvenc" || len(in.HWCodecs) != 2 || len(in.HWFeatures) != 3 {
		t.Fatalf("input capability was mutated: %+v", in)
	}
}

func TestPlanDemotion_UnsupportedOption_NoTuning_NoChange(t *testing.T) {
	// A capability that already has no tuning options: clearing is a no-op.
	cap := EncodeCapability{HWBackend: "vaapi", HWMaxHeight: 1080, HWCodecs: []string{"h264"}}
	d := PlanDemotion(cap, FailureHWUnsupportedOption, "h264")
	if d.Changed {
		t.Errorf("clearing already-empty HWFeatures should be Changed=false")
	}
	if d.RetryTier != TierHWMinimal {
		t.Errorf("tier = %q, want hw_minimal", d.RetryTier)
	}
}

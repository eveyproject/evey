package transcoding

import "time"

// PolicyVersion identifies the Derive policy currently in force. Bump it whenever
// Derive's RULES change (a margin, a floor, the way a ceiling is read off the
// samples) - never for a proof/measurement change. On boot, a persisted
// capability stamped with an older PolicyVersion is re-derived from its own bench
// evidence (see CalibrationService.EnsureCalibrated), so a policy change in a
// release re-caps every host deterministically, without re-benching.
const PolicyVersion = 1

// Bench ladder labels (BenchRungSample.Ladder). The benchmark tags each rung with
// the pipeline shape it measured; Derive groups samples by these to read one
// ceiling per shape. Kept here (next to Derive, the sole consumer of the grouping)
// so the proof->policy split owns the vocabulary.
const (
	LadderSoftware    = "software"
	LadderSoftwareHDR = "software-hdr"
	LadderSoftwareAV1 = "software-av1"
	LadderHW          = "hw"
)

// DerivePolicy is the tunable policy that turns raw bench EVIDENCE into delivery
// CEILINGS: the realtime margin a rung must clear and the per-ladder floor used
// when none does. It is the policy half the plan pulls OUT of benchmark.go (where
// the 1.2 margin, the 2.0 AV1 multiplier and the 720/480/0 floors were welded into
// the measurement) so the same evidence can be re-capped under a new policy in a
// release without re-benching.
//
// Zero fields fall back to the production defaults (DefaultDerivePolicy) via
// withDefaults, so Derive(rep, DerivePolicy{}) == Derive(rep, DefaultDerivePolicy()).
//
// NOTE (scope): operator ceiling OVERRIDES (EVEY_TRANSCODING_SOFTWARE_MAX_HEIGHT,
// the live AV1 admin knob) are deliberately NOT here - they are a runtime OVERLAY
// applied by CalibrationService.Current() on every read, orthogonal to the
// bench->ceiling derivation, and must stay hot (a live AV1 change takes effect
// without a re-derive). Derive owns only the evidence->ceiling policy.
type DerivePolicy struct {
	// Margin is the realtime headroom a rung's encode fps must clear to count as
	// sustainable: required = SourceRate × Margin. Default 1.2 (20% headroom).
	Margin float64
	// AV1MarginMultiplier raises the margin the AV1 software ladder must clear, on
	// top of Margin (AV1 software cold-start needs real spare capacity). Default 2.0.
	AV1MarginMultiplier float64
	// SoftwareFloor / SoftwareHDRFloor / SoftwareAV1Floor are the heights used when
	// no rung of that ladder passes. 720 (universal SDR floor), 480 (HDR is
	// heavier), 0 (AV1: no floor - 0 means "AV1 software not viable, stay
	// hardware-only", which the handler gate reads).
	SoftwareFloor    int
	SoftwareHDRFloor int
	SoftwareAV1Floor int
}

// DefaultDerivePolicy is the production policy: the same margins and floors
// benchmark.go applied inline before the proof/policy split. Kept as the single
// source of truth so a change here re-derives every host on the next boot.
func DefaultDerivePolicy() DerivePolicy {
	return DerivePolicy{
		Margin:              1.2,
		AV1MarginMultiplier: 2.0,
		SoftwareFloor:       720,
		SoftwareHDRFloor:    480,
		SoftwareAV1Floor:    0,
	}
}

// withDefaults fills any zero field from DefaultDerivePolicy so a partially- or
// un-configured policy still derives sane ceilings.
func (p DerivePolicy) withDefaults() DerivePolicy {
	d := DefaultDerivePolicy()
	if p.Margin <= 0 {
		p.Margin = d.Margin
	}
	if p.AV1MarginMultiplier <= 0 {
		p.AV1MarginMultiplier = d.AV1MarginMultiplier
	}
	if p.SoftwareFloor <= 0 {
		p.SoftwareFloor = d.SoftwareFloor
	}
	if p.SoftwareHDRFloor <= 0 {
		p.SoftwareHDRFloor = d.SoftwareHDRFloor
	}
	// SoftwareAV1Floor default is 0 (hardware-only) - a legitimate value, so it is
	// NOT filled from the default (which is also 0). Left as the caller set it.
	return p
}

// BenchReport is the raw, policy-free EVIDENCE a benchmark produced: the per-rung
// throughput samples plus the host/HW facts probed alongside them. It is the
// PROOF. Every delivery ceiling is DERIVED from it by Derive, so a policy change
// re-derives each host from its last report without re-benching. Split out from
// the derived EncodeCapability precisely so the two cannot drift - the omitempty
// round-trip hazard that has bitten EncodeCapability (see capability.go).
//
// The HW fields are proof (what the accelerator was measured to do), not policy:
// Derive passes them through to the derived capability unchanged.
type BenchReport struct {
	// Samples is the per-rung evidence, tagged by ladder (LadderSoftware, ...).
	Samples []BenchRungSample
	// SourceRate is the fps the samples were measured against (required =
	// SourceRate × margin). 0 ⇒ unknown (old row): Derive trusts Sample.Passing.
	SourceRate int

	HWBackend           string
	HWDevice            string
	HWCodecs            []string
	HWDecodeCodecs      []string
	HW10BitTranscode    bool
	HWTonemap           bool
	HWFeatures          []string
	GPUClass            string
	DetectedBackends    []string
	CPUSignature        string
	BenchmarkedAt       time.Time
	BenchmarkDurationMs int64

	// DecodeSamples / BenchSource are the DECODE half of the proof - what the
	// host measured for each source codec, and which source the encode rungs
	// were decoded from. They must round-trip through here or every boot would
	// silently drop them (Derive rebuilds the capability from this report), and
	// admission would fall back to output-rung pricing until the next re-bench.
	DecodeSamples []DecodeRungSample
	BenchSource   SourceClass

	// HWDemotedAt is not proof but runtime state, carried for the same reason:
	// dropping it on boot would lose the demotion's clock, making a persisted
	// demotion permanent again - exactly the failure it exists to bound.
	HWDemotedAt time.Time
}

// Report projects an EncodeCapability onto its PROOF - the evidence + host/HW
// facts Derive re-derives ceilings from. It deliberately drops the derived fields
// (the ceilings, PolicyVersion): re-deriving from the proof reproduces them.
func (c EncodeCapability) Report() BenchReport {
	return BenchReport{
		Samples:             c.BenchSamples,
		SourceRate:          c.SourceRate,
		HWBackend:           c.HWBackend,
		HWDevice:            c.HWDevice,
		HWCodecs:            c.HWCodecs,
		HWDecodeCodecs:      c.HWDecodeCodecs,
		HW10BitTranscode:    c.HW10BitTranscode,
		HWTonemap:           c.HWTonemap,
		HWFeatures:          c.HWFeatures,
		GPUClass:            c.GPUClass,
		DetectedBackends:    c.DetectedBackends,
		CPUSignature:        c.CPUSignature,
		BenchmarkedAt:       c.BenchmarkedAt,
		BenchmarkDurationMs: c.BenchmarkDurationMs,
		DecodeSamples:       c.DecodeSamples,
		BenchSource:         c.BenchSource,
		HWDemotedAt:         c.HWDemotedAt,
	}
}

// Derive turns a BenchReport (proof) into an EncodeCapability (proof + derived
// ceilings + PolicyVersion) under policy. It is PURE - no I/O, no clock, no DB -
// so the whole ceiling policy is table-testable without ffmpeg, and a policy
// change re-derives every host from its last report on the next boot.
//
// It consolidates the ceiling policy that used to live inline in benchmark.go:
// the realtime margin, the AV1 margin multiplier, and the per-ladder floors. The
// pass/fail of each rung is RE-derived from FPS vs (SourceRate × margin) rather
// than trusting the bench-time Passing flag, so a margin change actually re-caps
// (when SourceRate is unknown - old rows - the stored Passing is trusted instead).
//
// The hardware ceiling is gated on rep.HWBackend: no live backend ⇒ no hw ceiling,
// even if stale hw samples are present (e.g. a persisted backend revalidated dead
// and stripped). The HW proof fields pass through unchanged.
func Derive(rep BenchReport, policy DerivePolicy) EncodeCapability {
	policy = policy.withDefaults()

	swMax := ceilingFor(rep, LadderSoftware, policy.Margin, policy.SoftwareFloor)
	swHDR := ceilingFor(rep, LadderSoftwareHDR, policy.Margin, policy.SoftwareHDRFloor)
	swAV1 := ceilingFor(rep, LadderSoftwareAV1, policy.Margin*policy.AV1MarginMultiplier, policy.SoftwareAV1Floor)

	hwMax := 0
	if rep.HWBackend != "" {
		// hw floor is 0: a host with a backend but no passing hw rung has no hw
		// ceiling (falls back to software), never a forced floor.
		hwMax = ceilingFor(rep, LadderHW, policy.Margin, 0)
	}

	return EncodeCapability{
		SoftwareMaxHeight:    swMax,
		SoftwareHDRMaxHeight: swHDR,
		SoftwareAV1MaxHeight: swAV1,
		HWMaxHeight:          hwMax,
		HWBackend:            rep.HWBackend,
		HWCodecs:             rep.HWCodecs,
		HWDecodeCodecs:       rep.HWDecodeCodecs,
		HW10BitTranscode:     rep.HW10BitTranscode,
		HWTonemap:            rep.HWTonemap,
		HWDevice:             rep.HWDevice,
		GPUClass:             rep.GPUClass,
		HWFeatures:           rep.HWFeatures,
		DetectedBackends:     rep.DetectedBackends,
		CPUSignature:         rep.CPUSignature,
		BenchmarkedAt:        rep.BenchmarkedAt,
		BenchmarkDurationMs:  rep.BenchmarkDurationMs,
		BenchSamples:         rep.Samples,
		SourceRate:           rep.SourceRate,
		DecodeSamples:        rep.DecodeSamples,
		BenchSource:          rep.BenchSource,
		HWDemotedAt:          rep.HWDemotedAt,
		PolicyVersion:        PolicyVersion,
	}
}

// ceilingFor returns the tallest rung of ladder that sustains realtime under
// margin, or floor when none does. A rung passes when FPS >= SourceRate × margin;
// when SourceRate is unknown (0, old rows), the bench-time Passing flag is trusted
// instead (Derive can't recompute without the rate). The MAX passing height is
// taken, not "walk until first failure": hw costs are not monotonic in height, so
// a taller rung can pass while a shorter one fails.
func ceilingFor(rep BenchReport, ladder string, margin float64, floor int) int {
	required := float64(rep.SourceRate) * margin
	best := 0
	for _, s := range rep.Samples {
		if s.Ladder != ladder {
			continue
		}
		var passing bool
		if rep.SourceRate > 0 {
			passing = s.FPS >= required
		} else {
			passing = s.Passing
		}
		if passing && s.Height > best {
			best = s.Height
		}
	}
	if best == 0 {
		return floor
	}
	return best
}

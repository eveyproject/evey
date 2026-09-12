package transcoding

// probe_trigger.go - which titles earn a per-title probe (plan P3). A probe
// costs real encodes, so it runs ONLY where its information gain justifies it:
// never a library sweep. The decision is a pure function of what is already
// persisted about the title plus a runtime signal the serving path records,
// so it is testable and the same on the worker and in a dry run.

// ProbeReason names why a title is a probe candidate; "" = no probe.
type ProbeReason string

const (
	// ProbeNoMetrics: never classified, so the cost model has no prediction.
	ProbeNoMetrics ProbeReason = "no_metrics"
	// ProbeYDIFOutOfRange: the motion is outside the fitted range, so the
	// prediction is an extrapolation (confidence 0.5).
	ProbeYDIFOutOfRange ProbeReason = "ydif_out_of_range"
	// ProbeCeilingHit: a real transcode of this title kept hitting its
	// advertised ceiling (segments at maxrate) - the model under-priced it.
	ProbeCeilingHit ProbeReason = "ceiling_hit"
	// ProbeModelWeakSpot: the title sits in a region the model is known to
	// mispredict (calibration: animation, the worst LOO case).
	ProbeModelWeakSpot ProbeReason = "model_weak_spot"
)

// ProbeSignals are the runtime facts the serving path records for a title,
// alongside its persisted metrics.
type ProbeSignals struct {
	// CeilingHitSegments is how many segments of a real transcode of this
	// title shipped at (or above) their advertised ceiling; a handful is
	// noise, a sustained count means the price was too low.
	CeilingHitSegments int
	// Animation marks the class the calibration mispredicted worst.
	Animation bool
}

// ProbeCeilingHitThreshold is the sustained count above which a ceiling-hit
// title is a candidate (a spike or two is normal VBR behaviour).
const ProbeCeilingHitThreshold = 12

// ProbeCandidate reports whether a title should be probed, and why. A title
// already probed at the current model version is never re-probed (the probe
// is the ground truth; only a model bump reopens it). The order encodes
// priority: a title with no prediction at all first, then a measured
// under-price, then an extrapolation, then a known weak spot.
func ProbeCandidate(m ComplexityMetrics, sig ProbeSignals, model CostModel) (ProbeReason, bool) {
	if p := m.Probe; p != nil && !p.Failed && p.Protocol == TitleProbeProtocol && p.ModelVersion == model.Version {
		return "", false // already ground-truthed for this model
	}
	motion, ok := m.CanonicalMotion()
	if !ok || m.FramesSampled < model.MinFrames || motion <= 0 {
		return ProbeNoMetrics, true
	}
	if sig.CeilingHitSegments >= ProbeCeilingHitThreshold {
		return ProbeCeilingHit, true
	}
	if motion < model.YDIFMin || motion > model.YDIFMax {
		return ProbeYDIFOutOfRange, true
	}
	if sig.Animation {
		return ProbeModelWeakSpot, true
	}
	return "", false
}

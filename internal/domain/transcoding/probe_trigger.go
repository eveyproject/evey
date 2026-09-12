package transcoding

// probe_trigger.go - which titles earn a per-title probe (plan P3). A probe
// costs real encodes, so it runs ONLY where its information gain justifies it:
// never a library sweep. The decision is a pure function of what is already
// persisted about the title plus a runtime signal the serving path records,
// so it is testable and the same on the worker and in a dry run.

// ProbeReason names why a title is a probe candidate; "" = no probe.
type ProbeReason string

const (
	// ProbeClassifyFailed: the classifier produced a row but sampled no motion
	// (a readable file it could not analyse - the sample-free fallback). The
	// cheap classifier has already tried and settled; only a probe can price
	// this title. A title never classified at all is NOT here: the classifier
	// runs first, and the probe fires only if its result then trips a trigger.
	ProbeClassifyFailed ProbeReason = "classify_failed"
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
// already probed at the current identity (protocol + model version + encoder)
// is never re-probed - whether that probe succeeded (it is the ground truth) or
// failed (the source could not be measured; retrying it forever is pointless).
// Only an identity bump reopens it. The order encodes priority: a title with no
// prediction at all first, then a measured under-price, then an extrapolation,
// then a known weak spot.
func ProbeCandidate(m ComplexityMetrics, sig ProbeSignals, model CostModel) (ProbeReason, bool) {
	if p := m.Probe; p.CurrentIdentity(model.Version) {
		return "", false // already probed (or terminally failed) for this identity
	}
	motion, ok := m.CanonicalMotion()
	if !ok {
		// No canonical metrics: never classified, or a pre-canonical row. This
		// is the CLASSIFIER's job (a cheap YDIF sample -> a P1/P2 prediction),
		// not the probe's. The probe fires only once the classifier's result
		// trips a trigger below.
		return "", false
	}
	if m.FramesSampled < model.MinFrames || motion <= 0 {
		// The classifier ran but sampled no motion (a readable file it could
		// not analyse): it has settled as failed, so the probe measures the
		// truth. A single fallback is terminal - the classifier re-fails a
		// present file identically, so it never retries it.
		return ProbeClassifyFailed, true
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

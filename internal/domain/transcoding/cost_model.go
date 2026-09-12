package transcoding

import "math"

// cost_model.go - the per-title COST model (ladder engine, plan P1): what a
// fixed quality costs on this title, in bits per pixel per frame. It predicts
// cost, never quality: how faithful a CRF is to a title is measured by the
// per-title probes (P3), not guessed here.
//
// Calibrated on the ladder-bench corpus (evey admin ladder-bench): every
// candidate feature was ranked by its leave-one-title-out predictive gain
// before any rule was written. One feature earned its place, the mean
// frame-to-frame luma difference (signalstats YDIF, the classifier's
// MotionMean); temporal information said the same thing (rho 0.92), spatial
// entropy added nothing once YDIF was known, and scene cuts, grain, blur,
// blockiness, HDR, bit depth, source codec, source bitrate and source height
// predicted nothing. The model stays that simple until a corpus says otherwise.

// CostModel is one calibrated fit, versioned: a new fit is a new version,
// and the version rides the ladder it shaped (a different prediction is a
// different rung bitrate, hence a different cache hash).
type CostModel struct {
	Version int
	// log(bpp at the reference point) = Intercept + Slope * log(YDIF).
	Intercept, Slope float64
	// The reference operating point of the fit: rung height, encoder CRF.
	RefHeight, RefCRF int
	// ResolutionExponent: cost follows pixels^exponent (smaller rungs cost
	// more per pixel). CRFHalvingStep: CRF points per halving of the cost.
	ResolutionExponent float64
	CRFHalvingStep     float64
	// CeilingMargin multiplies the predicted mean into a distribution ceiling:
	// the P90 of the leave-one-out error, so nine titles in ten fit under it.
	CeilingMargin float64
	// YDIFMin/YDIFMax bound the fitted range; a prediction outside it is an
	// extrapolation (clamped, lower confidence). MinFrames is the sampling
	// floor under which no prediction is made.
	YDIFMin, YDIFMax float64
	MinFrames        int
	// Calibration record.
	Titles, Scenes                  int
	R2LOO, ErrP50, ErrP90, ErrWorst float64
	Corpus                          string
}

// CostModelV1: 20 titles / 60 scenes of the library, libx264 veryfast with
// the production tuning, tonemapx on HDR sources, 2026-09-12.
var CostModelV1 = CostModel{
	Version:            1,
	Intercept:          -3.8748,
	Slope:              0.6915,
	RefHeight:          1080,
	RefCRF:             23,
	ResolutionExponent: 0.8,  // 2160p = 0.81x the bpp of 1080p, 720p 1.15x, 480p 1.42x
	CRFHalvingStep:     3.98, // x2.38 per 5 CRF down, x0.47 per 5 up
	CeilingMargin:      1.53,
	YDIFMin:            0.7,
	YDIFMax:            8.5,
	MinFrames:          100,
	Titles:             17,
	Scenes:             51,
	R2LOO:              0.631,
	ErrP50:             1.22,
	ErrP90:             1.53,
	ErrWorst:           1.90,
	Corpus:             "ladder-corpus2-2026-09-12",
}

// TitleCost is the model's answer for one title.
type TitleCost struct {
	ModelVersion int
	// YDIF is the motion the prediction was made from (clamped to the fitted
	// range); BPPRef the predicted bits per pixel per frame at the reference
	// point. Confidence is 1 inside the fitted range, 0.5 on an extrapolation.
	YDIF        float64
	BPPRef      float64
	Confidence  float64
	SampleCount int
	// Probed: BPPRef is the title's measured cost (plan P3) and Margin the
	// margin it carries instead of the model's generic P90.
	Probed bool
	Margin float64
}

// Predict answers for a title's persisted metrics; ok is false when the
// sampling is too thin to say anything (the class table then applies). A
// usable probe (see TitleProbe.Usable) overrides the prediction with the
// measured cost, brought back to the reference point.
func (m CostModel) Predict(metrics ComplexityMetrics) (TitleCost, bool) {
	if p := metrics.Probe; p.Usable(m.Version, StaticSoftwareCRF) {
		// The probe measured bits per pixel per frame at the policy CRF on the
		// reference height; the model's unit is the same at the reference CRF.
		bpp := p.MeasuredBPP / math.Pow(2, float64(m.RefCRF-StaticSoftwareCRF)/m.CRFHalvingStep)
		return TitleCost{ModelVersion: m.Version, YDIF: metrics.MotionMean, BPPRef: bpp, Confidence: 1, SampleCount: metrics.FramesSampled, Probed: true, Margin: p.Margin()}, true
	}
	if metrics.FramesSampled < m.MinFrames || metrics.MotionMean <= 0 {
		return TitleCost{}, false
	}
	c := TitleCost{ModelVersion: m.Version, YDIF: metrics.MotionMean, Confidence: 1, SampleCount: metrics.FramesSampled}
	if c.YDIF < m.YDIFMin || c.YDIF > m.YDIFMax {
		c.YDIF = math.Min(math.Max(c.YDIF, m.YDIFMin), m.YDIFMax)
		c.Confidence = 0.5
	}
	c.BPPRef = math.Exp(m.Intercept + m.Slope*math.Log(c.YDIF))
	return c, true
}

// RungKbps is the predicted mean cost of a rung encoded at crf, in kbps.
// fps <= 0 assumes film cadence.
func (m CostModel) RungKbps(c TitleCost, width, height int, fps float64, crf int) int64 {
	if fps <= 0 {
		fps = 24
	}
	refPx := float64(m.RefHeight) * float64(m.RefHeight) * 16 / 9
	px := float64(width) * float64(height)
	bpp := c.BPPRef * math.Pow(px/refPx, m.ResolutionExponent-1)
	bpp *= math.Pow(2, float64(m.RefCRF-crf)/m.CRFHalvingStep)
	return int64(math.Round(bpp * px * fps / 1000))
}

// CeilingKbps is RungKbps under the model's margin: what the rung advertises
// and what its encoder may not exceed.
func (m CostModel) CeilingKbps(c TitleCost, width, height int, fps float64, crf int) int64 {
	margin := m.CeilingMargin
	if c.Probed && c.Margin > 0 {
		margin = c.Margin
	}
	return int64(math.Round(float64(m.RungKbps(c, width, height, fps, crf)) * margin))
}

// PriceLadder replaces each rung's video bitrate with the model's ceiling at
// crf and flags the rung as cost-modelled (its distribution ceiling is then
// the advertised bitrate itself, no further headroom). The rung is priced at
// the frame it will really encode: the builders frame by width, so a 2.39:1
// source fills 1920x800 of the "1080p" rung, not 1920x1080. Never mutates
// ladder.
func (m CostModel) PriceLadder(ladder []HLSProfile, c TitleCost, src SourceFacts, crf int) []HLSProfile {
	out := make([]HLSProfile, len(ladder))
	for i, p := range ladder {
		h := p.Height
		if src.Width > 0 && src.Height > 0 {
			h = int(math.Round(float64(p.Width) * float64(src.Height) / float64(src.Width)))
		}
		p.VideoBitrate = m.CeilingKbps(c, p.Width, h, src.FPS, crf)
		p.CostModelled = true
		out[i] = p
	}
	return out
}

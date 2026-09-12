package transcoding

import (
	"math"
	"time"
)

// title_probe.go - per-title ground truth (ladder engine, plan P3). A probe
// encodes one scene of the title at the static policy's CRF on the CPU
// reference and compares the cost with the cost model's prediction. Simulated
// on the P1 corpus before any code: one scene confirms the prediction 82 % of
// the time (1.29 points per title on average), but a single scene does NOT
// beat the model on the titles it confirms (error x1.20 vs x1.18: it carries
// the x2.3 scene-to-scene spread of the title). The truth therefore only
// replaces the prediction where the prediction is OUT of its margin, once two
// or three scenes have been measured (worst case x2.03 -> x1.52), and never
// with less than a floor of margin.

// TitleProbeProtocol bumps when the probe's meaning changes.
const TitleProbeProtocol = 1

// TitleProbeEncoderID names the encoder + preset + tuning a probe measures on.
// It is part of a probe's reuse identity (see CurrentIdentity): a change that
// alters the encoded bytes - a different preset, a tuning change, or an ffmpeg
// build known to shift libx264's output - MUST bump this string, so every probe
// measured on the old encode is treated as stale on both the read path (the
// model ignores it) and the queue (the worker re-probes). The build that ran a
// probe is also recorded in TitleProbe.FFmpeg for audit, but the build id is a
// runtime value and is not consulted on the pure-domain decision path; bumping
// this constant is the lever for an output-changing build, exactly as
// TitleProbeProtocol is for a change of the probe's meaning.
const TitleProbeEncoderID = "libx264 veryfast (production tuning)"

// TitleProbeMaxScenes and TitleProbeMaxCRFPoints are the hard caps.
const (
	TitleProbeMaxScenes    = 3
	TitleProbeMaxCRFPoints = 2
)

// TruthMarginFloor is the least margin a probed title keeps over its
// measured cost: two or three scenes do not see the whole title.
const TruthMarginFloor = 1.25

// ProbePoint is one encode of one scene.
type ProbePoint struct {
	StartSec float64 `json:"start_sec"`
	CRF      int     `json:"crf"`
	Kbps     float64 `json:"kbps"`
	XPSNRY   float64 `json:"xpsnr_y"`
}

// TitleProbe is the persisted probe of one title, next to its metrics.
type TitleProbe struct {
	Protocol     int    `json:"protocol"`
	ModelVersion int    `json:"model_version"`
	FFmpeg       string `json:"ffmpeg"`
	Encoder      string `json:"encoder"`
	Height       int    `json:"height"`
	// PredictedKbps is what the model said the probe height at the probe CRF
	// would cost; MeasuredKbps the mean of the scenes at that CRF; Ratio
	// measured over predicted.
	PredictedKbps float64 `json:"predicted_kbps"`
	MeasuredKbps  float64 `json:"measured_kbps"`
	// MeasuredBPP is the measured cost in the model's unit: bits per pixel
	// per frame at the probe CRF, over the frame really encoded.
	MeasuredBPP float64      `json:"measured_bpp"`
	Ratio       float64      `json:"ratio"`
	Confirmed   bool         `json:"confirmed"`
	Points      []ProbePoint `json:"points"`
	// Spread is max/min of the scene costs at the probe CRF (1 = one scene).
	Spread     float64   `json:"spread"`
	MeasuredAt time.Time `json:"measured_at"`
	// Failed marks a probe that could not run (unreadable source...): the
	// title leaves the candidate set instead of being retried forever.
	Failed bool `json:"failed,omitempty"`
}

// Scenes counts the distinct scenes measured at the probe CRF.
func (p *TitleProbe) Scenes(crf int) int {
	seen := map[float64]bool{}
	for _, pt := range p.Points {
		if pt.CRF == crf {
			seen[pt.StartSec] = true
		}
	}
	return len(seen)
}

// Margin is the ceiling margin a probed title carries: half the observed
// spread (in log space), never under the floor.
func (p *TitleProbe) Margin() float64 {
	if p.Spread <= 1 {
		return TruthMarginFloor
	}
	return math.Max(TruthMarginFloor, math.Sqrt(p.Spread))
}

// CurrentIdentity reports whether the probe was produced by the encode this
// build would run now: same protocol, model version and encoder identity. A
// probe from an older identity is stale - the model ignores it (Usable) and the
// worker re-probes (ProbeCandidate) - so a protocol, model or encoder bump
// reopens every persisted probe consistently across both paths. Failed status
// is orthogonal: a failed probe at the CURRENT identity is terminal (it is not
// retried until an identity bump), which is why this check does not look at it.
func (p *TitleProbe) CurrentIdentity(modelVersion int) bool {
	return p != nil && p.Protocol == TitleProbeProtocol && p.ModelVersion == modelVersion && p.Encoder == TitleProbeEncoderID
}

// Usable reports a probe the model may read: the current identity, not failed,
// and - the measured rule - NOT confirmed with at least two scenes. A confirmed
// probe leaves the prediction alone (one scene is a worse estimate than the
// model); a probe that found the prediction out of its margin replaces it with
// what it measured.
func (p *TitleProbe) Usable(modelVersion, crf int) bool {
	return p.CurrentIdentity(modelVersion) && !p.Failed &&
		!p.Confirmed && p.Scenes(crf) >= 2 && p.MeasuredBPP > 0
}

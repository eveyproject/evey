package transcoding

import "strings"

// codec_efficiency.go - the CodecEfficiencyModel (ladder engine, plan P2).
// The cost model says what a title costs on the CPU reference (libx264
// veryfast, the production tuning, CRF 23). This model says what the SAME
// operating point costs on another codec, encoder and preset: the factor is
// the bitrate that encoder needs to reach the reference's XPSNR on the same
// scenes, over the reference's bitrate, measured by `evey admin ladder-bench
// --efficiency` and never assumed. No entry, or no confidence, means the
// planner keeps the class table for that route.

// EncoderKey names one calibrated encoder configuration at one range.
type EncoderKey struct {
	Codec  string
	Accel  HWAccelType // "" = CPU
	Preset string
	// HDR: an HDR passthrough operating point (Main10, HDR kept). Only the
	// SDR point is calibrated so far; an HDR key never matches.
	HDR bool
}

// EfficiencyEntry is one (encoder, rung height) calibration cell.
type EfficiencyEntry struct {
	// Height is the rung stratum the cell was measured at (0 = any height).
	Height int
	// Factor: bitrate needed over the reference's, at the reference fidelity.
	Factor float64
	// Confidence: 1 when the corpus was large and tight, 0.5 when thin, 0 = unusable.
	Confidence float64
	// Calibration record: scenes, titles, scenes the curve never reached the
	// reference fidelity on, dispersion of the per-scene factors around the
	// median and the leave-one-title-out error, all as factors.
	SampleCount, Titles, OutOfRange int
	DispP50, DispP90, DispWorst     float64
	LOOP50, LOOP90, LOOWorst        float64
	MedianFPS                       float64
}

// Margin is the uncertainty the planner carries into the rung's ceiling: the
// P90 of the per-scene dispersion, so nine scenes in ten of a title fit.
func (e EfficiencyEntry) Margin() float64 {
	if e.DispP90 < 1 {
		return 1
	}
	return e.DispP90
}

// CodecEfficiencyModel is one versioned calibration table.
type CodecEfficiencyModel struct {
	Version          int
	ReferenceEncoder string
	Corpus           string
	Host             string
	Entries          map[EncoderKey][]EfficiencyEntry
}

// ProductionPreset names the preset a route encodes with, the key the
// calibration was made under.
func ProductionPreset(accel HWAccelType) string {
	switch accel {
	case NVENC:
		return "p4"
	case VAAPI:
		return "driver"
	case QSV:
		return "veryfast"
	}
	return "veryfast"
}

// Lookup returns the usable cell for a route at a rung height: the stratum
// nearest to height, provided its confidence is above zero.
func (m CodecEfficiencyModel) Lookup(key EncoderKey, height int) (EfficiencyEntry, bool) {
	key.Codec = strings.ToLower(key.Codec)
	cells := m.Entries[key]
	best, found := EfficiencyEntry{}, false
	for _, c := range cells {
		if c.Confidence <= 0 || c.Factor <= 0 {
			continue
		}
		if !found || abs(c.Height-height) < abs(best.Height-height) {
			best, found = c, true
		}
	}
	return best, found
}

// PriceLadder converts a cost-priced ladder (the CPU reference ceilings) to
// this encoder's ceilings: factor and margin per rung, from the cell of its
// height. A rung with no cell keeps the reference price and is left
// un-flagged - the caller decides whether that is acceptable for the route.
func (m CodecEfficiencyModel) PriceLadder(ladder []HLSProfile, key EncoderKey) []HLSProfile {
	out := make([]HLSProfile, len(ladder))
	for i, p := range ladder {
		if e, ok := m.Lookup(key, p.Height); ok {
			p.VideoBitrate = int64(float64(p.VideoBitrate)*e.Factor*e.Margin() + 0.5)
			p.EfficiencyVersion = m.Version
		}
		out[i] = p
	}
	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// CodecEfficiencyModelV1 is filled from the ladder-bench efficiency corpus;
// an empty table keeps every non-reference route on the class table.
var CodecEfficiencyModelV1 = CodecEfficiencyModel{
	Version:          1,
	ReferenceEncoder: "libx264 veryfast (production tuning) crf 23, SDR",
	Entries:          map[EncoderKey][]EfficiencyEntry{},
}

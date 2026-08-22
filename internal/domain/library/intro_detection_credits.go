package library

// SuspiciousCreditsLeadMs is the floor of the end-credits plausibility bound: a
// credits marker starting farther than this from the end of the file is a
// detector mistake, not a credits roll. Real episode credits run ~30s-2min; the
// prod reports of a "skip the credits" prompt appearing mid-scene all sat past
// this line.
const SuspiciousCreditsLeadMs = 4 * 60 * 1000

// creditsLeadRuntimeFraction relaxes the floor for long runtimes: a feature film
// really can roll 12 minutes of credits, and the flat 4-minute floor would call
// those a mistake. 12% of a 2h10 film is 15min36 - long enough for any real
// credits roll, short enough that the 600s scan-bound false positives measured
// on 40-min episodes (bound: 4min) stay refused.
const creditsLeadRuntimeFraction = 0.12

// maxPlausibleCreditsLeadMs returns the largest distance from the end of a
// runtimeMs file at which an end-credits marker is still believable: the floor,
// or the fraction of the runtime whenever that is larger.
func maxPlausibleCreditsLeadMs(runtimeMs, floorMs int64) int64 {
	return max(floorMs, int64(float64(runtimeMs)*creditsLeadRuntimeFraction))
}

// CreditsLeadImplausible is the ONE answer to "do these credits start too early
// to be credits?", shared by the detector that emits them, the repair pass that
// re-runs it and the serving gate that withholds the stored ones. leadMs is the
// distance between the credits start and the end of the file - for a
// blackdetect row, whose EndMs is the file duration, that is exactly EndMs-StartMs.
// A floorMs of 0 or less means the caller disabled the bound.
func CreditsLeadImplausible(leadMs, runtimeMs, floorMs int64) bool {
	if floorMs <= 0 || runtimeMs <= 0 {
		return false
	}
	return leadMs > maxPlausibleCreditsLeadMs(runtimeMs, floorMs)
}

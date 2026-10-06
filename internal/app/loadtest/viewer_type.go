package loadtest

import (
	"strconv"
	"strings"
)

// viewerType is a simulated viewer's per-request behavior: whether it
// negotiates naturally and demands mkvgo-served responses, forces a
// transcode, or (ScenarioNatural) just negotiates naturally with no
// verification. It is independent of Config.Scenario for the mix scenarios,
// where each viewer slot gets its own type from a deterministic pattern.
type viewerType string

const (
	viewerTypeMkvgo     viewerType = "mkvgo"
	viewerTypeTranscode viewerType = "transcode"
	viewerTypeNatural   viewerType = "natural"
)

// Mix patterns are repeated over the 1-based ramp slot to give a
// deterministic, reproducible weighted split (e.g. light's 80/20 repeats
// [mkvgo,mkvgo,mkvgo,mkvgo,transcode]) rather than randomizing per run.
var (
	lightPattern    = []viewerType{viewerTypeMkvgo, viewerTypeMkvgo, viewerTypeMkvgo, viewerTypeMkvgo, viewerTypeTranscode}
	balancedPattern = []viewerType{viewerTypeMkvgo, viewerTypeTranscode}
	heavyPattern    = []viewerType{viewerTypeMkvgo, viewerTypeTranscode, viewerTypeTranscode, viewerTypeTranscode, viewerTypeTranscode}
)

// scenarioViewerType returns the viewer type for the given 1-based ramp
// slot under scenario. Pure scenarios return a constant type; mix scenarios
// cycle through their weighted pattern. A skipped mkvgo candidate retries
// the same slot (the run loop decrements tier before retrying), so the slot
// number - and therefore the type - never changes mid-retry.
func scenarioViewerType(scenario string, slot int) viewerType {
	switch scenario {
	case ScenarioMkvgo:
		return viewerTypeMkvgo
	case ScenarioTranscode:
		return viewerTypeTranscode
	case ScenarioLight:
		return lightPattern[(slot-1)%len(lightPattern)]
	case ScenarioBalanced:
		return balancedPattern[(slot-1)%len(balancedPattern)]
	case ScenarioHeavy:
		return heavyPattern[(slot-1)%len(heavyPattern)]
	default:
		// Start has already resolved a history mix and rejected a bad one.
		if spec, ok := strings.CutPrefix(scenario, ScenarioClients+":"); ok {
			if mix, err := parseClientMix(spec); err == nil {
				return clientViewerType(clientForSlot(mix, slot))
			}
			return viewerTypeNatural
		}
		if pinned, ok := parsePinnedScenario(scenario); ok {
			if slot <= pinned {
				return viewerTypeTranscode
			}
			return viewerTypeMkvgo
		}
		return viewerTypeNatural
	}
}

// parsePinnedScenario recognises the "mkvgo+N" family: N transcode viewers
// PINNED at the bottom of the ramp (the household's fixed tax - the one old
// TV that cannot direct-play), then a pure-mkvgo ramp on top. Unlike the
// ratio mixes (light/balanced/heavy) the transcode share does not grow with
// the ramp, so the ceiling answers "how many direct-play viewers fit ON TOP
// of K incompressible encodes". N is clamped to [1, 32].
func parsePinnedScenario(scenario string) (pinned int, ok bool) {
	rest, found := strings.CutPrefix(scenario, ScenarioMkvgo+"+")
	if !found {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n < 1 {
		return 0, false
	}
	if n > 32 {
		n = 32
	}
	return n, true
}

// scenarioSpawnsEncoders reports whether a scenario ever puts a viewer on the
// transcode path, i.e. whether the run will start an ffmpeg at all.
//
// It decides how long a tier must be held before it can be judged. An encoder
// pays a cold start (up to ~25s worst case) before it reaches steady state, so a
// run that starts one has to wait it out. A pure direct-play run never starts
// one: its viewers burst a few segments to fill their buffers and are then in
// steady state within seconds. Waiting an encoder's warm-up on a run that has no
// encoder is dead time - and across a bisection's several tiers, it is most of
// the run's wall clock.
//
// ScenarioNatural is treated as if it might: the server decides each viewer's
// path, and it may well pick a transcode.
func scenarioSpawnsEncoders(scenario string) bool {
	if scenario == ScenarioMkvgo {
		return false
	}
	if pinned, ok := parsePinnedScenario(scenario); ok {
		return pinned > 0
	}
	return true
}

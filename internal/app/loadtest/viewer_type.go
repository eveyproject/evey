package loadtest

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
		return viewerTypeNatural
	}
}

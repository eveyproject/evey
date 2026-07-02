package mkv

// ValidationSeverity classifies a streaming-readiness finding.
type ValidationSeverity string

const (
	// ValidationError is a defect that breaks seeking or on-demand serving
	// (missing Cues, a cue pointing at a non-video track, a missing
	// TimecodeScale). The file should be reindexed or remuxed.
	ValidationError ValidationSeverity = "error"
	// ValidationWarning is a non-fatal issue (a track without a language, no
	// BlockDuration on subtitle blocks) worth surfacing but not blocking.
	ValidationWarning ValidationSeverity = "warning"
)

// ValidationIssue is one streaming-readiness finding: what is wrong and how
// severe. The message names the fix (it comes straight from mkvgo's validate).
type ValidationIssue struct {
	Severity ValidationSeverity
	Message  string
}

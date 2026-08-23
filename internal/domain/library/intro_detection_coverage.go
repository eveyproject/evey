package library

import "slices"

// MinCoveringIntroMs is the shortest stored intro that counts as coverage. A
// shorter one is a degenerate detection (the studio-logo silence at 0:00, a
// pHash match that confirmed a sliver): the episode still needs a real answer,
// so it stays on the sweep's work list.
const MinCoveringIntroMs = 5_000

// MinDetectedIntroMs is the shortest intro a DETECTOR may have believed. It
// polices two points that must never disagree: the serving gate that withholds
// a stored row, and the detectors that decide whether a candidate is worth
// storing at all. Same doctrine as CreditsLeadImplausible - one answer to
// "too short to be an intro?", never two constants drifting apart.
//
// Bounded by production, not chosen by feel (2026-08-24, read-only dump of the
// 19 intros the server actually served): the three shortest are 5 000, 5 347
// and 10 000 ms and are exactly the three gold false positives; the next one
// up is 18 015 ms and is a true positive. The safe margin is [10 001, 18 014] ms.
//
// An mkv chapter is EXEMPT at the serving gate: a muxer wrote it, no detector
// guessed it. It keeps the absolute 5s product floor instead (user decision
// 2026-08-06).
const MinDetectedIntroMs = 12_000

// IntroLockedSources are the operator marks the sweep must never overwrite.
// An episode carrying one is settled whatever the span: the operator decided.
func IntroLockedSources() []string { return []string{"manual", "manual_propagated"} }

// IntroCoveringSources are the FILE-DERIVED sources whose intro, once it clears
// MinCoveringIntroMs, makes a re-detection pointless - it would re-run pHash and
// Chromaprint over the same file to land on the same answer.
//
// Excluded on purpose: the community DBs ("tidb"/"introdb") describe someone
// else's copy of the media and were measured tens of seconds off, and the
// last-resort heuristics ("audio", "blackdetect"). Those episodes stay eligible
// so a file-derived detection can replace what they carry.
func IntroCoveringSources() []string { return []string{"mkv_chapter", "audio_cluster", "phash"} }

// IntroDetectionSettled reports whether the intro segment an episode already
// carries takes it OFF the detection work list. It is the ONE answer to
// "does this episode still need its intro detected?", shared by the in-process
// season sweep and by the SQL that rebuilds the sweep's work list after a
// restart - the SQL is handed IntroLockedSources/IntroCoveringSources and
// MinCoveringIntroMs as parameters rather than repeating the lists, so the two
// cannot drift.
//
// An episode with NO intro row at all is never settled; callers handle that
// case before reaching here.
func IntroDetectionSettled(source string, spanMs int64) bool {
	if slices.Contains(IntroLockedSources(), source) {
		return true
	}
	return slices.Contains(IntroCoveringSources(), source) && spanMs >= MinCoveringIntroMs
}

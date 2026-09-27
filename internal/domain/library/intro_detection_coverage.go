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

// IntroVersionedSources are the covering sources a DETECTOR computed: their answer
// changes with the detection algorithm, so they only settle an episode when its last
// detection attempt ran at the current algorithm version. A bump re-detects them once.
// An mkv chapter was written by a muxer, not guessed: it stays settled across bumps.
func IntroVersionedSources() []string { return []string{"audio_cluster", "phash"} }

// IsCommunityIntroSource reports whether the segment came from a community
// database rather than from this server's own analysis of this file. Community
// timestamps describe SOMEONE ELSE'S copy (repack, edit, different cut), so
// their accuracy on our files is luck: the 2026-08-25 gold measure had one at
// IoU 1.00 (Spider noir) and one 232 seconds off (Resident alien) - same
// source, same day. User decision 2026-08-25: never SERVE them; the rows stay
// stored so the sweep can replace them with a file-derived detection and the
// eval can keep scoring them.
func IsCommunityIntroSource(source string) bool {
	return source == "tidb" || source == "introdb"
}

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
//
// detectedAtCurrentVersion reports whether the episode's last detection attempt ran at
// the current algorithm version; it only matters for IntroVersionedSources.
func IntroDetectionSettled(source string, spanMs int64, detectedAtCurrentVersion bool) bool {
	if slices.Contains(IntroLockedSources(), source) {
		return true
	}
	if !slices.Contains(IntroCoveringSources(), source) || spanMs < MinCoveringIntroMs {
		return false
	}
	return detectedAtCurrentVersion || !slices.Contains(IntroVersionedSources(), source)
}

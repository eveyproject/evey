package transcoding

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// withDefaults is the only thing standing between a partially-filled options
// struct (an admin panel that only set two knobs, a caller that forgot one) and
// a producer running with a zero read rate, a zero GC interval or a zero
// session cap. Every field is asserted individually: a default that silently
// stops applying is invisible until the box misbehaves under load.
func TestSoftwareProducerOptions_WithDefaults_FillsEveryZeroField(t *testing.T) {
	got := SoftwareProducerOptions{}.withDefaults()

	assert.InDelta(t, 1.5, got.ReadRate, 0.001, "the transcode pacing rate")
	assert.InDelta(t, ManifestSegmentSeconds, got.SegmentSeconds, 0.001)
	assert.Equal(t, 30*time.Minute, got.InactivityTimeout)
	assert.Equal(t, time.Minute, got.GCInterval)
	assert.Equal(t, 20*time.Second, got.PauseIdleThreshold)
	assert.Equal(t, 60*time.Second, got.WaitBudgetDur)
	assert.Equal(t, 2, got.MaxSessionsPerHash)
	assert.Equal(t, 2, got.MaxConcurrentVariantsPerHash)
	assert.Equal(t, 60*time.Second, got.NewbornEvictionShield)
	// The RULE, not a copy of the number: the window is whatever a paced
	// encoder can cross inside the client's patience. Freezing "16" here is
	// exactly what let the old value drift out of agreement with the pacing.
	assert.Equal(t, derivedSeekReachWindow(got.ReadRate, got.SegmentSeconds), got.SeekReachWindowSegs)
	assert.Equal(t, 8, got.ResumeCushionSegs)
	assert.Equal(t, 30, got.CopyLeadSegs)
	assert.Equal(t, 12, got.CopyKeepBehindSegs)
}

// KillPausedAfter is the documented exception: 0 means "disabled" and must STAY
// disabled. Giving it a fallback would silently start killing suspended
// encoders on every box that never asked for it.
func TestSoftwareProducerOptions_WithDefaults_LeavesKillPausedAfterDisabled(t *testing.T) {
	assert.Zero(t, SoftwareProducerOptions{}.withDefaults().KillPausedAfter,
		"0 means the paused-session reaper is off, not that it needs a default")

	explicit := SoftwareProducerOptions{KillPausedAfter: 5 * time.Minute}.withDefaults()
	assert.Equal(t, 5*time.Minute, explicit.KillPausedAfter, "an operator value must survive untouched")
}

// A negative value is not a value: it reaches the same fallback as zero. Left
// through, a negative GC interval panics time.NewTicker and a negative session
// cap disables the LRU bound entirely.
func TestSoftwareProducerOptions_WithDefaults_NegativesAlsoFallBack(t *testing.T) {
	got := SoftwareProducerOptions{
		ReadRate:                     -1,
		SegmentSeconds:               -1,
		InactivityTimeout:            -time.Second,
		GCInterval:                   -time.Second,
		PauseIdleThreshold:           -time.Second,
		WaitBudgetDur:                -time.Second,
		MaxSessionsPerHash:           -1,
		MaxConcurrentVariantsPerHash: -1,
		NewbornEvictionShield:        -time.Second,
		SeekReachWindowSegs:          -1,
		ResumeCushionSegs:            -1,
	}.withDefaults()

	assert.InDelta(t, 1.5, got.ReadRate, 0.001)
	assert.InDelta(t, ManifestSegmentSeconds, got.SegmentSeconds, 0.001)
	assert.Positive(t, got.InactivityTimeout)
	assert.Positive(t, got.GCInterval)
	assert.Positive(t, got.PauseIdleThreshold)
	assert.Positive(t, got.WaitBudgetDur)
	assert.Positive(t, got.MaxSessionsPerHash)
	assert.Positive(t, got.MaxConcurrentVariantsPerHash)
	assert.Positive(t, got.NewbornEvictionShield)
	assert.Positive(t, got.SeekReachWindowSegs)
	assert.Positive(t, got.ResumeCushionSegs)
}

// A caller that DID set a knob owns it - the defaults must never overwrite an
// operator's deliberate choice, including one that happens to sit below the
// default (a weak NAS deliberately shortening its GC interval).
func TestSoftwareProducerOptions_WithDefaults_KeepsCallerValues(t *testing.T) {
	in := SoftwareProducerOptions{
		ReadRate:                     8.0, // the copy-mode pacing
		SegmentSeconds:               6.0,
		InactivityTimeout:            90 * time.Second,
		GCInterval:                   5 * time.Second,
		PauseIdleThreshold:           2 * time.Second,
		WaitBudgetDur:                10 * time.Second,
		MaxSessionsPerHash:           1,
		MaxConcurrentVariantsPerHash: 1,
		NewbornEvictionShield:        3 * time.Second,
		SeekReachWindowSegs:          4,
		ResumeCushionSegs:            2,
		KillPausedAfter:              time.Minute,
		CopyLeadSegs:                 7,
		CopyKeepBehindSegs:           3,
	}

	assert.Equal(t, in, in.withDefaults(), "a fully specified options struct must come back unchanged")
}

// withDefaults returns a COPY: it must not mutate the caller's struct, which
// the producer keeps and re-reads.
func TestSoftwareProducerOptions_WithDefaults_DoesNotMutateTheInput(t *testing.T) {
	in := SoftwareProducerOptions{}
	_ = in.withDefaults()

	assert.Zero(t, in.ReadRate, "the caller's struct must be left alone")
	assert.Zero(t, in.MaxSessionsPerHash)
}

// The seek-reach window exists to answer one question: can a paced encoder
// reach this segment before the player hangs up? A fixed count cannot answer
// it, because the answer moves with the pacing. Prod, 2026-09-06: at the old
// fixed 16, a request could land 48 s of content ahead of the frontier and the
// server would make a 1.5x encoder crawl to it for 32 s - while the client
// abandoned the segment after ~8 s and retried (measured 10.026 s apart,
// `context canceled` server-side). The window promised a wait no client honours.
func TestDerivedSeekReachWindow_StaysInsideClientPatience(t *testing.T) {
	cases := []struct {
		name           string
		readRate       float64
		segmentSeconds float64
	}{
		{"transcode pacing", 1.5, ManifestSegmentSeconds},
		{"copy pacing", 8.0, ManifestSegmentSeconds},
		{"audio rendition pacing", 8.0, 6.0},
		{"slow box", 1.0, ManifestSegmentSeconds},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			segs := derivedSeekReachWindow(c.readRate, c.segmentSeconds)
			// Worst-case wall clock to cross the window at the steady rate.
			waitSec := float64(segs) * c.segmentSeconds / c.readRate
			assert.LessOrEqual(t, waitSec, clientSegmentPatience.Seconds(),
				"a request at the edge of the window must be servable before the client gives up")
			assert.GreaterOrEqual(t, segs, minSeekReachWindowSegs,
				"continuous playback draws 0-1 segments ahead and must never spawn a rival encoder")
		})
	}
}

// The old constant is the regression this replaces: at the default pacing the
// window must now be far below it, or nothing changed.
func TestDerivedSeekReachWindow_MuchTighterThanTheOldFixed16(t *testing.T) {
	segs := derivedSeekReachWindow(1.5, ManifestSegmentSeconds)
	assert.Less(t, segs, 16)
	assert.GreaterOrEqual(t, segs, minSeekReachWindowSegs)
}

// A faster producer legitimately reaches further in the same time - the window
// is about wall clock, not about segment counts.
func TestDerivedSeekReachWindow_GrowsWithTheProductionRate(t *testing.T) {
	slow := derivedSeekReachWindow(1.5, ManifestSegmentSeconds)
	fast := derivedSeekReachWindow(8.0, ManifestSegmentSeconds)
	assert.Greater(t, fast, slow)
}

// Garbage pacing must not produce a zero window: 0 would make EVERY request one
// segment ahead of the frontier spawn a rival encoder, i.e. a spawn storm on
// ordinary continuous playback.
func TestDerivedSeekReachWindow_NeverZero(t *testing.T) {
	assert.GreaterOrEqual(t, derivedSeekReachWindow(0, 0), minSeekReachWindowSegs)
	assert.GreaterOrEqual(t, derivedSeekReachWindow(-1, -1), minSeekReachWindowSegs)
	assert.GreaterOrEqual(t, derivedSeekReachWindow(0.01, 3), minSeekReachWindowSegs)
}

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
	assert.Equal(t, 16, got.SeekReachWindowSegs)
	assert.Equal(t, 8, got.ResumeCushionSegs)
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

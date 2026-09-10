package transcoding

import (
	"testing"
	"time"
)

// orchestrator_options.go had no test file, and withDefaults is the function
// that turns a zero-valued OrchestratorOptions into a working configuration -
// which is exactly how most callers construct it.
//
// Every guard here is `if x <= 0 { x = default }`. Mutating it to `< 0` leaves a
// ZERO value untouched, i.e. the default never applies: a look-ahead of 0
// segments, a GC that never ticks, a zero-second segment duration. Twelve such
// mutants survived, one per field.
func TestOrchestratorOptions_WithDefaults_FillsEveryZeroField(t *testing.T) {
	got := (&OrchestratorOptions{}).withDefaults()

	if got.LookAheadSegments != 30 {
		t.Errorf("LookAheadSegments = %d, want 30", got.LookAheadSegments)
	}
	if got.InactivityTimeout != 60*time.Second {
		t.Errorf("InactivityTimeout = %v, want 60s", got.InactivityTimeout)
	}
	if got.GCInterval != 15*time.Second {
		t.Errorf("GCInterval = %v, want 15s", got.GCInterval)
	}
	if got.SegmentSeconds != ManifestSegmentSeconds {
		t.Errorf("SegmentSeconds = %v, want %v", got.SegmentSeconds, ManifestSegmentSeconds)
	}
	if got.ThrottlePauseAhead != 10 {
		t.Errorf("ThrottlePauseAhead = %d, want 10", got.ThrottlePauseAhead)
	}
	if got.ThrottleResumeBelow != 5 {
		t.Errorf("ThrottleResumeBelow = %d, want 5", got.ThrottleResumeBelow)
	}
	if got.ThrottleTickInterval != time.Second {
		t.Errorf("ThrottleTickInterval = %v, want 1s", got.ThrottleTickInterval)
	}
	if got.MaxRafalesPerHash != 3 {
		t.Errorf("MaxRafalesPerHash = %d, want 3", got.MaxRafalesPerHash)
	}
	if got.HWFirstSegmentTimeout != 10*time.Second {
		t.Errorf("HWFirstSegmentTimeout = %v, want 10s", got.HWFirstSegmentTimeout)
	}
	if got.WaitBudgetDur != 25*time.Second {
		t.Errorf("WaitBudgetDur = %v, want 25s", got.WaitBudgetDur)
	}
}

// A negative value is as unusable as a zero one and must also fall back - the
// guard is `<= 0`, not `== 0`.
func TestOrchestratorOptions_WithDefaults_NegativesAlsoFallBack(t *testing.T) {
	got := (&OrchestratorOptions{
		LookAheadSegments:     -1,
		InactivityTimeout:     -time.Second,
		GCInterval:            -time.Second,
		SegmentSeconds:        -1,
		ThrottlePauseAhead:    -1,
		ThrottleResumeBelow:   -1,
		ThrottleTickInterval:  -time.Second,
		MaxRafalesPerHash:     -1,
		HWFirstSegmentTimeout: -time.Second,
		WaitBudgetDur:         -time.Second,
	}).withDefaults()

	if got.LookAheadSegments != 30 || got.GCInterval != 15*time.Second ||
		got.SegmentSeconds != ManifestSegmentSeconds || got.MaxRafalesPerHash != 3 ||
		got.WaitBudgetDur != 25*time.Second {
		t.Errorf("negative values must fall back to defaults, got %+v", got)
	}
}

// Caller-supplied values are preserved, one field at a time - otherwise a guard
// could be "always apply the default" and the test above would still pass.
func TestOrchestratorOptions_WithDefaults_KeepsCallerValues(t *testing.T) {
	got := (&OrchestratorOptions{
		LookAheadSegments:     7,
		InactivityTimeout:     90 * time.Second,
		GCInterval:            5 * time.Second,
		SegmentSeconds:        6,
		ThrottlePauseAhead:    20,
		ThrottleResumeBelow:   4,
		ThrottleTickInterval:  2 * time.Second,
		MaxRafalesPerHash:     9,
		HWFirstSegmentTimeout: 11 * time.Second,
		WaitBudgetDur:         42 * time.Second,
	}).withDefaults()

	if got.LookAheadSegments != 7 {
		t.Errorf("LookAheadSegments = %d, want the caller's 7", got.LookAheadSegments)
	}
	if got.InactivityTimeout != 90*time.Second {
		t.Errorf("InactivityTimeout = %v, want 90s", got.InactivityTimeout)
	}
	if got.GCInterval != 5*time.Second {
		t.Errorf("GCInterval = %v, want 5s", got.GCInterval)
	}
	if got.SegmentSeconds != 6 {
		t.Errorf("SegmentSeconds = %v, want 6", got.SegmentSeconds)
	}
	if got.ThrottlePauseAhead != 20 || got.ThrottleResumeBelow != 4 {
		t.Errorf("throttle thresholds = %d/%d, want 20/4", got.ThrottlePauseAhead, got.ThrottleResumeBelow)
	}
	if got.ThrottleTickInterval != 2*time.Second {
		t.Errorf("ThrottleTickInterval = %v, want 2s", got.ThrottleTickInterval)
	}
	if got.MaxRafalesPerHash != 9 {
		t.Errorf("MaxRafalesPerHash = %d, want 9", got.MaxRafalesPerHash)
	}
	if got.HWFirstSegmentTimeout != 11*time.Second {
		t.Errorf("HWFirstSegmentTimeout = %v, want 11s", got.HWFirstSegmentTimeout)
	}
	if got.WaitBudgetDur != 42*time.Second {
		t.Errorf("WaitBudgetDur = %v, want 42s", got.WaitBudgetDur)
	}
}

// The hysteresis invariant: the resume threshold must stay STRICTLY below the
// pause threshold, or a session flaps between paused and running every tick.
// The repair halves the pause threshold.
func TestOrchestratorOptions_WithDefaults_HysteresisInvariant(t *testing.T) {
	// Equal thresholds would flap: repaired.
	got := (&OrchestratorOptions{ThrottlePauseAhead: 10, ThrottleResumeBelow: 10}).withDefaults()
	if got.ThrottleResumeBelow >= got.ThrottlePauseAhead {
		t.Errorf("resume %d must be strictly below pause %d", got.ThrottleResumeBelow, got.ThrottlePauseAhead)
	}
	if got.ThrottleResumeBelow != 5 {
		t.Errorf("the repair halves the pause threshold: got %d, want 5", got.ThrottleResumeBelow)
	}

	// A resume ABOVE pause is the same illness.
	got = (&OrchestratorOptions{ThrottlePauseAhead: 8, ThrottleResumeBelow: 20}).withDefaults()
	if got.ThrottleResumeBelow >= got.ThrottlePauseAhead {
		t.Errorf("resume %d must be strictly below pause %d", got.ThrottleResumeBelow, got.ThrottlePauseAhead)
	}

	// A already-valid pair is left alone.
	got = (&OrchestratorOptions{ThrottlePauseAhead: 12, ThrottleResumeBelow: 3}).withDefaults()
	if got.ThrottleResumeBelow != 3 || got.ThrottlePauseAhead != 12 {
		t.Errorf("a valid pair must not be rewritten, got %d/%d", got.ThrottlePauseAhead, got.ThrottleResumeBelow)
	}
}

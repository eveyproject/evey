package transcoding

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// fakeProducer records the RemoveSession / Close fan-out and reports a fixed
// active-session count, so the selector's aggregation contracts can be checked
// without real ffmpeg sessions.
type fakeProducer struct {
	name    string
	active  int
	removed []string
	closed  bool
}

func (f *fakeProducer) Profiles([]transcoding.HLSProfile, SourceDims, transcoding.PlaybackDecision, transcoding.EncodeCapability) []transcoding.HLSProfile {
	return nil
}
func (f *fakeProducer) EnsureSegmentRange(context.Context, DecisionFingerprint, int, int) error {
	return nil
}
func (f *fakeProducer) PreWarm(DecisionFingerprint, int, int)        {}
func (f *fakeProducer) WaitBudget() time.Duration                    { return time.Second }
func (f *fakeProducer) NotifyClientHint(string, int, int, bool, int) {}
func (f *fakeProducer) ActiveSessionCount() int                      { return f.active }
func (f *fakeProducer) RemoveSession(mediaItemID string)             { f.removed = append(f.removed, mediaItemID) }
func (f *fakeProducer) Close()                                       { f.closed = true }

// The selector evaluates no gate: the resolved route's side picks the
// producer (ProducerForPlan), and a route on the GPU with no hardware
// producer wired falls to software rather than to nil.
func TestProducerSelector_ForRoute_FollowsTheRoute(t *testing.T) {
	sw := &fakeProducer{name: "sw"}
	hw := &fakeProducer{name: "hw"}
	sel := NewProducerSelector(sw, hw)
	if got := sel.forRoute(true); got != StreamProducer(hw) {
		t.Fatalf("a GPU route serves on the hardware producer, got %v", got)
	}
	if got := sel.forRoute(false); got != StreamProducer(sw) {
		t.Fatalf("a CPU route serves on the software producer, got %v", got)
	}
	if got := NewProducerSelector(sw, nil).forRoute(true); got != StreamProducer(sw) {
		t.Fatalf("no hardware producer wired: software, got %v", got)
	}
}

// RemoveSession is the contract that fixes the StopStream asymmetry: it must
// reach BOTH producers so a stopped software stream is torn down, not just the
// hardware orchestrator's sessions.
func TestProducerSelector_RemoveSession_FansOutToBoth(t *testing.T) {
	sw := &fakeProducer{name: "sw"}
	hw := &fakeProducer{name: "hw"}
	sel := NewProducerSelector(sw, hw)

	sel.RemoveSession("mi-1")

	if len(sw.removed) != 1 || sw.removed[0] != "mi-1" {
		t.Fatalf("software producer must receive RemoveSession, got %v", sw.removed)
	}
	if len(hw.removed) != 1 || hw.removed[0] != "mi-1" {
		t.Fatalf("hardware producer must receive RemoveSession, got %v", hw.removed)
	}
}

func TestProducerSelector_ActiveSessionCount_SumsBoth(t *testing.T) {
	sw := &fakeProducer{name: "sw", active: 3}
	hw := &fakeProducer{name: "hw", active: 2}
	sel := NewProducerSelector(sw, hw)
	if got := sel.ActiveSessionCount(); got != 5 {
		t.Fatalf("ActiveSessionCount must sum both producers, got %d", got)
	}
}

// THE bug the guards had: both producers only know about ffmpeg sessions, so a
// playback served entirely by mkvgo (or by direct play) read as an idle server
// and every background worker - drain, trickplay, intro detection, calibration
// - kept competing for the NAS disk under a live viewer.
func TestProducerSelector_ActivePlaybackCount_CountsNonFFmpegPlayback(t *testing.T) {
	sw := &fakeProducer{name: "sw", active: 0}
	hw := &fakeProducer{name: "hw", active: 0}
	sel := NewProducerSelector(sw, hw)

	act := NewPlaybackActivity(time.Minute)
	sel.WithPlaybackActivity(act)
	act.Touch("mi-1")

	if got := sel.ActivePlaybackCount(); got != 1 {
		t.Fatalf("a non-ffmpeg playback must count as active, got %d", got)
	}
}

func TestProducerSelector_ActivePlaybackCount_SumsWithProducers(t *testing.T) {
	sw := &fakeProducer{name: "sw", active: 3}
	hw := &fakeProducer{name: "hw", active: 2}
	sel := NewProducerSelector(sw, hw)

	act := NewPlaybackActivity(time.Minute)
	sel.WithPlaybackActivity(act)
	act.Touch("mi-1")

	if got := sel.ActivePlaybackCount(); got != 6 {
		t.Fatalf("non-ffmpeg playback must add to the ffmpeg counts, got %d", got)
	}
}

// The two counts answer DIFFERENT questions and must not be conflated. This is
// the regression guard: ActiveSessionCount feeds the admin dashboard's
// "active transcodes" and the evey_active_hls_sessions gauge, so folding a
// direct-play or an mkvgo copy into it would report encoding work that is not
// happening.
func TestProducerSelector_ActiveSessionCount_ExcludesNonFFmpegPlayback(t *testing.T) {
	sw := &fakeProducer{name: "sw", active: 2}
	hw := &fakeProducer{name: "hw", active: 0}
	sel := NewProducerSelector(sw, hw)

	act := NewPlaybackActivity(time.Minute)
	sel.WithPlaybackActivity(act)
	act.Touch("mi-1")
	act.Touch("mi-2")

	if got := sel.ActiveSessionCount(); got != 2 {
		t.Fatalf("ActiveSessionCount must stay ffmpeg-only, got %d", got)
	}
	if got := sel.ActivePlaybackCount(); got != 4 {
		t.Fatalf("ActivePlaybackCount must see everything, got %d", got)
	}
}

// Unwired tracker: the wider count must degrade to the ffmpeg count, never
// panic - that is the "feature not wired" contract the test handlers rely on.
func TestProducerSelector_ActivePlaybackCount_NilTrackerEqualsSessions(t *testing.T) {
	sel := NewProducerSelector(&fakeProducer{active: 3}, &fakeProducer{active: 1})
	if got := sel.ActivePlaybackCount(); got != 4 {
		t.Fatalf("nil tracker must degrade to the ffmpeg count, got %d", got)
	}
}

func TestProducerSelector_Close_ClosesBoth(t *testing.T) {
	sw := &fakeProducer{name: "sw"}
	hw := &fakeProducer{name: "hw"}
	NewProducerSelector(sw, hw).Close()
	if !sw.closed || !hw.closed {
		t.Fatalf("Close must close both producers (sw=%v hw=%v)", sw.closed, hw.closed)
	}
}

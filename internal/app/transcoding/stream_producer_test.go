package transcoding

import (
	"context"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/transcoding"
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
func (f *fakeProducer) PreWarm(DecisionFingerprint, int, int)   {}
func (f *fakeProducer) WaitBudget() time.Duration               { return time.Second }
func (f *fakeProducer) NotifyClientHint(string, int, bool, int) {}
func (f *fakeProducer) ActiveSessionCount() int                 { return f.active }
func (f *fakeProducer) RemoveSession(mediaItemID string)        { f.removed = append(f.removed, mediaItemID) }
func (f *fakeProducer) Close()                                  { f.closed = true }

func TestProducerSelector_For_RoutesByPredicate(t *testing.T) {
	sw := &fakeProducer{name: "sw"}
	hw := &fakeProducer{name: "hw"}

	hwUsable := true
	sel := NewProducerSelector(sw, hw, func(transcoding.PlaybackDecision, SourceDims) bool { return hwUsable })

	if got := sel.For(transcoding.PlaybackDecision{}, SourceDims{}); got != StreamProducer(hw) {
		t.Fatalf("predicate true must route to hardware, got %v", got)
	}
	hwUsable = false
	if got := sel.For(transcoding.PlaybackDecision{}, SourceDims{}); got != StreamProducer(sw) {
		t.Fatalf("predicate false must route to software, got %v", got)
	}
}

// A nil predicate (the fan-out remover instance) must default to software for
// For() and never panic - routing is unused on that instance.
func TestProducerSelector_For_NilPredicate_DefaultsSoftware(t *testing.T) {
	sw := &fakeProducer{name: "sw"}
	hw := &fakeProducer{name: "hw"}
	sel := NewProducerSelector(sw, hw, nil)
	if got := sel.For(transcoding.PlaybackDecision{}, SourceDims{}); got != StreamProducer(sw) {
		t.Fatalf("nil predicate must default to software, got %v", got)
	}
}

// RemoveSession is the contract that fixes the StopStream asymmetry: it must
// reach BOTH producers so a stopped software stream is torn down, not just the
// hardware orchestrator's sessions.
func TestProducerSelector_RemoveSession_FansOutToBoth(t *testing.T) {
	sw := &fakeProducer{name: "sw"}
	hw := &fakeProducer{name: "hw"}
	sel := NewProducerSelector(sw, hw, nil)

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
	sel := NewProducerSelector(sw, hw, nil)
	if got := sel.ActiveSessionCount(); got != 5 {
		t.Fatalf("ActiveSessionCount must sum both producers, got %d", got)
	}
}

func TestProducerSelector_Close_ClosesBoth(t *testing.T) {
	sw := &fakeProducer{name: "sw"}
	hw := &fakeProducer{name: "hw"}
	NewProducerSelector(sw, hw, nil).Close()
	if !sw.closed || !hw.closed {
		t.Fatalf("Close must close both producers (sw=%v hw=%v)", sw.closed, hw.closed)
	}
}

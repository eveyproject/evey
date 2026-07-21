package transcoding

import (
	"sync"
	"time"
)

// ondemand_activity.go - the "someone is watching" signal for the mkvgo
// on-demand path.
//
// Every anti-starvation guard in the server (keyframe drain suspension, level
// backfill yield, trickplay scheduling, intro/complexity SIGSTOP, calibration
// idle-gate, load-test gate) reads ONE number: ActiveSessionCount. Both of its
// sources count live ffmpeg processes - and the on-demand packager spawns none.
// It serves playlists, inits and segments straight from mkvgo, stateless, with
// no session bookkeeping by design.
//
// The consequence was a silent hole rather than a wrong number: a viewer served
// entirely by mkvgo left the counter at 0, so every background worker kept
// hammering the same disk the viewer was streaming from. On a weak NAS that is
// exactly the starvation the guards exist to prevent.
//
// Rather than give the stateless path a session lifecycle (there is no "stop"
// event to hang one on - the client simply stops asking), activity is inferred
// from the requests themselves and decays on its own.

// OnDemandActivityTTL is how long one request keeps its media counted as
// playing. It must outlast the quiet gap between two segment fetches: a player
// buffers many segments ahead in a burst, then goes silent until the buffer
// drains, so the gap is set by the buffer depth (tens of seconds), not by the
// 6 s segment duration. One minute clears that with margin.
//
// The asymmetry is deliberate. Too short and a steady viewer flaps in and out
// of "active", which is the bug being fixed. Too long and background work stays
// paused for up to a minute after the last viewer leaves - work that is, by
// definition, background. Erring toward protecting playback is the cheap side.
const OnDemandActivityTTL = time.Minute

// OnDemandActivity counts media currently being served by the on-demand
// packager, on a sliding TTL window fed by the request path itself.
//
// The unit is the MEDIA, not the request: one viewer scrubbing a file fires
// hundreds of segment requests and is still one playback. Counting requests
// would over-report load by orders of magnitude and suspend background work far
// beyond the real pressure.
//
// The zero value is not usable; a nil *OnDemandActivity is, and reads as zero -
// that is the "no on-demand path wired" case.
type OnDemandActivity struct {
	mu   sync.Mutex
	ttl  time.Duration
	last map[string]time.Time

	// now is the clock seam: the TTL semantics are the whole contract here, and
	// they are untestable against a real clock without sleeping.
	now func() time.Time
}

// NewOnDemandActivity builds a tracker whose entries expire ttl after their
// last touch. Pass OnDemandActivityTTL unless a caller has a reason not to.
func NewOnDemandActivity(ttl time.Duration) *OnDemandActivity {
	return &OnDemandActivity{
		ttl:  ttl,
		last: make(map[string]time.Time),
		now:  time.Now,
	}
}

// Touch records that mediaItemID was just served on the on-demand path, and
// restarts its TTL window. Called from the HTTP segment path, so it stays a map
// write under a mutex and nothing more.
func (a *OnDemandActivity) Touch(mediaItemID string) {
	if a == nil || mediaItemID == "" {
		return
	}
	a.mu.Lock()
	a.last[mediaItemID] = a.now()
	a.mu.Unlock()
}

// ActiveSessionCount reports how many distinct media were served on the
// on-demand path within the TTL window, matching the StreamProducer method
// name so ProducerSelector can sum it alongside the ffmpeg counts.
//
// It also reclaims expired entries: this map is fed by every segment request of
// every viewer, so read-through-only expiry would grow it by one entry per
// media ever played until restart. The sweep is O(live media) and runs on the
// worker-poll path, not the request path.
func (a *OnDemandActivity) ActiveSessionCount() int {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	cutoff := a.now().Add(-a.ttl)
	for id, seen := range a.last {
		if seen.Before(cutoff) {
			delete(a.last, id)
		}
	}
	return len(a.last)
}

// size exposes the retained entry count for the leak test. Test-only: the
// production contract is ActiveSessionCount.
func (a *OnDemandActivity) size() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.last)
}

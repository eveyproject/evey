package transcoding

import (
	"sync"
	"time"
)

// playback_activity.go - the "someone is watching" signal for every serving
// path that spawns no ffmpeg process.
//
// Every anti-starvation guard in the server (keyframe drain suspension, level
// backfill yield, trickplay scheduling, intro/complexity SIGSTOP, calibration
// idle-gate, load-test gate) reads ONE number: ActiveSessionCount. Its two
// original sources both count live ffmpeg processes - and two whole serving
// paths spawn none:
//
//   - the mkvgo on-demand packager, which builds playlists, inits and segments
//     per request, stateless, with no session bookkeeping by design;
//   - direct play, which is a bare http.ServeFile off the source file.
//
// The consequence was a silent hole rather than a wrong number: a viewer on
// either path left the counter at 0, so every background worker kept hammering
// the same disk the viewer was streaming from. On a weak NAS that is exactly
// the starvation the guards exist to prevent.
//
// Rather than give these stateless paths a session lifecycle (there is no
// "stop" event to hang one on - the client simply stops asking), activity is
// inferred from the requests themselves, in the two shapes they actually take.

// PlaybackActivityTTL is how long one completed request keeps its media counted
// as playing. It must outlast the quiet gap between two requests: a player
// buffers many segments ahead in a burst, then goes silent until the buffer
// drains, so the gap is set by the buffer depth (tens of seconds), not by the
// 6 s segment duration. One minute clears that with margin.
//
// The asymmetry is deliberate. Too short and a steady viewer flaps in and out
// of "active", which is the bug being fixed. Too long and background work stays
// paused for up to a minute after the last viewer leaves - work that is, by
// definition, background. Erring toward protecting playback is the cheap side.
const PlaybackActivityTTL = time.Minute

// mediaActivity is one media's activity state. Both fields are needed because
// the two serving paths have opposite traffic shapes:
//
//   - on-demand HLS is MANY SHORT requests, so the media is active between them
//     and only lastSeen can express that;
//   - direct play is ONE LONG request that can block for the whole film, so a
//     TTL measured from its start would expire mid-playback and only an
//     in-flight count can express that.
//
// A media is active when EITHER holds, which covers both shapes and their
// mixtures (a direct-play client issuing many short range requests).
type mediaActivity struct {
	lastSeen time.Time
	inFlight int
}

// PlaybackActivity counts media currently being served by a non-ffmpeg path.
//
// The unit is the MEDIA, not the request: one viewer scrubbing a file fires
// hundreds of segment requests, and two players on one file are still one file
// being read off the disk. Counting requests would over-report load by orders
// of magnitude and suspend background work far beyond the real pressure.
//
// The zero value is not usable; a nil *PlaybackActivity is, and reads as zero -
// that is the "not wired" case.
type PlaybackActivity struct {
	mu    sync.Mutex
	ttl   time.Duration
	media map[string]*mediaActivity

	// now is the clock seam: the TTL semantics are the whole contract here, and
	// they are untestable against a real clock without sleeping.
	now func() time.Time
}

// NewPlaybackActivity builds a tracker whose entries expire ttl after their
// last request ends. Pass PlaybackActivityTTL unless a caller has a reason not
// to.
func NewPlaybackActivity(ttl time.Duration) *PlaybackActivity {
	return &PlaybackActivity{
		ttl:   ttl,
		media: make(map[string]*mediaActivity),
		now:   time.Now,
	}
}

// Touch records that mediaItemID was just served, and restarts its TTL window.
// The right call for a SHORT request that is already finished (or about to be)
// by the time it matters - an on-demand segment, playlist or init.
func (a *PlaybackActivity) Touch(mediaItemID string) {
	if a == nil || mediaItemID == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entry(mediaItemID).lastSeen = a.now()
}

// Begin marks the start of a LONG-LIVED request and returns its release func,
// meant to be deferred. The media counts as active for the whole time the
// request runs, however long that is, and then for one TTL window after it ends
// so a client making back-to-back range requests doesn't flap.
//
// The release is idempotent: a double call (a defer plus a manual one, or a
// retry path) must not push the count negative or leave a media stuck active.
func (a *PlaybackActivity) Begin(mediaItemID string) func() {
	if a == nil || mediaItemID == "" {
		return func() {}
	}
	a.mu.Lock()
	e := a.entry(mediaItemID)
	e.inFlight++
	e.lastSeen = a.now()
	a.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			a.mu.Lock()
			defer a.mu.Unlock()
			cur, ok := a.media[mediaItemID]
			if !ok {
				return
			}
			if cur.inFlight > 0 {
				cur.inFlight--
			}
			// Stamp on the way OUT too: the TTL window that follows a long
			// request must start when it ended, not when it began.
			cur.lastSeen = a.now()
		})
	}
}

// ActiveSessionCount reports how many distinct media are currently being served
// on a non-ffmpeg path, matching the StreamProducer method name so
// ProducerSelector can sum it alongside the ffmpeg counts.
//
// It also reclaims settled entries: this map is fed by every request of every
// viewer, so read-through-only expiry would grow it by one entry per media ever
// played until restart. An entry with a request still in flight is never
// reclaimed, however stale its stamp - that request's release still needs it.
// The sweep is O(live media) and runs on the worker-poll path, not the request
// path.
func (a *PlaybackActivity) ActiveSessionCount() int {
	if a == nil {
		return 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	cutoff := a.now().Add(-a.ttl)
	n := 0
	for id, e := range a.media {
		switch {
		case e.inFlight > 0:
			n++
		case e.lastSeen.Before(cutoff):
			delete(a.media, id)
		default:
			n++
		}
	}
	return n
}

// Forget drops these media from the activity count at once, instead of letting
// their TTL run out. A media with a request still in flight is kept: someone
// is reading it right now, whoever asked to forget it. The load test calls it
// at its end so its own simulated viewers stop counting as an audience.
func (a *PlaybackActivity) Forget(mediaItemIDs ...string) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, id := range mediaItemIDs {
		if e, ok := a.media[id]; ok && e.inFlight == 0 {
			delete(a.media, id)
		}
	}
}

// entry returns the media's state, creating it on first use. Caller holds mu.
func (a *PlaybackActivity) entry(mediaItemID string) *mediaActivity {
	e, ok := a.media[mediaItemID]
	if !ok {
		e = &mediaActivity{}
		a.media[mediaItemID] = e
	}
	return e
}

// size exposes the retained entry count for the leak test. Test-only: the
// production contract is ActiveSessionCount.
func (a *PlaybackActivity) size() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.media)
}

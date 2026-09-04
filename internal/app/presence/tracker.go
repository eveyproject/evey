// Package presence is an in-memory, real-time registry of who is currently
// using the server. It is deliberately NOT persisted: presence is ephemeral and
// cheap to rebuild from the next heartbeat, and keeping it out of the DB avoids
// a write on the request hot path. The web client beats periodically while its
// tab is visible; the admin panel reads Online() to show "who is connected now".
package presence

import (
	"sort"
	"sync"
	"time"
)

// DefaultWindow is how long after a user's last heartbeat they still count as
// online. The web client beats every 30s, so 90s absorbs a couple of missed
// beats (a backgrounded/throttled tab, a transient network blip) before the
// user drops off the list.
const DefaultWindow = 90 * time.Second

// CastReceiverUserAgent is the product user agent a beaconing Chromecast is
// filed under. The receiver's own CrKey string would render as a desktop Chrome
// on Linux, so the beacon handler substitutes this, and the admin panel keys the
// TV chip - and the "stop this receiver" control - off it.
//
// One constant rather than the literal in each place: the string is the only
// thing that says "this online device is a television", and three copies of it
// is three chances for the control to silently stop appearing.
const CastReceiverUserAgent = "EveyCast (Chromecast)"

// IsCastReceiver reports whether an online entry is a beaconing Chromecast.
func IsCastReceiver(userAgent string) bool { return userAgent == CastReceiverUserAgent }

// Entry is one online DEVICE's live presence snapshot: the same account on the
// web app and the mobile app is two entries. The latest heartbeat for a given
// (user, user agent) wins, so a profile switch on one device updates in place.
type Entry struct {
	UserID    string
	Username  string
	ProfileID string
	IsAdmin   bool
	ClientIP  string
	UserAgent string
	LastSeen  time.Time
}

// Tracker is a concurrency-safe presence registry keyed by (user id, user
// agent). Keying by user alone made the row of an admin browsing the web
// panel while testing the mobile app flap between the two devices every beat
// (2026-07-26); one row per device is also what the future TV app needs.
type Tracker struct {
	mu     sync.Mutex
	byKey  map[string]Entry
	window time.Duration
	nowFn  func() time.Time
}

// NewTracker builds a tracker with the default online window.
func NewTracker() *Tracker {
	return &Tracker{byKey: make(map[string]Entry), window: DefaultWindow, nowFn: time.Now}
}

// entryKey separates devices of one account by user agent - stable for a
// given app/browser session, no client cooperation needed. The NUL separator
// cannot appear in either part.
func entryKey(e Entry) string {
	return e.UserID + "\x00" + e.UserAgent
}

// Touch records that e.UserID was just seen on e.UserAgent, refreshing that
// device's snapshot. A blank user id is ignored (unauthenticated callers
// never appear).
func (t *Tracker) Touch(e Entry) {
	if e.UserID == "" {
		return
	}
	t.mu.Lock()
	e.LastSeen = t.nowFn()
	t.byKey[entryKey(e)] = e
	t.mu.Unlock()
}

// Online returns every device seen within the window, most-recently-seen
// first, and lazily drops any that has since gone stale.
func (t *Tracker) Online() []Entry {
	cutoff := t.nowFn().Add(-t.window)
	t.mu.Lock()
	out := make([]Entry, 0, len(t.byKey))
	for key, e := range t.byKey {
		if e.LastSeen.Before(cutoff) {
			delete(t.byKey, key)
			continue
		}
		out = append(out, e)
	}
	t.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen.After(out[j].LastSeen) })
	return out
}

// Count returns the number of currently-online distinct USERS (and GCs stale
// entries): "1 user on two devices" still counts as one.
func (t *Tracker) Count() int {
	users := make(map[string]struct{})
	for _, e := range t.Online() {
		users[e.UserID] = struct{}{}
	}
	return len(users)
}

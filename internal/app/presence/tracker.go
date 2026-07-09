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

// Entry is one online user's live presence snapshot. The latest heartbeat wins,
// so a user who switches profile or device shows their current one.
type Entry struct {
	UserID    string
	Username  string
	ProfileID string
	IsAdmin   bool
	ClientIP  string
	UserAgent string
	LastSeen  time.Time
}

// Tracker is a concurrency-safe presence registry keyed by user id.
type Tracker struct {
	mu     sync.Mutex
	byUser map[string]Entry
	window time.Duration
	nowFn  func() time.Time
}

// NewTracker builds a tracker with the default online window.
func NewTracker() *Tracker {
	return &Tracker{byUser: make(map[string]Entry), window: DefaultWindow, nowFn: time.Now}
}

// Touch records that e.UserID was just seen, refreshing their snapshot. A blank
// user id is ignored (unauthenticated callers never appear).
func (t *Tracker) Touch(e Entry) {
	if e.UserID == "" {
		return
	}
	t.mu.Lock()
	e.LastSeen = t.nowFn()
	t.byUser[e.UserID] = e
	t.mu.Unlock()
}

// Online returns everyone seen within the window, most-recently-seen first, and
// lazily drops anyone who has since gone stale.
func (t *Tracker) Online() []Entry {
	cutoff := t.nowFn().Add(-t.window)
	t.mu.Lock()
	out := make([]Entry, 0, len(t.byUser))
	for id, e := range t.byUser {
		if e.LastSeen.Before(cutoff) {
			delete(t.byUser, id)
			continue
		}
		out = append(out, e)
	}
	t.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen.After(out[j].LastSeen) })
	return out
}

// Count returns the number of currently-online users (and GCs stale entries).
func (t *Tracker) Count() int {
	return len(t.Online())
}

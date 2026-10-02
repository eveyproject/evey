package music

import (
	"context"
	"sync"
	"time"
)

// nowPlayingTTL is how long a music session survives without a heartbeat before
// it drops off "En cours de lecture". The client heartbeats every ~10s, so this
// tolerates a couple of missed beats (a tab backgrounded, a network blip) yet
// clears a session whose client crashed or navigated away.
const nowPlayingTTL = 30 * time.Second

// NowPlayingSession is one live music playback, held only in memory: "currently
// playing" is ephemeral state, not history (that is music_track_plays). Enough
// to render the admin card and route a remote command back to the player.
type NowPlayingSession struct {
	SessionID string
	ProfileID string
	// Username is the ACCOUNT name the heartbeat authenticated as. It used to
	// be stored (and shown) as the profile name, which is a different thing:
	// the profile's own name is resolved from ProfileID by whoever renders the
	// session, since this registry has no profile repository.
	Username   string
	TrackID    string
	DeviceName string
	PositionMs int64
	IsPaused   bool
	StartedAt  time.Time
	LastSeen   time.Time
}

// NowPlayingRegistry tracks live music sessions in memory and carries a pending
// remote command per session (play/pause/stop). A player parked in WaitCommand
// gets it at once; otherwise it rides back on the next heartbeat, the only
// channel older clients know. Safe for concurrent use.
type NowPlayingRegistry struct {
	mu       sync.Mutex
	ttl      time.Duration
	now      func() time.Time
	sessions map[string]*nowPlayingEntry
	closed   bool
}

type nowPlayingEntry struct {
	sess    NowPlayingSession
	pending string // queued remote command, consumed by whichever reader comes first
	// wake is closed and replaced whenever a command is queued or the session
	// ends, releasing every WaitCommand parked on it.
	wake chan struct{}
}

func (e *nowPlayingEntry) signal() {
	close(e.wake)
	e.wake = make(chan struct{})
}

func (e *nowPlayingEntry) take() string {
	cmd := e.pending
	e.pending = ""
	return cmd
}

// NewNowPlayingRegistry builds an empty registry.
func NewNowPlayingRegistry() *NowPlayingRegistry {
	return &NowPlayingRegistry{
		ttl:      nowPlayingTTL,
		now:      time.Now,
		sessions: make(map[string]*nowPlayingEntry),
	}
}

// Heartbeat upserts the session's live state and returns (then clears) any
// pending remote command for it. Called on each player heartbeat.
func (r *NowPlayingRegistry) Heartbeat(sessionID, profileID, username, trackID, deviceName string, positionMs int64, isPaused bool) string {
	if sessionID == "" {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	e, ok := r.sessions[sessionID]
	if !ok {
		e = &nowPlayingEntry{sess: NowPlayingSession{SessionID: sessionID, StartedAt: now}, wake: make(chan struct{})}
		r.sessions[sessionID] = e
	}
	e.sess.ProfileID = profileID
	e.sess.Username = username
	e.sess.TrackID = trackID
	e.sess.DeviceName = deviceName
	e.sess.PositionMs = positionMs
	e.sess.IsPaused = isPaused
	e.sess.LastSeen = now
	return e.take()
}

// SetCommand queues a remote command for a live session. Reports whether the
// session exists (so the admin handler can 404 an unknown/expired one).
func (r *NowPlayingRegistry) SetCommand(sessionID, command string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.sessions[sessionID]
	if !ok || r.expiredLocked(e, r.now()) {
		return false
	}
	e.pending = command
	e.signal()
	return true
}

// WaitCommand parks until a command is queued for the session, the session
// ends, the registry closes or ctx is done, and returns the command ("" when
// woken without one). ok=false means the caller has no live session under that
// id - unknown, expired or another profile's - so there is nothing to wait on.
func (r *NowPlayingRegistry) WaitCommand(ctx context.Context, sessionID, profileID string) (cmd string, ok bool) {
	r.mu.Lock()
	e, found := r.sessions[sessionID]
	if r.closed || !found || e.sess.ProfileID != profileID || r.expiredLocked(e, r.now()) {
		r.mu.Unlock()
		return "", false
	}
	if cmd := e.take(); cmd != "" {
		r.mu.Unlock()
		return cmd, true
	}
	wake := e.wake
	r.mu.Unlock()

	select {
	case <-wake:
	case <-ctx.Done():
		return "", true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if cur, live := r.sessions[sessionID]; live && cur == e {
		return e.take(), true
	}
	return "", true
}

// Close releases every parked WaitCommand and refuses new ones, so a server
// shutdown is not held open by long polls.
func (r *NowPlayingRegistry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.closed = true
	for _, e := range r.sessions {
		e.signal()
	}
}

// Remove drops a session outright (explicit stop / track ended with nothing
// next). A missing id is a no-op.
func (r *NowPlayingRegistry) Remove(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.sessions[sessionID]; ok {
		e.signal()
		delete(r.sessions, sessionID)
	}
}

// List returns the live (non-expired) sessions, evicting stale ones as it goes.
func (r *NowPlayingRegistry) List() []NowPlayingSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	out := make([]NowPlayingSession, 0, len(r.sessions))
	for id, e := range r.sessions {
		if r.expiredLocked(e, now) {
			e.signal()
			delete(r.sessions, id)
			continue
		}
		out = append(out, e.sess)
	}
	return out
}

func (r *NowPlayingRegistry) expiredLocked(e *nowPlayingEntry, now time.Time) bool {
	return now.Sub(e.sess.LastSeen) > r.ttl
}

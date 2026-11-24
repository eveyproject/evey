package playback

import (
	"context"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/playback"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// The served path and the played state of a session are written by their two
// witnesses only (domain/playback/served.go): the HTTP layer reports every
// response that carried media bytes (RecordServed), the player reports what it
// decodes (RecordPlayed). A segment goes out every few seconds per leg; the
// witness keeps the last observation of each live session in memory and
// writes the row when it changes, plus once per witnessRefresh so the "seen
// at" time stays current while nothing changes.

const (
	// witnessRefresh rewrites an unchanged observation, so a session still
	// being fed reads as fed.
	witnessRefresh = 30 * time.Second
	// witnessMissTTL is how long a (profile, media) with no live session is
	// remembered as such: bytes requested without a session (a prewarm, a
	// player that has not called StartStream yet) do not cost a query each.
	witnessMissTTL = 5 * time.Second
)

type witnessKey struct{ profileID, mediaID string }

type witnessEntry struct {
	sessionID   string
	served      playback.Served
	servedAt    time.Time // last write of served
	played      *playback.Played
	playedAt    time.Time // last write of played
	missedUntil time.Time // no live session until then
}

type witness struct {
	mu      sync.Mutex
	entries map[witnessKey]*witnessEntry
}

func (w *witness) forget(profileID, mediaID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.entries, witnessKey{profileID, mediaID})
}

// entry returns the live entry of (profile, media), loading the session on a
// first sight. nil when no live session matches. Called with w.mu held; the
// lookup runs under it, which serializes the first sight of a session with
// its concurrent legs (the video and audio segments of one playback).
func (s *CommandService) witnessEntry(ctx context.Context, k witnessKey, now time.Time) (*witnessEntry, error) {
	w := &s.witness
	if w.entries == nil {
		w.entries = make(map[witnessKey]*witnessEntry)
	}
	if e, ok := w.entries[k]; ok {
		if e.sessionID != "" {
			return e, nil
		}
		if now.Before(e.missedUntil) {
			return nil, nil
		}
	}
	sess, err := s.activeSessionFor(ctx, k.profileID, k.mediaID)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		w.entries[k] = &witnessEntry{missedUntil: now.Add(witnessMissTTL)}
		return nil, nil
	}
	e := &witnessEntry{sessionID: sess.ID, served: sess.Served, played: sess.Played}
	w.entries[k] = e
	return e, nil
}

// RecordServed is the server's witness: a response carried bytes of one leg
// of the (profile, media) playback, described by l. Only a change of what is
// served (or the refresh) reaches the row.
func (s *CommandService) RecordServed(ctx context.Context, profileID, mediaID string, leg playback.Leg, l playback.ServedLeg) error {
	if profileID == "" || mediaID == "" {
		return nil
	}
	now := time.Now()
	if l.At.IsZero() {
		l.At = now
	}
	w := &s.witness
	w.mu.Lock()
	defer w.mu.Unlock()
	k := witnessKey{profileID, mediaID}
	e, err := s.witnessEntry(ctx, k, now)
	if err != nil || e == nil {
		return err
	}
	next := e.served
	cur := next.Audio
	if leg == playback.LegVideo {
		cur = next.Video
	}
	// A response that began before the one already recorded is older news,
	// whatever order the two finished in.
	if cur != nil && l.At.Before(cur.At) {
		return nil
	}
	if cur != nil && cur.SameBytes(l) && now.Sub(e.servedAt) < witnessRefresh {
		return nil
	}
	if leg == playback.LegVideo {
		next.Video = &l
	} else {
		next.Audio = &l
	}
	ctx, span := tracing.StartSpan(ctx, "app.playback.RecordServed")
	defer span.End()
	ok, err := s.sessions.SetServed(ctx, e.sessionID, next)
	if err != nil {
		return err
	}
	if !ok {
		// The session ended under us: look again at the next bytes.
		delete(w.entries, k)
		return nil
	}
	e.served, e.servedAt = next, now
	return nil
}

// RecordPlayed is the player's witness: what it reports it decodes.
func (s *CommandService) RecordPlayed(ctx context.Context, profileID, mediaID string, p playback.Played) error {
	if profileID == "" || mediaID == "" {
		return nil
	}
	now := time.Now()
	if p.At.IsZero() {
		p.At = now
	}
	w := &s.witness
	w.mu.Lock()
	defer w.mu.Unlock()
	k := witnessKey{profileID, mediaID}
	e, err := s.witnessEntry(ctx, k, now)
	if err != nil || e == nil {
		return err
	}
	if e.played != nil && e.played.SameState(p) && now.Sub(e.playedAt) < witnessRefresh {
		return nil
	}
	ctx, span := tracing.StartSpan(ctx, "app.playback.RecordPlayed")
	defer span.End()
	ok, err := s.sessions.SetPlayed(ctx, e.sessionID, p)
	if err != nil {
		return err
	}
	if !ok {
		delete(w.entries, k)
		return nil
	}
	e.played, e.playedAt = &p, now
	return nil
}

// Heartbeat is a player's progress tick carrying its witness: the tick is
// recorded (ReportProgressBySession), the played report too when the player
// sent one, and the answer is what the session is served - so the player's
// own panel reads the same served path the admin pages do.
func (s *CommandService) Heartbeat(ctx context.Context, sessionID string, positionMs int64, isPaused bool, castDeviceName string, played *playback.Played) (playback.Served, error) {
	session, err := s.reportProgress(ctx, sessionID, positionMs, isPaused, castDeviceName)
	if err != nil {
		return playback.Served{}, err
	}
	profileID, mediaID := string(session.ProfileID), string(session.MediaItemID)
	if played != nil {
		if err := s.RecordPlayed(ctx, profileID, mediaID, *played); err != nil {
			return playback.Served{}, err
		}
	}
	w := &s.witness
	w.mu.Lock()
	defer w.mu.Unlock()
	if e, ok := w.entries[witnessKey{profileID, mediaID}]; ok && e.sessionID == session.ID {
		return e.served, nil
	}
	return session.Served, nil
}

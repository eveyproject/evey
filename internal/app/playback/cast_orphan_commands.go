package playback

import (
	"sync"
	"time"
)

// cast_orphan_commands.go - reaching a Chromecast that is still beating but has
// no play session left.
//
// The admin's remote control (pause / play / stop / message) rides the
// session: SendSessionCommand writes onto a row, and the receiver's alive
// beacon reads it back. That covers a cast being watched. It does not cover the
// case this exists for.
//
// A receiver keeps beating for as long as its page is loaded - the heartbeat is
// a bare setInterval, and a television that is never really switched off never
// unloads it. Its session, meanwhile, ends the moment the picture stops moving
// (see TouchCastSession). So the state an operator actually meets is: a TV
// listed in "online devices", beating every 30s, with nothing in now-playing to
// click on. Before this, the only way to release it was to physically unplug
// the thing.
//
// Keyed by PROFILE, not by session or media: a profile is all the online-devices
// panel knows, and it is what the receiver's stream token carries, so the two
// ends meet without inventing an identifier for a session that no longer exists.

// castOrphanCommandTTL bounds how long a parked command waits.
//
// Long enough to cross several 30s beats (a receiver mid-stall can be slow to
// answer), short enough that a command nobody collected does not fire at a
// television hours later, when whoever sent it has long forgotten. A stop that
// arrives out of nowhere the next evening is worse than one that never arrives.
const castOrphanCommandTTL = 5 * time.Minute

type orphanCommand struct {
	command string
	setAt   time.Time
}

// castOrphanCommands parks one pending command per profile. In memory on
// purpose: the receiver it targets is itself a live, unpersisted thing, and a
// command that outlived a server restart would be a surprise, not a feature.
type castOrphanCommands struct {
	mu  sync.Mutex
	cmd map[string]orphanCommand
	now func() time.Time // swappable in tests
}

func newCastOrphanCommands() *castOrphanCommands {
	return &castOrphanCommands{cmd: make(map[string]orphanCommand), now: time.Now}
}

// set parks a command for the profile, replacing any earlier one. Replacing
// rather than queueing is deliberate: an operator clicking stop after pause
// means stop, not both in order.
func (c *castOrphanCommands) set(profileID, command string) {
	if profileID == "" || command == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cmd[profileID] = orphanCommand{command: command, setAt: c.now()}
	c.sweepLocked()
}

// take returns and removes the profile's pending command, or "" when there is
// none or it has expired. Destructive, like the session path: the first beacon
// to ask gets it.
func (c *castOrphanCommands) take(profileID string) string {
	if profileID == "" {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sweepLocked()
	entry, ok := c.cmd[profileID]
	if !ok {
		return ""
	}
	delete(c.cmd, profileID)
	return entry.command
}

// sweepLocked drops expired entries. Called from both paths rather than on a
// ticker: the map holds at most one entry per profile that an admin has
// explicitly acted on, so there is nothing here worth a goroutine.
func (c *castOrphanCommands) sweepLocked() {
	cutoff := c.now().Add(-castOrphanCommandTTL)
	for id, entry := range c.cmd {
		if entry.setAt.Before(cutoff) {
			delete(c.cmd, id)
		}
	}
}

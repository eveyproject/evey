package playback

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Reaching a Chromecast that is still beating but has no session left. The
// session-bound remote control has no row to write on, and the television is
// not gone - it is in the online-devices list, beating every 30s, holding the
// screen.

func TestCastOrphanCommands_ParksAndHandsOverOnce(t *testing.T) {
	t.Parallel()
	c := newCastOrphanCommands()
	c.set("prof-1", "stop")

	assert.Equal(t, "stop", c.take("prof-1"))
	assert.Empty(t, c.take("prof-1"), "destructive read - the first beacon to ask gets it")
}

func TestCastOrphanCommands_IsScopedToItsProfile(t *testing.T) {
	t.Parallel()
	c := newCastOrphanCommands()
	c.set("prof-1", "stop")

	assert.Empty(t, c.take("prof-OTHER"), "another household's television must not be stopped")
	assert.Equal(t, "stop", c.take("prof-1"))
}

// An operator clicking stop after pause means stop, not both in order.
func TestCastOrphanCommands_LastCommandWins(t *testing.T) {
	t.Parallel()
	c := newCastOrphanCommands()
	c.set("prof-1", "pause")
	c.set("prof-1", "stop")

	assert.Equal(t, "stop", c.take("prof-1"))
	assert.Empty(t, c.take("prof-1"))
}

// A command nobody collected must not fire at a television hours later, when
// whoever sent it has long forgotten it.
func TestCastOrphanCommands_ExpiresUncollected(t *testing.T) {
	t.Parallel()
	c := newCastOrphanCommands()
	now := time.Now()
	c.now = func() time.Time { return now }
	c.set("prof-1", "stop")

	now = now.Add(castOrphanCommandTTL - time.Second)
	assert.Equal(t, "stop", c.take("prof-1"), "still inside the window")

	c.set("prof-1", "stop")
	now = now.Add(castOrphanCommandTTL + time.Second)
	assert.Empty(t, c.take("prof-1"), "past the window it must be forgotten")
}

func TestCastOrphanCommands_IgnoresEmptyInput(t *testing.T) {
	t.Parallel()
	c := newCastOrphanCommands()
	c.set("", "stop")
	c.set("prof-1", "")
	assert.Empty(t, c.take("prof-1"))
	assert.Empty(t, c.take(""))
}

package postgres

import (
	"bytes"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// lockedBuffer makes the slog handler's writes and the test's reads safe under
// -race: the heartbeat goroutine writes while the test polls.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) snapshot() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf.Bytes()...)
}

// TestMigrationHeartbeat_BeatsWhileRunningAndStopsOnStop pins the visibility
// contract added after the 2026-08-24 incident: the migration runner has no
// context, so on a database that never answers the boot used to hang with zero
// log output. The heartbeat is that missing signal - it must fire while the
// run is in flight and fall silent once stopped.
func TestMigrationHeartbeat_BeatsWhileRunningAndStopsOnStop(t *testing.T) {
	var buf lockedBuffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	prevInterval := migrationHeartbeatInterval
	migrationHeartbeatInterval = 10 * time.Millisecond
	t.Cleanup(func() { migrationHeartbeatInterval = prevInterval })

	stop := migrationHeartbeat()
	require.Eventually(t, func() bool {
		return bytes.Contains(buf.snapshot(), []byte("migrations: still running"))
	}, 2*time.Second, 5*time.Millisecond,
		"the heartbeat must log while the migration run is in flight - a silent hang is what it exists to prevent")

	stop()
	// A beat already selected when stop landed may still be writing: give it
	// one interval to land, then require silence over several more.
	time.Sleep(30 * time.Millisecond)
	settled := buf.snapshot()
	time.Sleep(60 * time.Millisecond)
	require.Equal(t, settled, buf.snapshot(), "no beats after stop - the goroutine must exit with the run")
}

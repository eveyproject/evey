package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
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

// hangingConnector blocks every connection attempt: the "database accepts the
// TCP connection and then never answers" incident shape.
type hangingConnector struct{ block chan struct{} }

func (h *hangingConnector) Connect(context.Context) (driver.Conn, error) {
	<-h.block
	return nil, errors.New("released")
}
func (h *hangingConnector) Driver() driver.Driver { return nil }

// TestRunMigrations_FailsLoudlyWhenTheDatabaseNeverAnswers pins the bound
// added after the 2026-08-24 incident: golang-migrate has no context, so a
// database that hangs used to hold the whole boot forever. The run must come
// back as an ERROR within the budget instead.
func TestRunMigrations_FailsLoudlyWhenTheDatabaseNeverAnswers(t *testing.T) {
	prevBudget := migrationRunBudget
	migrationRunBudget = 100 * time.Millisecond
	t.Cleanup(func() { migrationRunBudget = prevBudget })

	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	db := sql.OpenDB(&hangingConnector{block: block})
	t.Cleanup(func() { _ = db.Close() })

	start := time.Now()
	err := RunMigrations(db, BackupOpts{})
	require.Error(t, err, "a database that never answers must fail the boot, not hold it")
	require.Contains(t, err.Error(), "not finished after",
		"the error must name the hang so the operator knows what happened")
	require.Less(t, time.Since(start), 5*time.Second, "the bound must actually bound")
}

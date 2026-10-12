package postgres_test

import (
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/stretchr/testify/require"
)

// CI 2026-07-11: one leaked transaction somewhere in the suite held an
// ACCESS SHARE lock, and the next test's truncateAll sat on its TRUNCATE for
// five minutes until the package timeout produced a goroutine dump naming
// the victim, not the culprit. The harness must instead bound the wait, name
// the open transactions (pid + query), terminate them and recover.
func TestTruncateAll_NamesAndKillsLeakedTransactions(t *testing.T) {
	old := testhelpers.LockTimeout
	testhelpers.LockTimeout = 2 * time.Second
	defer func() { testhelpers.LockTimeout = old }()

	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	// Simulate the bug class: a transaction left open with a lock on a
	// table. ACCESS SHARE is the weakest lock - anything a stray SELECT
	// holds - and it already conflicts with TRUNCATE's ACCESS EXCLUSIVE.
	tx, err := db.Begin()
	require.NoError(t, err)
	_, err = tx.Exec("LOCK TABLE users IN ACCESS SHARE MODE")
	require.NoError(t, err)
	// no Commit, no Rollback: leaked.

	start := time.Now()
	_, cleanup2 := testhelpers.StartPostgres(t) // truncates again: must self-heal, not hang
	defer cleanup2()
	require.Less(t, time.Since(start), 30*time.Second,
		"truncateAll must recover from a leaked transaction well before the package timeout")

	// The leaked backend was terminated, proving recovery came from the
	// guard and not from luck.
	require.Error(t, tx.Commit(), "the leaked transaction's backend must have been terminated")
}

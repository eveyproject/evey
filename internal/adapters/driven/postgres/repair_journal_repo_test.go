package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	appmaintenance "github.com/eveyproject/evey/internal/app/maintenance"
	"github.com/eveyproject/evey/internal/domain/activity"
)

// The repair journal is the only way back after a rewrite: an entry must
// come back exactly as written (shift map, evidence, outcome), a file's
// history must follow the PATH even after its media id changed, and the
// library-wide listing must honour the window and the batch kind.
func TestRepairJournalRepo_RoundTripHistoryAndList(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()
	audit := postgres.NewAuditLogRepository(db)
	repo := postgres.NewRepairJournalRepository(audit)

	start := time.Date(2026, 9, 16, 1, 0, 0, 0, time.UTC)
	first := appmaintenance.RepairRecord{
		Schema: appmaintenance.RepairJournalSchema, MediaID: "mi-old", Path: "/media/a.mkv", Op: appmaintenance.RepairAudioRetime, Job: "audio_shift",
		Started: start, SizeBefore: 1000, SizeAfter: 1000, ShiftSec: map[uint64]float64{2: -0.25},
		AudibleBefore: 3, AudibleAfter: 3, Evidence: appmaintenance.RepairEvidence{Method: "operator", Detail: "judged on a preview clip"},
		Outcome: "applied",
	}
	require.NoError(t, repo.RecordRepair(ctx, first))
	// The same file, re-added under a new media id, reindexed later.
	second := appmaintenance.RepairRecord{
		Schema: appmaintenance.RepairJournalSchema, MediaID: "mi-new", Path: "/media/a.mkv", Op: appmaintenance.RepairReindex, Job: "reindex",
		Started: start.Add(time.Hour), Outcome: "failed", Error: "read-only file system",
	}
	require.NoError(t, repo.RecordRepair(ctx, second))
	other := appmaintenance.RepairRecord{
		Schema: appmaintenance.RepairJournalSchema, MediaID: "mi-b", Path: "/media/b.mkv", Op: appmaintenance.RepairReindex, Job: "reindex",
		Started: start.Add(2 * time.Hour), Outcome: "applied",
	}
	require.NoError(t, repo.RecordRepair(ctx, other))

	history, err := repo.RepairHistory(ctx, "/media/a.mkv", 10)
	require.NoError(t, err)
	require.Len(t, history, 2, "both ids of the same path")
	assert.Equal(t, "mi-new", history[0].MediaID, "newest first")
	assert.Equal(t, "read-only file system", history[0].Error)
	got := history[1]
	assert.Equal(t, first.Op, got.Op)
	assert.Equal(t, first.ShiftSec, got.ShiftSec)
	assert.Equal(t, first.Evidence, got.Evidence)
	assert.Equal(t, first.AudibleAfter, got.AudibleAfter)
	assert.True(t, got.Started.Equal(first.Started))
	assert.True(t, got.Reversible(), "a retime is replayable backwards")
	assert.False(t, history[0].Reversible(), "a failed reindex is not")

	none, err := repo.RepairHistory(ctx, "/media/never.mkv", 10)
	require.NoError(t, err)
	assert.Empty(t, none)

	all, err := repo.ListRepairs(ctx, appmaintenance.RepairQuery{})
	require.NoError(t, err)
	assert.Len(t, all, 3)
	reindexes, err := repo.ListRepairs(ctx, appmaintenance.RepairQuery{Job: "reindex"})
	require.NoError(t, err)
	assert.Len(t, reindexes, 2)
	limited, err := repo.ListRepairs(ctx, appmaintenance.RepairQuery{Limit: 1})
	require.NoError(t, err)
	assert.Len(t, limited, 1)
	// The window filters on the row's own created_at (now), not on Started.
	past, err := repo.ListRepairs(ctx, appmaintenance.RepairQuery{Until: time.Now().Add(-time.Hour)})
	require.NoError(t, err)
	assert.Empty(t, past)

	// The rows sit in the admin audit log under one action, keyed by media id.
	action := postgres.RepairAuditAction
	entries, err := audit.List(ctx, activity.AuditLogFilter{Action: &action, Limit: 10})
	require.NoError(t, err)
	assert.Len(t, entries, 3)
	for _, e := range entries {
		assert.Equal(t, postgres.RepairAuditResource, e.ResourceType)
		require.NotNil(t, e.ResourceID)
	}
}

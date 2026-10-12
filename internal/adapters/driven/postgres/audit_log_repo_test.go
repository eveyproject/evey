package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptrStr(s string) *string { return &s }

func TestAuditLogRepo_CreateAndList(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	repo := postgres.NewAuditLogRepository(db)
	ctx := context.Background()

	ua := "Mozilla/5.0"
	dn := "iPhone"
	e := &activity.AuditLogEntry{
		ID:           "al-1",
		UserID:       "user-1",
		Action:       "login",
		ResourceType: "session",
		Details:      map[string]any{"method": "local"},
		ClientIP:     ptrStr("192.168.1.1"),
		UserAgent:    &ua,
		DeviceName:   &dn,
		CreatedAt:    time.Now().UTC().Truncate(time.Millisecond),
	}

	require.NoError(t, repo.Create(ctx, e))

	userID := "user-1"
	entries, err := repo.List(ctx, activity.AuditLogFilter{UserID: &userID, Limit: 10})
	require.NoError(t, err)
	require.Len(t, entries, 1)

	got := entries[0]
	assert.Equal(t, "login", got.Action)
	require.NotNil(t, got.UserAgent)
	assert.Equal(t, ua, *got.UserAgent)
	require.NotNil(t, got.DeviceName)
	assert.Equal(t, dn, *got.DeviceName)
}

// An audit trail that only names the ACCOUNT cannot tell two profiles of one
// household apart. The profile is persisted with the entry and its name comes
// back from the JOIN, and the filter narrows to it.
func TestAuditLogRepo_ProfileTrail(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	repo := postgres.NewAuditLogRepository(db)
	ctx := context.Background()

	u := newTestUser("alu1")
	require.NoError(t, userRepo.Create(ctx, u))
	main := newTestProfile("alp1", u.ID)
	main.Name = "Papa"
	require.NoError(t, profileRepo.Create(ctx, main))
	kids := newTestProfile("alp2", u.ID)
	kids.Name = "Les enfants"
	require.NoError(t, profileRepo.Create(ctx, kids))

	now := time.Now().UTC().Truncate(time.Millisecond)
	require.NoError(t, repo.Create(ctx, &activity.AuditLogEntry{
		ID: "al-p1", UserID: u.ID, ProfileID: main.ID,
		Action: "page_view", ResourceType: "page", CreatedAt: now,
	}))
	require.NoError(t, repo.Create(ctx, &activity.AuditLogEntry{
		ID: "al-p2", UserID: u.ID, ProfileID: kids.ID,
		Action: "page_view", ResourceType: "page", CreatedAt: now.Add(-time.Minute),
	}))
	// A server-side action has no profile at all; it must still be storable.
	require.NoError(t, repo.Create(ctx, &activity.AuditLogEntry{
		ID: "al-p3", UserID: u.ID,
		Action: "backup", ResourceType: "system", CreatedAt: now.Add(-2 * time.Minute),
	}))

	userID := string(u.ID)
	all, err := repo.List(ctx, activity.AuditLogFilter{UserID: &userID, Limit: 10})
	require.NoError(t, err)
	require.Len(t, all, 3)
	byID := map[string]*activity.AuditLogEntry{}
	for _, e := range all {
		byID[e.ID] = e
	}
	assert.Equal(t, "Papa", byID["al-p1"].ProfileName)
	assert.Equal(t, main.ID, byID["al-p1"].ProfileID)
	assert.Equal(t, "Les enfants", byID["al-p2"].ProfileName)
	assert.Empty(t, byID["al-p3"].ProfileID)
	assert.Empty(t, byID["al-p3"].ProfileName)

	profileID := string(kids.ID)
	only, err := repo.List(ctx, activity.AuditLogFilter{ProfileID: &profileID, Limit: 10})
	require.NoError(t, err)
	require.Len(t, only, 1)
	assert.Equal(t, "al-p2", only[0].ID)

	n, err := repo.Count(ctx, activity.AuditLogFilter{ProfileID: &profileID})
	require.NoError(t, err)
	assert.Equal(t, 1, n)
}

// Retention: Purge drops entries older than the cutoff and keeps the rest -
// the operator's recent trail must never go with the old one.
func TestAuditLogRepo_Purge(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	repo := postgres.NewAuditLogRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	for id, at := range map[string]time.Time{
		"al-old-1": now.Add(-100 * 24 * time.Hour),
		"al-old-2": now.Add(-40 * 24 * time.Hour),
		"al-new":   now.Add(-time.Hour),
	} {
		require.NoError(t, repo.Create(ctx, &activity.AuditLogEntry{
			ID: id, UserID: "user-p", Action: "login", ResourceType: "session", CreatedAt: at,
		}))
	}

	n, err := repo.Purge(ctx, now.Add(-30*24*time.Hour))
	require.NoError(t, err)
	assert.EqualValues(t, 2, n)

	userID := "user-p"
	left, err := repo.List(ctx, activity.AuditLogFilter{UserID: &userID, Limit: 10})
	require.NoError(t, err)
	require.Len(t, left, 1)
	assert.Equal(t, "al-new", left[0].ID)

	n, err = repo.Purge(ctx, now.Add(-30*24*time.Hour))
	require.NoError(t, err)
	assert.Zero(t, n, "nothing older left")
}

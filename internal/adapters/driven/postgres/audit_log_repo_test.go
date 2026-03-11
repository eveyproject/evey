package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/gravity-zero/evey/internal/domain/activity"
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

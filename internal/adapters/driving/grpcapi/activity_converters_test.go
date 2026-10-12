package grpcapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoginEntryToProto_FullAndFailure(t *testing.T) {
	t.Parallel()
	reason := "bad password"
	e := &activity.LoginEntry{
		ID:            "login-1",
		UserID:        shared.UserID("usr-1"),
		ClientIP:      "10.0.0.1",
		UserAgent:     "ua",
		Success:       false,
		FailureReason: &reason,
		CreatedAt:     time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	pb := loginEntryToProto(e)
	assert.Equal(t, "login-1", pb.Id)
	assert.Equal(t, "usr-1", pb.UserId)
	assert.Equal(t, "10.0.0.1", pb.IpAddress)
	assert.Equal(t, "ua", pb.UserAgent)
	assert.False(t, pb.Success)
	assert.Equal(t, "bad password", pb.FailureReason)
	require.NotNil(t, pb.Timestamp)
}

func TestLoginEntryToProto_NilFailureReason(t *testing.T) {
	t.Parallel()
	e := &activity.LoginEntry{ID: "l", UserID: shared.UserID("u"), Success: true}
	pb := loginEntryToProto(e)
	assert.Equal(t, "", pb.FailureReason)
}

func TestServerMetricToProto_MemoryPercent(t *testing.T) {
	t.Parallel()
	m := &activity.ServerMetric{
		CPUPercent:       42.5,
		MemoryUsedBytes:  500,
		MemoryTotalBytes: 1000,
		ActiveStreams:    3,
		ActiveTranscodes: 1,
		RecordedAt:       time.Now(),
	}
	pb := serverMetricToProto(m)
	assert.InDelta(t, 42.5, pb.CpuPercent, 0.0001)
	assert.InDelta(t, 50.0, pb.MemoryPercent, 0.0001)
	assert.Equal(t, int64(500), pb.MemoryUsedBytes)
	assert.Equal(t, int32(3), pb.ActiveStreams)
	assert.Equal(t, int32(1), pb.ActiveTranscodes)
}

func TestServerMetricToProto_ZeroTotalMemory(t *testing.T) {
	t.Parallel()
	m := &activity.ServerMetric{MemoryUsedBytes: 100, MemoryTotalBytes: 0, RecordedAt: time.Now()}
	pb := serverMetricToProto(m)
	assert.Equal(t, float64(0), pb.MemoryPercent)
}

func TestPlayHistoryEntryToProto_FullFields(t *testing.T) {
	t.Parallel()
	end := time.Date(2024, 2, 2, 0, 0, 0, 0, time.UTC)
	dur, _ := shared.NewDurationMs(1000)
	pos, _ := shared.NewDurationMs(500)
	e := &activity.PlayHistoryEntry{
		ID:          "ph-1",
		ProfileID:   shared.ProfileID("p1"),
		MediaItemID: shared.MediaItemID("m1"),
		MediaTitle:  "t",
		MediaType:   "movie",
		Duration:    dur,
		Position:    pos,
		Completed:   true,
		DeviceName:  "dev",
		ClientName:  "cli",
		PlayMethod:  "direct",
		StartedAt:   time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
		EndedAt:     &end,
	}
	pb := playHistoryEntryToProto(e)
	assert.Equal(t, "ph-1", pb.Id)
	assert.Equal(t, "p1", pb.ProfileId)
	assert.Equal(t, "m1", pb.MediaItemId)
	assert.Equal(t, "t", pb.MediaTitle)
	assert.Equal(t, int64(1000), pb.DurationMs)
	assert.Equal(t, int64(500), pb.PositionMs)
	assert.True(t, pb.Completed)
	assert.Equal(t, "dev", pb.DeviceName)
	assert.Equal(t, "cli", pb.ClientName)
	assert.Equal(t, "direct", pb.PlayMethod)
	require.NotNil(t, pb.EndedAt)
}

func TestPlayHistoryEntryToProto_NilEndedAt(t *testing.T) {
	t.Parallel()
	e := &activity.PlayHistoryEntry{ID: "x", MediaType: "movie"}
	pb := playHistoryEntryToProto(e)
	assert.Nil(t, pb.EndedAt)
}

func TestAuditLogEntryToProto_FullFieldsWithDetails(t *testing.T) {
	t.Parallel()
	resID := "res-1"
	ip := "1.2.3.4"
	e := &activity.AuditLogEntry{
		ID:           "aud-1",
		UserID:       shared.UserID("usr-1"),
		Username:     "alice",
		Action:       "create",
		ResourceType: "library",
		ResourceID:   &resID,
		ResourceName: "Movies",
		Details:      map[string]any{"foo": "bar"},
		ClientIP:     &ip,
		CreatedAt:    time.Now(),
	}
	pb := auditLogEntryToProto(e)
	assert.Equal(t, "aud-1", pb.Id)
	assert.Equal(t, "usr-1", pb.UserId)
	assert.Equal(t, "alice", pb.Username)
	assert.Equal(t, "create", pb.Action)
	assert.Equal(t, "library", pb.ResourceType)
	assert.Equal(t, "res-1", pb.ResourceId)
	assert.Equal(t, "Movies", pb.ResourceName)
	assert.Equal(t, "1.2.3.4", pb.IpAddress)
	// Details JSON round-trip
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(pb.Details), &got))
	assert.Equal(t, "bar", got["foo"])
}

func TestAuditLogEntryToProto_NilOptionalsAndEmptyDetails(t *testing.T) {
	t.Parallel()
	e := &activity.AuditLogEntry{ID: "a", UserID: shared.UserID("u"), Action: "x"}
	pb := auditLogEntryToProto(e)
	assert.Equal(t, "", pb.ResourceId)
	assert.Equal(t, "", pb.IpAddress)
	assert.Equal(t, "", pb.Details)
}

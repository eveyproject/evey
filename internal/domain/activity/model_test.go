package activity_test

import (
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/domain/shared"
)

func TestAuthMethodConstants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		method activity.AuthMethod
		want   string
	}{
		{activity.Local, "local"},
		{activity.OIDC, "oidc"},
		{activity.APIKeyAuth, "api_key"},
		{activity.QuickConnect, "quick_connect"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			t.Parallel()
			if string(c.method) != c.want {
				t.Errorf("AuthMethod = %q, want %q", c.method, c.want)
			}
		})
	}
}

func TestPlayHistoryEntry_CompletionFlag(t *testing.T) {
	t.Parallel()

	now := time.Now()

	t.Run("completed play", func(t *testing.T) {
		t.Parallel()
		dur, _ := shared.NewDurationMs(5400000) // 90 min
		pos, _ := shared.NewDurationMs(5400000)
		entry := &activity.PlayHistoryEntry{
			ID:          "h1",
			ProfileID:   "p1",
			MediaItemID: "m1",
			MediaTitle:  "Some Movie",
			MediaType:   "movie",
			Duration:    dur,
			Position:    pos,
			Completed:   true,
			StartedAt:   now.Add(-90 * time.Minute),
		}
		if !entry.Completed {
			t.Error("entry should be marked completed")
		}
		if entry.Position.Milliseconds() != entry.Duration.Milliseconds() {
			t.Errorf("position should equal duration for completed item: pos=%d dur=%d", entry.Position.Milliseconds(), entry.Duration.Milliseconds())
		}
	})

	t.Run("partial play", func(t *testing.T) {
		t.Parallel()
		dur, _ := shared.NewDurationMs(2700000) // 45 min
		pos, _ := shared.NewDurationMs(1350000) // 22.5 min - halfway
		entry := &activity.PlayHistoryEntry{
			ID:          "h2",
			ProfileID:   "p1",
			MediaItemID: "m2",
			MediaTitle:  "Series Ep 1",
			MediaType:   "episode",
			Duration:    dur,
			Position:    pos,
			Completed:   false,
			StartedAt:   now.Add(-30 * time.Minute),
		}
		if entry.Completed {
			t.Error("partial play should not be marked completed")
		}
		if entry.Position.Milliseconds() >= entry.Duration.Milliseconds() {
			t.Error("position should be less than duration for incomplete item")
		}
	})

	t.Run("ended at set on completion", func(t *testing.T) {
		t.Parallel()
		ended := now
		entry := &activity.PlayHistoryEntry{
			ID:        "h3",
			Completed: true,
			StartedAt: now.Add(-time.Hour),
			EndedAt:   &ended,
		}
		if entry.EndedAt == nil {
			t.Error("EndedAt should be set when play session ended")
		}
	})
}

func TestLoginEntry_SuccessAndFailure(t *testing.T) {
	t.Parallel()

	t.Run("successful login", func(t *testing.T) {
		t.Parallel()
		entry := &activity.LoginEntry{
			ID:         "l1",
			UserID:     "u1",
			ClientIP:   "192.168.1.1",
			AuthMethod: string(activity.Local),
			Success:    true,
			CreatedAt:  time.Now(),
		}
		if !entry.Success {
			t.Error("entry should be successful")
		}
		if entry.FailureReason != nil {
			t.Error("FailureReason should be nil on success")
		}
	})

	t.Run("failed login with reason", func(t *testing.T) {
		t.Parallel()
		reason := "invalid password"
		entry := &activity.LoginEntry{
			ID:            "l2",
			UserID:        "u1",
			ClientIP:      "10.0.0.1",
			AuthMethod:    string(activity.Local),
			Success:       false,
			FailureReason: &reason,
			CreatedAt:     time.Now(),
		}
		if entry.Success {
			t.Error("entry should not be successful")
		}
		if entry.FailureReason == nil {
			t.Error("FailureReason should be set on failure")
		}
		if *entry.FailureReason != "invalid password" {
			t.Errorf("FailureReason = %q, want %q", *entry.FailureReason, "invalid password")
		}
	})
}

func TestServerMetric_Fields(t *testing.T) {
	t.Parallel()

	m := &activity.ServerMetric{
		ID:                "sm1",
		CPUPercent:        35.5,
		MemoryUsedBytes:   1 << 30, // 1 GB
		MemoryTotalBytes:  8 << 30, // 8 GB
		ActiveStreams:     3,
		ActiveTranscodes:  1,
		BandwidthBps:      50_000_000,
		StorageUsedBytes:  500 << 30,
		StorageTotalBytes: 2000 << 30,
		RecordedAt:        time.Now(),
	}

	if m.CPUPercent < 0 || m.CPUPercent > 100 {
		t.Errorf("CPUPercent out of range: %v", m.CPUPercent)
	}
	if m.MemoryUsedBytes > m.MemoryTotalBytes {
		t.Error("MemoryUsedBytes should not exceed MemoryTotalBytes")
	}
	if m.StorageUsedBytes > m.StorageTotalBytes {
		t.Error("StorageUsedBytes should not exceed StorageTotalBytes")
	}
}

func TestUserStats_GenreBreakdown(t *testing.T) {
	t.Parallel()

	stats := &activity.UserStats{
		ProfileID:        "p1",
		TotalWatchTimeMs: 3_600_000, // 1 hour
		TotalPlayCount:   5,
		GenreBreakdown: map[string]activity.GenreStats{
			"action":   {TotalTimeMs: 1800000, PlayCount: 3},
			"sci-fi":   {TotalTimeMs: 1200000, PlayCount: 2},
			"thriller": {TotalTimeMs: 600000, PlayCount: 1},
		},
		MostPlayedItems: []activity.MediaPlayCount{
			{MediaItemID: "m1", Title: "Some Movie", PlayCount: 3},
			{MediaItemID: "m2", Title: "Another Movie", PlayCount: 2},
		},
		PlayByMonth: map[string]activity.MonthlyStats{
			"2026-01": {PlayCount: 2, TotalTimeMs: 7200000},
			"2026-02": {PlayCount: 3, TotalTimeMs: 10800000},
		},
	}

	if stats.TotalPlayCount <= 0 {
		t.Error("TotalPlayCount must be positive")
	}
	if len(stats.GenreBreakdown) != 3 {
		t.Errorf("GenreBreakdown len = %d, want 3", len(stats.GenreBreakdown))
	}
	if stats.GenreBreakdown["action"].TotalTimeMs != 1800000 {
		t.Errorf("action watch time = %d, want 1800000", stats.GenreBreakdown["action"].TotalTimeMs)
	}
	if stats.MostPlayedItems[0].PlayCount < stats.MostPlayedItems[1].PlayCount {
		t.Error("MostPlayedItems should be ordered by play count descending")
	}
}

func TestServerStatsResult_Fields(t *testing.T) {
	t.Parallel()

	r := &activity.ServerStatsResult{
		ActiveStreams:    2,
		ActiveTranscodes: 1,
		TotalUsers:       100,
		TotalItems:       5000,
		TotalPlayTimeMs:  36_000_000,
	}

	if r.ActiveStreams < 0 {
		t.Error("ActiveStreams must not be negative")
	}
	if r.TotalItems <= 0 {
		t.Error("TotalItems must be positive")
	}
}

func TestAuditLogEntry_OptionalFields(t *testing.T) {
	t.Parallel()

	entry := &activity.AuditLogEntry{
		ID:           "al1",
		UserID:       "u1",
		Action:       "library.delete",
		ResourceType: "library",
		CreatedAt:    time.Now(),
	}

	// ResourceID, ClientIP, UserAgent, DeviceName are optional
	if entry.ResourceID != nil {
		t.Error("ResourceID should be nil when not targeting a specific resource")
	}
	if entry.ClientIP != nil {
		t.Error("ClientIP should be nil by default")
	}
	if entry.UserAgent != nil {
		t.Error("UserAgent should be nil by default")
	}
	if entry.DeviceName != nil {
		t.Error("DeviceName should be nil by default")
	}

	// with optional fields set
	resID := "lib1"
	ip := "10.0.0.1"
	entry.ResourceID = &resID
	entry.ClientIP = &ip
	entry.Details = map[string]any{"reason": "cleanup"}

	if *entry.ResourceID != "lib1" {
		t.Errorf("ResourceID = %q, want %q", *entry.ResourceID, "lib1")
	}
	if entry.Details["reason"] != "cleanup" {
		t.Errorf("Details[reason] = %v, want %q", entry.Details["reason"], "cleanup")
	}
}

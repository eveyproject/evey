package notification_test

import (
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/notification"
)

func TestTypeConstants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		typ  notification.Type
		want string
	}{
		{notification.TypeNewMovie, "new_movie"},
		{notification.TypeNewEpisode, "new_episode"},
		{notification.TypeNewSeason, "new_season"},
		{notification.TypeNewSeries, "new_series"},
		{notification.TypeNewAlbum, "new_album"},
		{notification.TypeLibraryScanned, "library_scanned"},
		{notification.TypePluginError, "plugin_error"},
		{notification.TypeCustom, "custom"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			t.Parallel()
			if string(c.typ) != c.want {
				t.Errorf("Type = %q, want %q", c.typ, c.want)
			}
		})
	}
}

func TestType_IsValid_Notification(t *testing.T) {
	t.Parallel()
	cases := map[notification.Type]bool{
		notification.TypeNewMovie:       true,
		notification.TypeNewEpisode:     true,
		notification.TypeNewSeason:      true,
		notification.TypeNewSeries:      true,
		notification.TypeNewAlbum:       true,
		notification.TypeLibraryScanned: true,
		notification.TypeBackupFailed:   true,
		notification.TypePluginError:    true,
		notification.TypeCustom:         true,
		notification.Type(""):           false,
		"garbage":                       false,
	}
	for typ, want := range cases {
		if typ.IsValid() != want {
			t.Errorf("%q.IsValid() = %v, want %v", typ, !want, want)
		}
	}
}

func TestLevelConstants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		level notification.Level
		want  string
	}{
		{notification.LevelDisabled, "disabled"},
		{notification.LevelAll, "all"},
		{notification.LevelRelevant, "relevant"},
		{notification.LevelHighlyRelevant, "highly_relevant"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			t.Parallel()
			if string(c.level) != c.want {
				t.Errorf("Level = %q, want %q", c.level, c.want)
			}
		})
	}
}

func TestNotification_ReadState(t *testing.T) {
	t.Parallel()

	now := time.Now()

	t.Run("unread", func(t *testing.T) {
		t.Parallel()
		n := &notification.Notification{
			ID:        "n1",
			ProfileID: "p1",
			Type:      notification.TypeNewMovie,
			Title:     "New Movie",
			Level:     notification.LevelAll,
			CreatedAt: now,
		}
		if n.ReadAt != nil {
			t.Error("ReadAt should be nil for unread notification")
		}
		if n.DeliveredAt != nil {
			t.Error("DeliveredAt should be nil before delivery")
		}
	})

	t.Run("delivered not read", func(t *testing.T) {
		t.Parallel()
		delivered := now.Add(time.Millisecond)
		n := &notification.Notification{
			ID:          "n2",
			ProfileID:   "p1",
			Type:        notification.TypeNewEpisode,
			Title:       "New Episode",
			Level:       notification.LevelAll,
			CreatedAt:   now,
			DeliveredAt: &delivered,
		}
		if n.DeliveredAt == nil {
			t.Error("DeliveredAt should be set after delivery")
		}
		if n.ReadAt != nil {
			t.Error("ReadAt should still be nil before user reads")
		}
	})

	t.Run("read", func(t *testing.T) {
		t.Parallel()
		delivered := now.Add(time.Millisecond)
		readAt := now.Add(time.Second)
		n := &notification.Notification{
			ID:          "n3",
			ProfileID:   "p1",
			Type:        notification.TypeNewSeries,
			Title:       "New Series",
			Level:       notification.LevelRelevant,
			CreatedAt:   now,
			DeliveredAt: &delivered,
			ReadAt:      &readAt,
		}
		if n.ReadAt == nil {
			t.Error("ReadAt should be set after user reads")
		}
		if !n.ReadAt.After(*n.DeliveredAt) {
			t.Error("ReadAt must be after DeliveredAt")
		}
	})
}

func TestNotification_Expiry(t *testing.T) {
	t.Parallel()

	now := time.Now()

	t.Run("no expiry", func(t *testing.T) {
		t.Parallel()
		n := &notification.Notification{
			ID:        "n1",
			CreatedAt: now,
		}
		if n.ExpiresAt != nil {
			t.Error("ExpiresAt should be nil when no expiry is set")
		}
	})

	t.Run("with expiry in past", func(t *testing.T) {
		t.Parallel()
		past := now.Add(-time.Minute)
		n := &notification.Notification{
			ID:        "n2",
			CreatedAt: now.Add(-time.Hour),
			ExpiresAt: &past,
		}
		if n.ExpiresAt == nil {
			t.Fatal("ExpiresAt should be set")
		}
		if !n.ExpiresAt.Before(now) {
			t.Error("notification should be expired")
		}
	})
}

func TestNotification_IsRead(t *testing.T) {
	t.Parallel()
	n := &notification.Notification{}
	if n.IsRead() {
		t.Error("should not be read")
	}
	readAt := time.Now()
	n.ReadAt = &readAt
	if !n.IsRead() {
		t.Error("should be read")
	}
}

func TestNotification_IsDelivered(t *testing.T) {
	t.Parallel()
	n := &notification.Notification{}
	if n.IsDelivered() {
		t.Error("should not be delivered")
	}
	d := time.Now()
	n.DeliveredAt = &d
	if !n.IsDelivered() {
		t.Error("should be delivered")
	}
}

func TestNotification_IsExpired_Method(t *testing.T) {
	t.Parallel()
	n := &notification.Notification{}
	if n.IsExpired() {
		t.Error("nil ExpiresAt = not expired")
	}
	past := time.Now().Add(-time.Minute)
	n.ExpiresAt = &past
	if !n.IsExpired() {
		t.Error("past ExpiresAt = expired")
	}
	future := time.Now().Add(time.Hour)
	n.ExpiresAt = &future
	if n.IsExpired() {
		t.Error("future ExpiresAt = not expired")
	}
}

func TestSettings_LevelForType(t *testing.T) {
	t.Parallel()
	s := &notification.Settings{
		MovieLevel:  notification.LevelAll,
		SeriesLevel: notification.LevelRelevant,
		MusicLevel:  notification.LevelDisabled,
	}
	tests := []struct {
		typ  notification.Type
		want notification.Level
	}{
		{notification.TypeNewMovie, notification.LevelAll},
		{notification.TypeNewEpisode, notification.LevelRelevant},
		{notification.TypeNewSeason, notification.LevelRelevant},
		{notification.TypeNewSeries, notification.LevelRelevant},
		{notification.TypeNewAlbum, notification.LevelDisabled},
		{notification.TypeCustom, notification.LevelAll},
	}
	for _, tt := range tests {
		if got := s.LevelForType(tt.typ); got != tt.want {
			t.Errorf("LevelForType(%q) = %q, want %q", tt.typ, got, tt.want)
		}
	}
}

func TestSettings_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		s       notification.Settings
		wantErr bool
	}{
		{"valid all levels", notification.Settings{MovieLevel: notification.LevelAll, SeriesLevel: notification.LevelRelevant, MusicLevel: notification.LevelDisabled}, false},
		{"valid highly_relevant", notification.Settings{MovieLevel: notification.LevelHighlyRelevant, SeriesLevel: notification.LevelAll, MusicLevel: notification.LevelAll}, false},
		{"invalid movie level", notification.Settings{MovieLevel: "bad", SeriesLevel: notification.LevelAll, MusicLevel: notification.LevelAll}, true},
		{"invalid series level", notification.Settings{MovieLevel: notification.LevelAll, SeriesLevel: "nope", MusicLevel: notification.LevelAll}, true},
		{"invalid music level", notification.Settings{MovieLevel: notification.LevelAll, SeriesLevel: notification.LevelAll, MusicLevel: "wrong"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.s.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() err=%v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestSettings_Defaults(t *testing.T) {
	t.Parallel()

	// default settings: all zero-value (empty string for Level = not explicitly set)
	s := &notification.Settings{
		ProfileID: "p1",
	}
	if s.MovieLevel != "" {
		t.Errorf("MovieLevel default should be empty string, got %q", s.MovieLevel)
	}

	// explicitly configured settings
	s2 := &notification.Settings{
		ProfileID:   "p2",
		MovieLevel:  notification.LevelAll,
		SeriesLevel: notification.LevelRelevant,
		MusicLevel:  notification.LevelDisabled,
	}
	if s2.MovieLevel != notification.LevelAll {
		t.Errorf("MovieLevel = %q, want %q", s2.MovieLevel, notification.LevelAll)
	}
	if s2.SeriesLevel != notification.LevelRelevant {
		t.Errorf("SeriesLevel = %q, want %q", s2.SeriesLevel, notification.LevelRelevant)
	}
	if s2.MusicLevel != notification.LevelDisabled {
		t.Errorf("MusicLevel = %q, want %q", s2.MusicLevel, notification.LevelDisabled)
	}
}

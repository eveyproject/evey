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
	})

	t.Run("read", func(t *testing.T) {
		t.Parallel()
		readAt := now.Add(time.Second)
		n := &notification.Notification{
			ID:        "n3",
			ProfileID: "p1",
			Type:      notification.TypeNewSeries,
			Title:     "New Series",
			Level:     notification.LevelRelevant,
			CreatedAt: now,
			ReadAt:    &readAt,
		}
		if n.ReadAt == nil {
			t.Error("ReadAt should be set after user reads")
		}
		if !n.IsRead() {
			t.Error("IsRead must report true once ReadAt is set")
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

func TestLevel_IsValid(t *testing.T) {
	t.Parallel()
	cases := map[notification.Level]bool{
		notification.LevelDisabled:       true,
		notification.LevelAll:            true,
		notification.LevelRelevant:       true,
		notification.LevelHighlyRelevant: true,
		notification.Level(""):           false,
		"garbage":                        false,
	}
	for l, want := range cases {
		if got := l.IsValid(); got != want {
			t.Errorf("%q.IsValid() = %v, want %v", l, got, want)
		}
	}
}

func TestNotification_Validate(t *testing.T) {
	t.Parallel()

	valid := notification.Notification{
		ID:        "n1",
		ProfileID: "p1",
		Type:      notification.TypeNewMovie,
		Title:     "New",
	}

	tests := []struct {
		name   string
		mutate func(n *notification.Notification)
		want   string
	}{
		{"valid", func(*notification.Notification) {}, ""},
		{"missing ID", func(n *notification.Notification) { n.ID = "" }, "notification ID is required"},
		{"missing profile", func(n *notification.Notification) { n.ProfileID = "" }, "notification profile ID is required"},
		{"bad type", func(n *notification.Notification) { n.Type = "bogus" }, "notification: invalid type"},
		{"empty type", func(n *notification.Notification) { n.Type = "" }, "notification: invalid type"},
		{"empty title", func(n *notification.Notification) { n.Title = "" }, "notification title is required"},
		{"bad level", func(n *notification.Notification) { n.Level = "bogus" }, "notification: invalid level"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			n := valid
			tt.mutate(&n)
			err := n.Validate()
			if tt.want == "" {
				if err != nil {
					t.Fatalf("unexpected: %v", err)
				}
				return
			}
			if err == nil || err.Error() != tt.want {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestNewNotification(t *testing.T) {
	t.Parallel()

	t.Run("happy path stamps CreatedAt", func(t *testing.T) {
		t.Parallel()
		expires := time.Now().Add(time.Hour)
		n, err := notification.NewNotification("n1", "p1", notification.TypeNewMovie, "New",
			notification.WithNotificationMessage("msg"),
			notification.WithNotificationMediaItem("mi1"),
			notification.WithNotificationLevel(notification.LevelRelevant),
			notification.WithNotificationExpiresAt(&expires),
		)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if n.CreatedAt.IsZero() || n.Message != "msg" || n.MediaItemID != "mi1" || n.Level != notification.LevelRelevant || n.ExpiresAt == nil {
			t.Errorf("fields not wired: %+v", n)
		}
	})

	t.Run("missing id rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := notification.NewNotification("", "p1", notification.TypeNewMovie, "t"); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("invalid type rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := notification.NewNotification("n1", "p1", "bogus", "t"); err == nil {
			t.Fatal("expected error, got nil")
		}
	})
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

// The admin-only set drives ListAdminNotifications and the web bell's type
// filter; a type added to the wrong side either hides a user's own row from
// them or leaks an operational one into a child profile's feed.
func TestType_IsAdminOnly(t *testing.T) {
	t.Parallel()
	adminOnly := map[notification.Type]bool{
		notification.TypeBackupFailed: true,
		notification.TypePluginError:  true,
		notification.TypeMediaRequest: true,
	}
	all := []notification.Type{
		notification.TypeNewMovie, notification.TypeNewEpisode, notification.TypeNewSeason,
		notification.TypeNewSeries, notification.TypeNewAlbum, notification.TypeLibraryScanned,
		notification.TypeBackupFailed, notification.TypePluginError, notification.TypeCustom,
		notification.TypeMediaRequest, notification.TypeRequestAnswered,
	}
	for _, typ := range all {
		if got := typ.IsAdminOnly(); got != adminOnly[typ] {
			t.Errorf("%s.IsAdminOnly() = %v, want %v", typ, got, adminOnly[typ])
		}
	}
}

// The series key is "<library>:<title>", scoped per library so two libraries
// holding the same show do not collapse into one thread, and an untitled
// series yields the empty key (no grouping) rather than a dangling ":" one.
func TestNewSeriesKey_And_LabelParts(t *testing.T) {
	t.Parallel()
	if got := notification.NewSeriesKey("lib1", "Some Show"); got != "lib1:Some Show" {
		t.Errorf("NewSeriesKey = %q", got)
	}
	if got := notification.NewSeriesKey("lib1", ""); got != "" {
		t.Errorf("empty title must give the empty key, got %q", got)
	}
	if notification.NewSeriesKey("lib1", "Show") == notification.NewSeriesKey("lib2", "Show") {
		t.Error("the same title in two libraries must not share a key")
	}

	n, err := notification.NewNotification("n1", "p1", notification.TypeNewEpisode, "New episodes",
		notification.WithSeriesKey(notification.NewSeriesKey("lib1", "Some Show")),
		notification.WithLabelParts("Some Show", 1, 3),
	)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if n.SeriesKey != "lib1:Some Show" || n.Subject != "Some Show" || n.SeasonCount != 1 || n.EpisodeCount != 3 {
		t.Errorf("label parts not wired: %+v", n)
	}
}

package playback_test

import (
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/playback"
	"github.com/eveyproject/evey/internal/domain/shared"
)

func mustDuration(t *testing.T, ms int64) shared.Duration {
	t.Helper()
	d, err := shared.NewDurationMs(ms)
	if err != nil {
		t.Fatalf("NewDurationMs(%d): %v", ms, err)
	}
	return d
}

func TestNewPlaySession(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		s, err := playback.NewPlaySession("s-1", "p-1", "m-1",
			playback.WithPlaySessionMethod(playback.DirectPlay),
			playback.WithPlaySessionDevice("Chrome", "web"),
			playback.WithPlaySessionBandwidth(5_000_000),
		)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if s.ID != "s-1" || s.ProfileID != "p-1" || s.MediaItemID != "m-1" {
			t.Errorf("core fields wrong: %+v", s)
		}
		if s.PlayMethod != string(playback.DirectPlay) || s.DeviceName != "Chrome" || s.DeviceType != "web" {
			t.Errorf("options not applied: %+v", s)
		}
		if s.StartedAt.IsZero() {
			t.Error("StartedAt not stamped")
		}
	})

	t.Run("missing fields", func(t *testing.T) {
		t.Parallel()
		if _, err := playback.NewPlaySession("", "p-1", "m-1"); err == nil {
			t.Fatal("expected error on empty ID")
		}
		if _, err := playback.NewPlaySession("s-1", "", "m-1"); err == nil {
			t.Fatal("expected error on empty profileID")
		}
		if _, err := playback.NewPlaySession("s-1", "p-1", ""); err == nil {
			t.Fatal("expected error on empty mediaItemID")
		}
	})
}

func TestPlayMethod_IsValid(t *testing.T) {
	t.Parallel()
	cases := map[playback.PlayMethod]bool{
		playback.DirectPlay:     true,
		playback.DirectStream:   true,
		playback.Transcode:      true,
		playback.PlayMethod(""): false,
		"garbage":               false,
	}
	for m, want := range cases {
		if m.IsValid() != want {
			t.Errorf("%q.IsValid() = %v, want %v", m, !want, want)
		}
	}
}

func TestWatchProgress_IsInProgress(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		wp   playback.WatchProgress
		want bool
	}{
		{"completed", playback.WatchProgress{Completed: true, Position: mustDuration(t, 5000)}, false},
		{"position zero", playback.WatchProgress{Completed: false}, false},
		{"in progress", playback.WatchProgress{Completed: false, Position: mustDuration(t, 5000)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.wp.IsInProgress(); got != tt.want {
				t.Errorf("IsInProgress() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlaySession_IsActive(t *testing.T) {
	t.Parallel()
	active := &playback.PlaySession{}
	if !active.IsActive() {
		t.Error("session without EndedAt should be active")
	}
	now := time.Now()
	ended := &playback.PlaySession{EndedAt: &now}
	if ended.IsActive() {
		t.Error("session with EndedAt should not be active")
	}
}

func TestPlaySession_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		session playback.PlaySession
		wantErr string
	}{
		{
			name:    "missing ID",
			session: playback.PlaySession{ProfileID: "p-1", MediaItemID: "m-1"},
			wantErr: "ID is required",
		},
		{
			name:    "missing profile id",
			session: playback.PlaySession{ID: "s-1", MediaItemID: "m-1"},
			wantErr: "profile ID is required",
		},
		{
			name:    "missing media item id",
			session: playback.PlaySession{ID: "s-1", ProfileID: "p-1"},
			wantErr: "media item ID is required",
		},
		{
			name:    "invalid play method",
			session: playback.PlaySession{ID: "s-1", ProfileID: "p-1", MediaItemID: "m-1", PlayMethod: "bogus"},
			wantErr: "invalid play method",
		},
		{
			name:    "valid session",
			session: playback.PlaySession{ID: "s-1", ProfileID: "p-1", MediaItemID: "m-1"},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.session.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !contains(err.Error(), tt.wantErr) {
				t.Errorf("Validate() = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestActiveSessionInfo_ProgressPercent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		positionMs int64
		durationMs int64
		want       float64
	}{
		{"zero duration", 0, 0, 0},
		{"negative duration", 5000, -1, 0},
		{"start of media", 0, 90000, 0},
		{"halfway", 45000, 90000, 50},
		{"end of media", 90000, 90000, 100},
		{"position exceeds duration clamps to 100", 100000, 90000, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a := &playback.ActiveSessionInfo{}
			if tt.positionMs > 0 {
				a.Position = mustDuration(t, tt.positionMs)
			}
			// shared.Duration rejects negatives; for the "negative duration"
			// case we leave the zero value, which matches the expected result.
			if tt.durationMs > 0 {
				a.Duration = mustDuration(t, tt.durationMs)
			}
			got := a.ProgressPercent()
			if got != tt.want {
				t.Errorf("ProgressPercent() = %v, want %v", got, tt.want)
			}
		})
	}
}

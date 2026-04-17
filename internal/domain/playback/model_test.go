package playback_test

import (
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/playback"
)

func TestWatchProgress_IsInProgress(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		wp   playback.WatchProgress
		want bool
	}{
		{"completed", playback.WatchProgress{Completed: true, PositionMs: 5000}, false},
		{"position zero", playback.WatchProgress{Completed: false, PositionMs: 0}, false},
		{"in progress", playback.WatchProgress{Completed: false, PositionMs: 5000}, true},
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
			name:    "missing profile id",
			session: playback.PlaySession{MediaItemID: "m-1"},
			wantErr: "profile ID is required",
		},
		{
			name:    "missing media item id",
			session: playback.PlaySession{ProfileID: "p-1"},
			wantErr: "media item ID is required",
		},
		{
			name:    "valid session",
			session: playback.PlaySession{ProfileID: "p-1", MediaItemID: "m-1"},
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
			a := &playback.ActiveSessionInfo{
				PositionMs: tt.positionMs,
				DurationMs: tt.durationMs,
			}
			got := a.ProgressPercent()
			if got != tt.want {
				t.Errorf("ProgressPercent() = %v, want %v", got, tt.want)
			}
		})
	}
}

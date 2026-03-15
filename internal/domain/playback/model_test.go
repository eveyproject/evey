package playback_test

import (
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/domain/playback"
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

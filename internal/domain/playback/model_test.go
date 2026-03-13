package playback_test

import (
	"testing"

	"github.com/gravity-zero/evey/internal/domain/playback"
)

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

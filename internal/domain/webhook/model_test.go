package webhook_test

import (
	"testing"

	"github.com/gravity-zero/evey/internal/domain/webhook"
)

func TestConfig_MatchesEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		events []string
		event  string
		want   bool
	}{
		{"wildcard matches any", []string{"*"}, "playback.started", true},
		{"exact match", []string{"playback.started"}, "playback.started", true},
		{"no match", []string{"playback.started"}, "playback.stopped", false},
		{"one of many", []string{"a", "b", "c"}, "b", true},
		{"empty events", nil, "any", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &webhook.Config{Events: tt.events}
			if got := cfg.MatchesEvent(tt.event); got != tt.want {
				t.Errorf("MatchesEvent(%q) = %v, want %v", tt.event, got, tt.want)
			}
		})
	}
}

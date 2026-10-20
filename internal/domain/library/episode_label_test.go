package library_test

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/library"
)

func TestEpisodeLabel(t *testing.T) {
	tests := []struct {
		series          string
		season, episode int
		title, want     string
	}{
		{"Slow Horses", 1, 3, "Bad Tradecraft", "Slow Horses · S01E03 · Bad Tradecraft"},
		{"Slow Horses", 0, 3, "Bad Tradecraft", "Slow Horses · E03 · Bad Tradecraft"},
		{"", 2, 10, "Finale", "S02E10 · Finale"},
		{" Slow Horses ", 0, 0, "  ", "Slow Horses"},
		{"", 0, 0, "Prédestination", "Prédestination"},
		{"Show", 12, 105, "Long", "Show · S12E105 · Long"},
		{"", 0, 0, "", ""},
	}
	for _, tt := range tests {
		if got := library.EpisodeLabel(tt.series, tt.season, tt.episode, tt.title); got != tt.want {
			t.Errorf("EpisodeLabel(%q, %d, %d, %q) = %q, want %q", tt.series, tt.season, tt.episode, tt.title, got, tt.want)
		}
	}
	if got := library.EpisodeCode(3, 0); got != "S03" {
		t.Errorf("EpisodeCode(3, 0) = %q, want S03", got)
	}
}

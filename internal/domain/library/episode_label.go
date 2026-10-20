package library

import (
	"fmt"
	"strings"
)

// EpisodeCode is "S01E03": "E03" when the season is unknown, "S01" without an
// episode number, "" with neither. Zero or a negative number is unknown.
func EpisodeCode(season, episode int) string {
	var b strings.Builder
	if season > 0 {
		fmt.Fprintf(&b, "S%02d", season)
	}
	if episode > 0 {
		fmt.Fprintf(&b, "E%02d", episode)
	}
	return b.String()
}

// EpisodeLabel is the one label for an episode wherever an admin has to know
// which one it is without opening it: "Series · S01E03 · Title", the format
// of the clients' episodeLabel (packages/api). A missing part is left out,
// never shown as a placeholder, so an item that is not an episode (no series,
// no numbers) is its title alone.
func EpisodeLabel(series string, season, episode int, title string) string {
	parts := make([]string, 0, 3)
	for _, p := range []string{strings.TrimSpace(series), EpisodeCode(season, episode), strings.TrimSpace(title)} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}

package notification

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/domain/notification"
	"github.com/evey-project/evey/internal/domain/user"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
)

const dedupeWindow = 5 * time.Minute
const defaultRetentionDays = 30

type ProfileLister interface {
	GetAll(ctx context.Context) ([]*user.Profile, error)
}

// AdminProfileLister narrows the profile list to profiles that belong to
// admin users. Used for security-critical notifications (plugin errors,
// etc.) that shouldn't spam non-admin users.
type AdminProfileLister interface {
	ListAdminProfiles(ctx context.Context) ([]*user.Profile, error)
}

// pushSpec describes the push-notification payload in a locale-agnostic way:
// the i18n key and the params to interpolate. fallback is the EN string used
// when no translator is wired (keeps tests backward-compatible).
type pushSpec struct {
	key      string
	params   map[string]string
	fallback string
}

func (p pushSpec) resolveTitle(t *eveyi18n.Translator, locale string) string {
	if p.key == "" {
		return p.fallback
	}
	if t == nil {
		return p.fallback
	}
	msg := t.T(locale, p.key, p.params)
	if msg == p.key {
		return p.fallback
	}
	return msg
}

func typeAndTitle(mediaType, title, seriesTitle string) (notification.Type, string, pushSpec) {
	label := title
	if seriesTitle != "" && (mediaType == "episode" || mediaType == "season") {
		label = seriesTitle + " - " + title
	}
	switch mediaType {
	case "movie":
		return notification.TypeNewMovie, "new_movie:" + label, pushSpec{
			key:      "notifications.push.new_movie.title",
			params:   map[string]string{"label": label},
			fallback: "New Movie: " + label,
		}
	case "episode":
		return notification.TypeNewEpisode, "new_episode:" + label, pushSpec{
			key:      "notifications.push.new_episode.title",
			params:   map[string]string{"label": label},
			fallback: "New Episode: " + label,
		}
	case "season":
		return notification.TypeNewSeason, "new_season:" + label, pushSpec{
			key:      "notifications.push.new_season.title",
			params:   map[string]string{"label": label},
			fallback: "New Season: " + label,
		}
	case "series":
		return notification.TypeNewSeries, "new_series:" + label, pushSpec{
			key:      "notifications.push.new_series.title",
			params:   map[string]string{"label": label},
			fallback: "New Series: " + label,
		}
	case "album":
		return notification.TypeNewAlbum, "new_album:" + label, pushSpec{
			key:      "notifications.push.new_album.title",
			params:   map[string]string{"label": label},
			fallback: "New Album: " + label,
		}
	}
	return "", "", pushSpec{}
}

func levelForType(cfg *notification.Settings, t notification.Type) notification.Level {
	return cfg.LevelForType(t)
}

// seriesKeyFor builds an opaque deduplication key for a series within a library.
// This key is stored in related_media_id and is not a real MediaItemID.
func seriesKeyFor(libraryID, seriesTitle string) string {
	if seriesTitle == "" {
		return ""
	}
	return libraryID + ":" + seriesTitle
}

// extractSeriesName recovers the series title from a stored notification title.
// Stored titles follow patterns:
//   - "new_episode:SeriesTitle - Episode Title"
//   - "new_episode:SeriesTitle (N episodes)"
//
// Returns the portion after the "new_episode:" prefix, up to " -" or " (".
func extractSeriesName(storedTitle string) string {
	payload := strings.TrimPrefix(storedTitle, "new_episode:")
	if idx := strings.Index(payload, " ("); idx > 0 {
		return payload[:idx]
	}
	if idx := strings.Index(payload, " - "); idx > 0 { // " - "
		return payload[:idx]
	}
	return payload
}

// plural renders "1 season" and "2 seasons". Only ever used for the English
// stored label; the push notification goes through i18n and never comes here.
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// batchedSeriesTitle returns the display label (without type prefix) and push spec.
func batchedSeriesTitle(seriesTitle string, seasonCount, episodeCount int) (label string, push pushSpec) {
	params := map[string]string{
		"series":   seriesTitle,
		"seasons":  fmt.Sprintf("%d", seasonCount),
		"episodes": fmt.Sprintf("%d", episodeCount),
	}
	// The STORED label is built here in English and persisted as-is, so the
	// clients translate the "new_series:" prefix and copy the parenthesis
	// through untouched - that is why a French TV shows "Nouvelle série :
	// Some Series (1 seasons, 1 episodes)". Carrying the counts as data instead is
	// a payload-format change across the three clients and is the user's call;
	// what is fixed here is the count that was ungrammatical even in English.
	seasons := plural(seasonCount, "season")
	episodes := plural(episodeCount, "episode")
	switch {
	case seasonCount > 0 && episodeCount > 0:
		label = fmt.Sprintf("%s (%s, %s)", seriesTitle, seasons, episodes)
		push = pushSpec{key: "notifications.push.series_with_seasons_and_episodes.title", params: params, fallback: "New Series: " + label}
	case seasonCount > 0:
		label = fmt.Sprintf("%s (%s)", seriesTitle, seasons)
		push = pushSpec{key: "notifications.push.series_with_seasons.title", params: params, fallback: "New Series: " + label}
	case episodeCount > 0:
		label = fmt.Sprintf("%s (%s)", seriesTitle, episodes)
		push = pushSpec{key: "notifications.push.series_with_episodes.title", params: params, fallback: "New Series: " + label}
	default:
		label = seriesTitle
		push = pushSpec{key: "notifications.push.new_series.title", params: map[string]string{"label": label}, fallback: "New Series: " + label}
	}
	return label, push
}

func batchedSeasonTitle(seriesTitle, seasonTitle string, episodeCount int) (label string, push pushSpec) {
	label = fmt.Sprintf("%s - %s (%d episodes)", seriesTitle, seasonTitle, episodeCount)
	push = pushSpec{
		key: "notifications.push.season_with_episodes.title",
		params: map[string]string{
			"series":   seriesTitle,
			"season":   seasonTitle,
			"episodes": fmt.Sprintf("%d", episodeCount),
		},
		fallback: "New Season: " + label,
	}
	return label, push
}

// batchedSeasonsTitle groups one or more new seasons of the same series (added
// without a full-series event) into a single label that always names the series
// - never a bare "Season N". One season → "Series - Season N"; several →
// "Series (N seasons)".
func batchedSeasonsTitle(seriesTitle, seasonTitle string, seasonCount int) (label string, push pushSpec) {
	if seasonCount > 1 {
		name := seriesTitle
		if name == "" {
			name = seasonTitle
		}
		label = fmt.Sprintf("%s (%d seasons)", name, seasonCount)
		push = pushSpec{
			key:      "notifications.push.seasons_batched.title",
			params:   map[string]string{"series": name, "count": fmt.Sprintf("%d", seasonCount)},
			fallback: "New Seasons: " + label,
		}
		return label, push
	}
	// Single season: name the series unless we don't have one (or it would just
	// repeat the season title) - then fall back to the bare season title.
	if seriesTitle == "" || seriesTitle == seasonTitle {
		label = seasonTitle
	} else {
		label = fmt.Sprintf("%s - %s", seriesTitle, seasonTitle)
	}
	push = pushSpec{
		key:      "notifications.push.new_season.title",
		params:   map[string]string{"label": label},
		fallback: "New Season: " + label,
	}
	return label, push
}

func batchedEpisodesTitle(seriesTitle string, count int) (label string, push pushSpec) {
	label = fmt.Sprintf("%s (%d episodes)", seriesTitle, count)
	push = pushSpec{
		key: "notifications.push.episodes_batched.title",
		params: map[string]string{
			"series": seriesTitle,
			"count":  fmt.Sprintf("%d", count),
		},
		fallback: "New Episodes: " + label,
	}
	return label, push
}

func batchedAlbumTitle(albumTitle string, trackCount int) (label string, push pushSpec) {
	if trackCount > 0 {
		label = fmt.Sprintf("%s (%s)", albumTitle, plural(trackCount, "track"))
		push = pushSpec{
			key: "notifications.push.album_with_tracks.title",
			params: map[string]string{
				"album":  albumTitle,
				"tracks": fmt.Sprintf("%d", trackCount),
			},
			fallback: "New Album: " + label,
		}
		return label, push
	}
	return albumTitle, pushSpec{
		key:      "notifications.push.new_album.title",
		params:   map[string]string{"label": albumTitle},
		fallback: "New Album: " + albumTitle,
	}
}

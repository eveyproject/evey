package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/evey-project/evey/internal/domain/notification"
	"github.com/evey-project/evey/internal/domain/user"
)

const dedupeWindow = 5 * time.Minute
const defaultRetentionDays = 30

type ProfileLister interface {
	GetAll(ctx context.Context) ([]*user.Profile, error)
}

func typeAndTitle(mediaType, title, seriesTitle string) (notification.Type, string, string) {
	label := title
	if seriesTitle != "" && (mediaType == "episode" || mediaType == "season") {
		label = seriesTitle + " — " + title
	}
	switch mediaType {
	case "movie":
		return notification.TypeNewMovie, "new_movie:" + label, "New Movie: " + label
	case "episode":
		return notification.TypeNewEpisode, "new_episode:" + label, "New Episode: " + label
	case "season":
		return notification.TypeNewSeason, "new_season:" + label, "New Season: " + label
	case "series":
		return notification.TypeNewSeries, "new_series:" + label, "New Series: " + label
	case "album":
		return notification.TypeNewAlbum, "new_album:" + label, "New Album: " + label
	}
	return "", "", ""
}

func levelForType(cfg *notification.Settings, t notification.Type) notification.Level {
	return cfg.LevelForType(t)
}

// batchedSeriesTitle returns the display label (without type prefix) and push title.
func batchedSeriesTitle(seriesTitle string, seasonCount, episodeCount int) (label, pushTitle string) {
	switch {
	case seasonCount > 0 && episodeCount > 0:
		label = fmt.Sprintf("%s (%d seasons, %d episodes)", seriesTitle, seasonCount, episodeCount)
	case seasonCount > 0:
		label = fmt.Sprintf("%s (%d seasons)", seriesTitle, seasonCount)
	case episodeCount > 0:
		label = fmt.Sprintf("%s (%d episodes)", seriesTitle, episodeCount)
	default:
		label = seriesTitle
	}
	return label, "New Series: " + label
}

func batchedSeasonTitle(seriesTitle, seasonTitle string, episodeCount int) (label, pushTitle string) {
	label = fmt.Sprintf("%s — %s (%d episodes)", seriesTitle, seasonTitle, episodeCount)
	return label, "New Season: " + label
}

func batchedEpisodesTitle(seriesTitle string, count int) (label, pushTitle string) {
	label = fmt.Sprintf("%s (%d episodes)", seriesTitle, count)
	return label, "New Episodes: " + label
}

func batchedAlbumTitle(albumTitle string, trackCount int) (label, pushTitle string) {
	if trackCount > 0 {
		label = fmt.Sprintf("%s (%d tracks)", albumTitle, trackCount)
		return label, "New Album: " + label
	}
	return albumTitle, "New Album: " + albumTitle
}

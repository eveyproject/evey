package notification

import (
	"context"
	"time"

	"github.com/gravity-zero/evey/internal/domain/notification"
	"github.com/gravity-zero/evey/internal/domain/user"
)

const dedupeWindow = 5 * time.Minute
const defaultRetentionDays = 30

type ProfileLister interface {
	GetAll(ctx context.Context) ([]*user.Profile, error)
}

func typeAndTitle(mediaType, title, seriesTitle string) (notification.Type, string) {
	switch mediaType {
	case "movie":
		return notification.TypeNewMovie, "New Movie: " + title
	case "episode":
		label := title
		if seriesTitle != "" {
			label = seriesTitle + " — " + title
		}
		return notification.TypeNewEpisode, "New Episode: " + label
	case "season":
		label := title
		if seriesTitle != "" {
			label = seriesTitle + " — " + title
		}
		return notification.TypeNewSeason, "New Season: " + label
	case "series":
		return notification.TypeNewSeries, "New Series: " + title
	case "album":
		return notification.TypeNewAlbum, "New Album: " + title
	}
	return "", ""
}

func levelForType(cfg *notification.Settings, t notification.Type) notification.Level {
	switch t {
	case notification.TypeNewMovie:
		return cfg.MovieLevel
	case notification.TypeNewEpisode, notification.TypeNewSeason, notification.TypeNewSeries:
		return cfg.SeriesLevel
	case notification.TypeNewAlbum:
		return cfg.MusicLevel
	}
	return notification.LevelAll
}

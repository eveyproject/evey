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

package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/filesystem"
	itunesadapter "github.com/evey-project/evey/internal/adapters/driven/itunes"
	"github.com/evey-project/evey/internal/adapters/driven/opensubtitles"
	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	proberpkg "github.com/evey-project/evey/internal/adapters/driven/prober"
	webpushadapter "github.com/evey-project/evey/internal/adapters/driven/webpush"
	appbackup "github.com/evey-project/evey/internal/app/backup"
	appfeed "github.com/evey-project/evey/internal/app/feed"
	applibrary "github.com/evey-project/evey/internal/app/library"
	appmusic "github.com/evey-project/evey/internal/app/music"
	appnotification "github.com/evey-project/evey/internal/app/notification"
	appplaylist "github.com/evey-project/evey/internal/app/playlist"
	appsubtitle "github.com/evey-project/evey/internal/app/subtitle"
	"github.com/evey-project/evey/internal/config"
	"github.com/evey-project/evey/internal/domain/library"
	eveylogger "github.com/evey-project/evey/internal/pkg/logger"
)

// wire_services.go holds the self-contained per-domain service builders that
// buildAppServices used to inline. Each takes only what it reads (never the
// tangled transcoding locals) and returns the services it produces, so the
// composition root stays a readable sequence while these blocks are named,
// isolated units. The interleaved media/transcoding core is deliberately NOT
// extracted here - its cross-block variable threading makes a helper a net loss.

// buildNotificationServices wires the notification command/query services,
// enabling Web Push when VAPID keys resolve and locale-aware push titles when a
// translator is present. Registers the command service on the event bus.
func buildNotificationServices(ctx context.Context, cfg *config.Config, infra *infraDeps, rootLog *slog.Logger) (*appnotification.CommandService, *appnotification.QueryService) {
	r := infra.repos
	bus := infra.bus
	var notifOpts []appnotification.Option
	vapidPub, vapidPriv := resolveVAPIDKeys(ctx, r.ServerCfg, cfg)
	if vapidPub != "" && vapidPriv != "" {
		wpSender := webpushadapter.NewSender(vapidPub, vapidPriv, cfg.VAPID.Subject)
		notifOpts = append(notifOpts, appnotification.WithPush(wpSender, r.PushSubscription))
		rootLog.Info("Web Push (VAPID) notifications enabled")
	}
	notifOpts = append(notifOpts, appnotification.WithAdminLister(r.Profile))
	if infra.translator != nil {
		notifOpts = append(notifOpts, appnotification.WithTranslator(infra.translator))
	}
	notifCmdSvc := appnotification.NewCommandService(r.Notification, r.NotificationSettings, r.WatchHistoryAnalyzer, r.Profile, bus, notifOpts...)
	notifCmdSvc.Register()
	notifQuerySvc := appnotification.NewQueryService(r.Notification, r.NotificationSettings, appnotification.WithPushSubs(r.PushSubscription))
	return notifCmdSvc, notifQuerySvc
}

// buildFeedServices wires the feed command/query services (the analyzer cached
// behind a 5-minute TTL) and ensures every existing library has a feed config.
func buildFeedServices(ctx context.Context, infra *infraDeps) (*appfeed.CommandService, *appfeed.QueryService) {
	r := infra.repos
	bus := infra.bus
	feedCmdSvc := appfeed.NewCommandService(r.FeedConfigCmd, r.FeedConfigQuery, appfeed.CommandWithEventBus(bus))
	cachedAnalyzer := appfeed.NewCachedAnalyzer(r.WatchHistoryAnalyzer, 5*time.Minute, appfeed.CacheWithEventBus(bus))
	feedQuerySvc := appfeed.NewQueryService(r.FeedConfigQuery, r.FeedQuerier, cachedAnalyzer)

	if libs, libErr := r.Library.List(ctx, library.ListLibrariesFilter{}); libErr == nil {
		for _, lib := range libs {
			if err := feedCmdSvc.EnsureLibraryFeedConfig(ctx, lib.ID, lib.Name); err != nil {
				slog.Warn("ensure library feed config", slog.String("library", string(lib.ID)), slog.Any("err", err))
			}
		}
	}
	return feedCmdSvc, feedQuerySvc
}

// buildMusicServices wires the music service (in-process mkvgo prober fast-path,
// iTunes album art, optional search index) and its now-playing registry.
func buildMusicServices(cfg *config.Config, infra *infraDeps) (*appmusic.Service, *appmusic.NowPlayingRegistry) {
	r := infra.repos
	bus := infra.bus
	musicScanner := filesystem.NewMusicScanner()
	musicSvc := appmusic.NewService(r.MusicTrack, bus, r.Library, musicScanner)
	musicSvc.WithPlayRepository(r.MusicPlay)
	musicNowPlaying := appmusic.NewNowPlayingRegistry()
	// Same in-process fast-path as the library scan: .m4a (AAC) reads channels /
	// sample rate / esds bitrate head-only via mkvgo, while .flac/.mp3 (not
	// ISO-BMFF/EBML) and ALAC (dropped sample entry) fall through to ffprobe. The
	// dispatcher is stateless, so a fresh instance over the same ffprobe fallback.
	musicSvc.CommandService().WithProber(proberpkg.New(infra.prober, nil, nil))
	musicSvc.CommandService().WithAlbumArtFetcher(itunesadapter.NewAlbumArtFetcher(infra.metadataHTTP), cfg.Transcoding.CacheDir)
	if infra.musicSearch != nil {
		musicSvc.CommandService().WithSearchIndex(infra.musicSearch)
		musicSvc.QueryService().WithSearchIndex(infra.musicSearch)
	}
	return musicSvc, musicNowPlaying
}

// buildPlaylistServices wires the playlist service (with its media/library
// access checkers) and the smart-playlist generator.
func buildPlaylistServices(infra *infraDeps) (*appplaylist.Service, *appplaylist.GeneratorService) {
	r := infra.repos
	playlistSvc := appplaylist.NewService(r.Playlist, r.PlaylistItem, r.SmartPlaylist, r.MediaItemQuery)
	playlistSvc.WithMusicTrackChecker(musicTrackCheckerAdapter{repo: r.MusicTrack})
	// AddItem resolves the media item (for its library) and gates it on the
	// acting profile's library access, so a profile cannot add - and then read
	// the metadata of - media from a library it has no access to.
	playlistSvc.WithMediaChecker(mediaItemCheckerAdapter{repo: r.MediaItem})
	playlistSvc.WithLibraryAccessChecker(playlistLibraryAccessAdapter{profiles: r.Profile, access: r.LibraryAccess})
	playlistGenSvc := appplaylist.NewGeneratorService(r.MusicTrack, r.MusicPlay, r.Playlist, r.PlaylistItem)
	return playlistSvc, playlistGenSvc
}

// buildSubtitleService wires the OpenSubtitles-backed subtitle service, or nil
// when no API key is configured (the feature stays cleanly disabled).
func buildSubtitleService(ctx context.Context, infra *infraDeps, libQuerySvc *applibrary.QueryService, rootLog *slog.Logger) *appsubtitle.Service {
	r := infra.repos
	osAPIKey := resolveAPIKey(ctx, r.ServerCfg, "api.opensubtitles_key", "EVEY_OPENSUBTITLES_API_KEY")
	if osAPIKey == "" {
		return nil
	}
	osClient := opensubtitles.New(osAPIKey, os.Getenv("EVEY_OPENSUBTITLES_USERNAME"), os.Getenv("EVEY_OPENSUBTITLES_PASSWORD"), infra.metadataHTTP)
	rootLog.Info("opensubtitles integration enabled")
	return appsubtitle.NewService(r.Subtitle, osClient, opensubtitles.NewLibraryMediaGetter(libQuerySvc))
}

// buildBackupServices wires the backup service (filesystem store + pg_dump) and
// its schedule runner.
func buildBackupServices(cfg *config.Config, infra *infraDeps, rootLog *slog.Logger) (*appbackup.Service, *appbackup.Scheduler) {
	r := infra.repos
	bus := infra.bus
	backupSvc := appbackup.NewService(filesystem.NewBackupStore(cfg.Backup.Dir),
		postgres.NewDatabaseDumper(cfg.DB.Host, cfg.DB.Port, cfg.DB.User, cfg.DB.Password, cfg.DB.DBName, cfg.DB.SSLMode),
		version, appbackup.WithEventBus(bus))
	backupSchedulerLog := eveylogger.NewWith(rootLog, "backup-scheduler")
	backupScheduler := appbackup.NewScheduler(r.BackupSchedule, backupSvc, backupSchedulerLog)
	return backupSvc, backupScheduler
}

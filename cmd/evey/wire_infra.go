package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/ffmpeg"
	"github.com/evey-project/evey/internal/adapters/driven/filesystem"
	meili "github.com/evey-project/evey/internal/adapters/driven/meilisearch"
	metadataadapter "github.com/evey-project/evey/internal/adapters/driven/metadata"
	mkvgoadapter "github.com/evey-project/evey/internal/adapters/driven/mkvgo"
	"github.com/evey-project/evey/internal/adapters/driven/pluginrunner"
	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/adapters/driven/search"
	httpapi "github.com/evey-project/evey/internal/adapters/driving/http"
	"github.com/evey-project/evey/internal/config"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/music"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/eventbus"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
)

// wire_infra.go builds the infrastructure layer (infraDeps): DB-backed and
// external adapters (search, prober, packagers, converters, crypto secrets)
// that the app services are composed on top of.

// infraDeps holds infrastructure adapters and low-level dependencies shared across app services.
type infraDeps struct {
	repos                  *postgres.Repos
	searchAdapter          library.SearchIndex
	searchHealth           httpapi.SearchHealthChecker
	musicSearch            music.MusicSearchIndex
	scanner                *filesystem.Scanner
	prober                 *ffmpeg.Prober
	transcoder             *ffmpeg.Transcoder
	hwDetector             *ffmpeg.HWDetector
	trickplayGen           *ffmpeg.TrickplayGenerator
	introDetector          *ffmpeg.IntroDetector
	imageConverter         *ffmpeg.ImageConverter
	mkvAdapter             *mkvgoadapter.Adapter
	onDemandPackager       *mkvgoadapter.OnDemandPackager
	onDemandConcatPackager *mkvgoadapter.ConcatPackager
	bus                    *eventbus.InMemoryEventBus
	hasher                 auth.BcryptHasher
	tokenGen               *auth.JWTGenerator
	hlsAESSecret           []byte
	translator             *eveyi18n.Translator
	metadataHTTP           *http.Client
	tmdbProvider           *metadataadapter.TMDBProvider
	imdbProvider           *metadataadapter.IMDBAPIProvider
	tvmazeProvider         *metadataadapter.TVMazeProvider
	omdbProvider           *metadataadapter.OMDbProvider
	artworkDL              *metadataadapter.ArtworkHTTPDownloader
	personFetcher          *metadataadapter.TMDBPersonFetcher
	pluginRunner           *pluginrunner.Runner
	pluginLogger           *pluginrunner.PluginLogger
	manifestParser         *pluginrunner.YAMLManifestParser
	pluginInstaller        *pluginrunner.ZipInstaller
	pluginAnalyzer         *pluginrunner.StaticAnalyzer
	pluginSigVerifier      *pluginrunner.CosignVerifier
	subScanner             *filesystem.SubtitleScanner
	tmdbKey                string
}

// buildInfra creates repos, infrastructure adapters, and low-level dependencies.
func buildInfra(ctx context.Context, cfg *config.Config, db *sql.DB, rootLog *slog.Logger) (*infraDeps, error) {
	r := postgres.NewRepos(db)

	sqlFallback := postgres.NewSQLSearchFallback(db)
	var searchAdapter library.SearchIndex
	var searchHealth httpapi.SearchHealthChecker
	if sa, err := meili.NewSearchAdapter(cfg.Meilisearch.URL, cfg.Meilisearch.APIKey); err != nil {
		slog.WarnContext(ctx, "meilisearch unavailable, using SQL search fallback", slog.Any("err", err))
		searchAdapter = search.NewResilient(nil, sqlFallback)
	} else {
		searchAdapter = search.NewResilient(sa, sqlFallback).WithQueue(r.PendingSearchWrite)
		searchHealth = sa
	}

	// Music has its own Meilisearch index (dedicated doc shape, library-scoped
	// or global) with the track repo's SQL ILIKE as fallback.
	musicSQLFallback := search.NewMusicRepoFallback(r.MusicTrack)
	var musicSearch music.MusicSearchIndex
	if msa, err := meili.NewMusicSearchAdapter(cfg.Meilisearch.URL, cfg.Meilisearch.APIKey); err != nil {
		slog.WarnContext(ctx, "meilisearch music index unavailable, using SQL search fallback", slog.Any("err", err))
		musicSearch = search.NewResilientMusic(nil, musicSQLFallback)
	} else {
		musicSearch = search.NewResilientMusic(msa, musicSQLFallback)
	}

	scanner := filesystem.NewScanner()
	prober := ffmpeg.NewProber("ffprobe")
	transcoder := ffmpeg.NewTranscoder("ffmpeg", cfg.Transcoding.CacheDir)
	hwDetector := ffmpeg.NewHWDetector("ffmpeg")
	// Boot safety net: warn (once, detached, non-blocking) if this ffmpeg build
	// ships no AV1 decoder, so AV1 source files don't fail playback with a
	// cryptic error on a host whose ffmpeg lacks dav1d.
	go func() {
		probeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ffmpeg.WarnIfNoAV1Decoder(probeCtx, "ffmpeg", slog.Default())
	}()
	trickplayGen := ffmpeg.NewTrickplayGenerator("ffmpeg", "ffprobe")
	introDetector := ffmpeg.NewIntroDetector("ffmpeg", "ffprobe")
	imageConverter := ffmpeg.NewImageConverter("ffmpeg")
	mkvAdapter := mkvgoadapter.New()
	onDemandPackager := mkvgoadapter.NewOnDemandPackager()
	onDemandConcatPackager := mkvgoadapter.NewConcatPackager()
	bus := eventbus.New()
	hasher := auth.BcryptHasher{}

	metadataHTTP := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			DialContext:         (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		},
	}
	tmdbKey := resolveAPIKey(ctx, r.ServerCfg, "api.tmdb_key", "EVEY_TMDB_API_KEY")
	serverCfgForLang := r.ServerCfg
	defaultLang := cfg.Metadata.Language
	// Resolution order: metadata.language override → server.language (hydrates
	// everything when set via /setup or admin) → hardcoded default ("en").
	tmdbProvider := metadataadapter.NewTMDBProviderWithGetter(tmdbKey, func() string {
		bg := context.Background()
		if entry, err2 := serverCfgForLang.Get(bg, "metadata.language"); err2 == nil && entry.Value != "" {
			return entry.Value
		}
		if entry, err2 := serverCfgForLang.Get(bg, "server.language"); err2 == nil && entry.Value != "" {
			return entry.Value
		}
		return defaultLang
	}, metadataHTTP)
	imdbProvider := metadataadapter.NewIMDBAPIProvider(metadataHTTP)
	tvmazeProvider := metadataadapter.NewTVMazeProvider(metadataHTTP)
	omdbKey := resolveAPIKey(ctx, r.ServerCfg, "api.omdb_key", "EVEY_OMDB_API_KEY")
	omdbProvider := metadataadapter.NewOMDbProvider(omdbKey, metadataHTTP)
	// nil → the downloader builds its own ssrf.SafeHTTPClient. Artwork URLs come
	// from provider/plugin responses (e.g. OMDb's Poster passthrough), which are
	// untrusted in Evey's threat model - they must not reach a plain client that
	// can hit loopback/link-local. The host-pinned API providers keep metadataHTTP.
	artworkDownloader := metadataadapter.NewArtworkHTTPDownloader(nil)
	personFetcher := metadataadapter.NewTMDBPersonFetcher(tmdbProvider)

	pluginLogDir := filepath.Join(cfg.Log.Dir, "plugins")
	pluginRetention := cfg.Log.PluginRetentionDays
	if pluginRetention <= 0 {
		pluginRetention = 30
	}
	pluginLogger := pluginrunner.NewPluginLogger(pluginLogDir, pluginRetention)
	// Dev mode can run without /app/libevey-sandbox.so (e.g. on a developer
	// laptop). Production is strict: a plugin without the 'subprocess' perm
	// that can't be sandboxed fails closed.
	//
	// SECURITY: emit a prominent warning when the sandbox bypass is active so
	// that ops can catch a misconfigured production instance immediately.
	if cfg.Log.DevMode {
		// Hard refusal: DevMode disables the sandbox; an operator who also
		// declares EVEY_ENV=production almost certainly mis-set DevMode and
		// would ship an unsandboxed plugin runtime. Fail closed.
		if err := refuseDevModeInProd(os.Getenv("EVEY_ENV")); err != nil {
			return nil, err
		}
		slog.Warn("SECURITY WARNING: plugin sandbox bypass is ENABLED - " +
			"DevMode=true must NEVER be set in production; " +
			"plugins without the 'subprocess' permission will start unsandboxed")
	}
	pluginRunner := pluginrunner.NewRunner().
		WithLogger(pluginLogger).
		WithAllowMissingSandbox(cfg.Log.DevMode).
		WithSandboxMode(cfg.Plugins.SandboxMode).
		WithResourceLimits(cfg.Plugins.MemoryMB*1024*1024, cfg.Plugins.PIDsMax).
		WithCPUNice(cfg.Plugins.CPUNice).
		WithDiskQuota(int64(cfg.Plugins.DiskMaxMB) * 1024 * 1024).
		WithUIDSeparation(cfg.Plugins.UIDSeparation).
		WithEventBus(bus)
	// A previous instance's plugin processes can outlive it (pdeathsig is
	// cleared by the launcher's file caps on older builds, and its delivery
	// races on some kernels); each leaked node runtime holds ~25 MB. Sweep
	// them before AutoStartPlugins spawns fresh ones.
	pluginRunner.ReapStaleProcesses()
	manifestParser := pluginrunner.NewManifestParser()
	pluginInstaller := pluginrunner.NewZipInstaller()
	pluginAnalyzer := pluginrunner.NewStaticAnalyzer()
	pluginSigVerifier := pluginrunner.NewCosignVerifier(cfg.Plugins.CosignPolicy, cfg.Plugins.CosignPublicKey)

	// The path is RELATIVE, and the published image copies the catalogue to
	// /i18n - which only resolves because the runtime stage leaves WorkingDir
	// empty while every other asset sits under /app. One WORKDIR /app added to
	// that stage and the load fails here.
	translator, i18nErr := eveyi18n.NewFromDir("i18n")
	if i18nErr != nil {
		translator = eveyi18n.NewEmpty()
	}
	// Say what an empty catalogue actually costs. The old line said "using
	// defaults" at WARN, which reads like a harmless fallback: there is no
	// default text: every message in the product, on every client, renders as
	// its own key ("errors.auth.invalid_pin"). That shipped once already, in
	// beta.38, and was found from a screenshot.
	if !translator.Loaded() {
		slog.ErrorContext(ctx, "i18n catalogue is EMPTY - every message will render as a raw key on every client",
			slog.String("dir", "i18n"), slog.Any("err", i18nErr))
	}

	privateKey, err := auth.LoadOrGenerateRSAKey(cfg.JWT.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("jwt key: %w", err)
	}
	tokenGen := auth.NewJWTGenerator(privateKey, cfg.JWT.AccessTokenTTL)

	// Secret for deriving per-media AES-128 HLS keys (on-demand encryption,
	// opt-in). Bootstrapped next to the JWT key, same first-run model. A
	// corrupt/unreadable secret must NOT block boot - encryption is an optional
	// hardening, so degrade to "encryption unavailable" (the handler requires a
	// non-empty secret) and let the operator repair the file.
	hlsAESSecret, secErr := auth.LoadOrGenerateSecret(filepath.Join(filepath.Dir(cfg.JWT.PrivateKeyPath), "hls_aes.key"), 32)
	if secErr != nil {
		slog.ErrorContext(ctx, "hls aes secret unavailable - on-demand HLS encryption disabled until the file is repaired or removed",
			slog.Any("err", secErr))
		hlsAESSecret = nil
	}

	subScanner := filesystem.NewSubtitleScanner()

	return &infraDeps{
		repos:                  r,
		searchAdapter:          searchAdapter,
		searchHealth:           searchHealth,
		musicSearch:            musicSearch,
		scanner:                scanner,
		prober:                 prober,
		transcoder:             transcoder,
		hwDetector:             hwDetector,
		trickplayGen:           trickplayGen,
		introDetector:          introDetector,
		imageConverter:         imageConverter,
		mkvAdapter:             mkvAdapter,
		onDemandPackager:       onDemandPackager,
		onDemandConcatPackager: onDemandConcatPackager,
		bus:                    bus,
		hasher:                 hasher,
		tokenGen:               tokenGen,
		hlsAESSecret:           hlsAESSecret,
		translator:             translator,
		metadataHTTP:           metadataHTTP,
		tmdbProvider:           tmdbProvider,
		imdbProvider:           imdbProvider,
		tvmazeProvider:         tvmazeProvider,
		omdbProvider:           omdbProvider,
		artworkDL:              artworkDownloader,
		personFetcher:          personFetcher,
		pluginRunner:           pluginRunner,
		pluginLogger:           pluginLogger,
		manifestParser:         manifestParser,
		pluginInstaller:        pluginInstaller,
		pluginAnalyzer:         pluginAnalyzer,
		pluginSigVerifier:      pluginSigVerifier,
		subScanner:             subScanner,
		tmdbKey:                tmdbKey,
	}, nil
}

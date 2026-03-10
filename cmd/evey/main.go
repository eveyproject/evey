package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	_ "github.com/lib/pq"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	appfeed "github.com/gravity-zero/evey/internal/app/feed"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	appmkv "github.com/gravity-zero/evey/internal/app/mkv"
	appnotification "github.com/gravity-zero/evey/internal/app/notification"
	appplayback "github.com/gravity-zero/evey/internal/app/playback"
	appplugin "github.com/gravity-zero/evey/internal/app/plugin"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	appuser "github.com/gravity-zero/evey/internal/app/user"
	appwebhook "github.com/gravity-zero/evey/internal/app/webhook"
	"github.com/gravity-zero/evey/internal/adapters/driven/ebml"
	"github.com/gravity-zero/evey/internal/adapters/driven/ffmpeg"
	"github.com/gravity-zero/evey/internal/adapters/driven/filesystem"
	meili "github.com/gravity-zero/evey/internal/adapters/driven/meilisearch"
	metadataadapter "github.com/gravity-zero/evey/internal/adapters/driven/metadata"
	"github.com/gravity-zero/evey/internal/adapters/driven/pluginrunner"
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/gravity-zero/evey/internal/config"
	"github.com/gravity-zero/evey/internal/domain/metadata"
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/pkg/eventbus"
	"github.com/gravity-zero/evey/internal/pkg/hdr"
	eveylogger "github.com/gravity-zero/evey/internal/pkg/logger"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	libraryevent "github.com/gravity-zero/evey/internal/domain/library"
	domainmeta "github.com/gravity-zero/evey/internal/domain/metadata"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

const version = "0.1.0"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.Any("err", err))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// --- Logger ---
	logCfg := eveylogger.Config{
		Level:         cfg.Log.Level,
		Dir:           cfg.Log.Dir,
		MaxSizeMB:     cfg.Log.MaxSizeMB,
		RetentionDays: cfg.Log.RetentionDays,
		DevMode:       cfg.Log.DevMode,
	}
	rootLog := eveylogger.New(logCfg)
	slog.SetDefault(rootLog)

	// --- Database ---
	db, err := sql.Open("postgres", cfg.DB.DSN())
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}
	rootLog.Info("connected to postgres")

	if err := postgres.RunMigrations(db); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}

	// --- Repositories ---
	userRepo := postgres.NewUserRepository(db)
	profileRepo := postgres.NewProfileRepository(db)
	refreshTokenRepo := postgres.NewRefreshTokenRepository(db)
	apiKeyRepo := postgres.NewAPIKeyRepository(db)
	inviteLinkRepo := postgres.NewInviteLinkRepository(db)
	quickConnectRepo := postgres.NewQuickConnectRepository(db)
	roleRepo := postgres.NewRoleRepository(db)
	userRoleRepo := postgres.NewUserRoleRepository(db)

	libraryRepo := postgres.NewLibraryRepository(db)
	mediaItemRepo := postgres.NewMediaItemRepository(db)
	mediaStreamRepo := postgres.NewMediaStreamRepository(db)
	personRepo := postgres.NewPersonRepository(db)
	mediaPersonRepo := postgres.NewMediaPersonRepository(db)

	watchProgressRepo := postgres.NewWatchProgressRepository(db)
	playSessionRepo := postgres.NewPlaySessionRepository(db)

	transcodeJobRepo := postgres.NewTranscodeJobRepository(db)
	transcodeProfileRepo := postgres.NewTranscodeProfileRepository(db)

	pluginRepo := postgres.NewPluginRepository(db)
	pluginRuntimeRepo := postgres.NewPluginRuntimeRepository(db)

	metadataOverrideRepo := postgres.NewMetadataOverrideRepository(db)
	artworkRepo := postgres.NewArtworkRepository(db)

	loginHistoryRepo := postgres.NewLoginHistoryRepository(db)
	serverMetricRepo := postgres.NewServerMetricRepository(db)
	auditLogRepo := postgres.NewAuditLogRepository(db)
	playSessionStatsRepo := postgres.NewPlaySessionStatsReader(db)
	playHistoryRepo := postgres.NewPlayHistoryRepository(db)

	notificationRepo := postgres.NewNotificationRepository(db)
	notificationSettingsRepo := postgres.NewNotificationSettingsRepository(db)
	watchHistoryAnalyzer := postgres.NewWatchHistoryAnalyzer(db)
	webhookRepo := postgres.NewWebhookRepository(db)
	webhookDeliveryRepo := postgres.NewWebhookDeliveryRepository(db)
	feedAdminCfgRepo := postgres.NewFeedAdminConfigRepository(db)
	feedProfileCfgRepo := postgres.NewFeedProfileConfigRepository(db)
	feedQuerier := postgres.NewFeedQuerier(db)

	// --- Infrastructure ---
	searchAdapter, _ := meili.NewSearchAdapter(cfg.Meilisearch.URL, cfg.Meilisearch.APIKey)
	scanner := filesystem.NewScanner()
	prober := ffmpeg.NewProber("ffprobe")
	transcoder := ffmpeg.NewTranscoder("ffmpeg", cfg.Transcoding.CacheDir)
	hwDetector := ffmpeg.NewHWDetector("ffmpeg")
	ebmlReader := ebml.NewReader()
	ebmlWriter := ebml.NewWriter("ffmpeg")
	bus := eventbus.New()
	hasher := auth.BcryptHasher{}
	tmdbProvider := metadataadapter.NewTMDBProvider(os.Getenv("EVEY_TMDB_API_KEY"), cfg.Metadata.Language, nil)
	imdbProvider := metadataadapter.NewIMDBAPIProvider(nil)
	artworkDownloader := metadataadapter.NewTMDBArtworkDownloader(nil)
	pluginRunner := pluginrunner.NewRunner()
	manifestParser := pluginrunner.NewManifestParser()

	// --- JWT ---
	privateKey, err := loadOrGenerateRSAKey(cfg.JWT.PrivateKeyPath)
	if err != nil {
		return fmt.Errorf("jwt key: %w", err)
	}
	tokenGen := auth.NewJWTGenerator(privateKey, cfg.JWT.AccessTokenTTL)

	// --- Application services ---
	librarySvc := applibrary.NewService(
		libraryRepo, mediaItemRepo,
		mediaStreamRepo, personRepo, mediaPersonRepo,
		scanner, prober, searchAdapter, bus,
	)

	userSvc := appuser.NewService(
		userRepo,
		profileRepo, refreshTokenRepo, apiKeyRepo,
		inviteLinkRepo, quickConnectRepo, roleRepo, userRoleRepo,
		hasher, tokenGen, bus,
	)

	playbackSvc := appplayback.NewService(
		watchProgressRepo, playSessionRepo, userRepo, bus,
	)

	transcodingSvc := apptranscoding.NewService(
		transcoder, hwDetector,
		transcodeJobRepo, transcodeProfileRepo,
		bus, cfg.Transcoding.MaxConcurrent,
	)

	mkvSvc := appmkv.NewService(ebmlReader, ebmlWriter)

	metadataSvc := appmetadata.NewService(
		metadataOverrideRepo, artworkRepo, artworkDownloader,
		bus, []metadata.MetadataProvider{imdbProvider, tmdbProvider},
	)
	enricher := metadataadapter.NewEnricher(artworkRepo, personRepo, mediaPersonRepo, mediaItemRepo)
	metadataSvc.WithEnricher(enricher, mediaItemRepo)

	activitySvc := appactivity.NewService(
		loginHistoryRepo, serverMetricRepo, auditLogRepo, playSessionStatsRepo, playHistoryRepo,
		bus,
	)

	pluginSvc := appplugin.NewService(
		pluginRepo, pluginRuntimeRepo, pluginRunner, manifestParser,
		bus,
	)

	// --- Metadata enrichment ---
	// Enrich existing items missing overview on startup, then listen for new ones.
	if os.Getenv("EVEY_TMDB_API_KEY") != "" {
		go func() {
			ctx := context.Background()
			items, err := mediaItemRepo.List(ctx, domainlibrary.ListMediaItemsFilter{Limit: 5000})
			if err == nil {
				enricher.AutoEnrichAll(ctx, items, tmdbProvider, rootLog)
			}
		}()
		bus.Subscribe("media.item.added", func(ctx context.Context, ev shared.Event) {
			if e, ok := ev.(libraryevent.MediaItemAdded); ok {
				go func() {
					item, err := mediaItemRepo.GetByID(context.Background(), e.MediaItemID)
					if err == nil {
						enricher.AutoEnrichItem(context.Background(), item, tmdbProvider, rootLog)
					}
				}()
			}
		})
	}

	webhookLog := eveylogger.NewWith(rootLog, "webhook")
	webhookSvc := appwebhook.NewService(webhookRepo, webhookDeliveryRepo, webhookLog, bus)

	notifSvc := appnotification.NewService(
		notificationRepo, notificationSettingsRepo, watchHistoryAnalyzer,
		profileRepo, bus,
	)
	go func() {
		if err := notifSvc.CleanupExpired(context.Background()); err != nil {
			rootLog.Warn("notification cleanup", slog.Any("err", err))
		}
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := notifSvc.CleanupExpired(context.Background()); err != nil {
				rootLog.Warn("notification cleanup", slog.Any("err", err))
			}
		}
	}()

	feedSvc := appfeed.NewService(feedAdminCfgRepo, feedProfileCfgRepo, feedQuerier, watchHistoryAnalyzer)

	// --- Auth interceptor ---
	authInterceptor := interceptors.NewAuthInterceptor(tokenGen)
	grpcLog := eveylogger.NewWith(rootLog, "grpc")

	// --- gRPC server ---
	grpcSrv := grpcapi.NewGRPCServer(
		librarySvc, userSvc, playbackSvc, transcodingSvc,
		mkvSvc, pluginSvc, metadataSvc, activitySvc,
		notifSvc, feedSvc, webhookSvc,
		userRepo, version,
		grpc.ChainUnaryInterceptor(
			interceptors.ClientInfoUnary(),
			interceptors.LoggingUnary(grpcLog),
			authInterceptor.Unary(),
		),
		grpc.ChainStreamInterceptor(
			interceptors.ClientInfoStream(),
			interceptors.LoggingStream(grpcLog),
			authInterceptor.Stream(),
		),
	)
	grpcSrv.RegisterServices()

	grpcAddr := fmt.Sprintf(":%d", cfg.Server.Port)

	// --- gRPC-Gateway HTTP proxy ---
	httpPort := cfg.Server.Port + 1
	gwMux := registerGateway(grpcAddr)
	var httpSrv *http.Server
	if gwMux != nil {
		mux := http.NewServeMux()

		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"ok","version":"` + version + `"}`))
		})
		mux.HandleFunc("/api/v1/admin/fs/browse", browseDirsHandler(tokenGen))
		mh := mediaHandler(tokenGen, librarySvc, artworkRepo)
		mux.HandleFunc("GET /api/v1/media/{id}/stream", mh)
		mux.HandleFunc("GET /api/v1/media/{id}/artwork/{artType}", mh)
		mux.HandleFunc("POST /api/v1/media/{id}/stream", mh)
		mux.HandleFunc("GET /api/v1/media/{id}/download", downloadHandler(tokenGen, librarySvc))
		mux.HandleFunc("GET /api/v1/media/{id}/transcode", transcodeHandler(tokenGen, librarySvc))
		mux.HandleFunc("GET /api/v1/media/{id}/subtitle/{track}", subtitleHandler(tokenGen, librarySvc))
		mux.HandleFunc("GET /api/v1/admin/library-stats", libraryStatsHandler(tokenGen, librarySvc))
		registerAdminRoutes(mux, metadataSvc, tmdbProvider, tokenGen)
		mux.Handle("/", gwMux)
		httpSrv = &http.Server{
			Addr:              fmt.Sprintf(":%d", httpPort),
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
			WriteTimeout:      10 * time.Minute,
			IdleTimeout:       120 * time.Second,
		}
	}

	// --- Graceful shutdown ---
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := refreshTokenRepo.DeleteExpired(context.Background()); err != nil {
					rootLog.Warn("refresh token cleanup", slog.Any("err", err))
				}
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := quickConnectRepo.DeleteExpired(context.Background()); err != nil {
					rootLog.Warn("quick connect cleanup", slog.Any("err", err))
				}
			}
		}
	}()

	errCh := make(chan error, 2)

	go func() {
		if err := grpcSrv.Start(grpcAddr); err != nil {
			errCh <- err
		}
	}()

	if httpSrv != nil {
		go func() {
			rootLog.Info("HTTP gateway listening", slog.Int("port", httpPort))
			if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
		}()
	}

	select {
	case <-ctx.Done():
		rootLog.Info("shutting down...")
	case err := <-errCh:
		return err
	}

	grpcSrv.Stop()
	if httpSrv != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		httpSrv.Shutdown(shutdownCtx)
	}
	rootLog.Info("shutdown complete")
	return nil
}

func mediaHandler(tokens interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}, libSvc interface {
	GetMediaItem(ctx context.Context, id string) (*domainlibrary.MediaItem, error)
}, artworkRepo interface {
	GetByMediaItem(ctx context.Context, mediaItemID string) ([]*domainmeta.ArtworkRecord, error)
}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// routes: GET /api/v1/media/{id}/stream
		//         GET /api/v1/media/{id}/artwork/{artType}
		mediaID := r.PathValue("id")
		artType := r.PathValue("artType") // empty for /stream route
		var action string
		if artType != "" {
			action = "artwork"
		} else {
			action = "stream"
		}

		switch action {
		case "stream":
			authHeader := r.Header.Get("Authorization")
			if len(authHeader) < 8 {
				if t := r.URL.Query().Get("token"); t != "" {
					authHeader = "Bearer " + t
				}
			}
			if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
				http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
				return
			}
			if _, err := tokens.ValidateAccessToken(authHeader[7:]); err != nil {
				http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
				return
			}
			item, err := libSvc.GetMediaItem(r.Context(), mediaID)
			if err != nil {
				http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", "*")
			http.ServeFile(w, r, item.FilePath)

		case "artwork":
			records, err := artworkRepo.GetByMediaItem(r.Context(), mediaID)
			if err != nil || len(records) == 0 {
				http.NotFound(w, r)
				return
			}
			var url string
			for _, rec := range records {
				if artType == "" || rec.Type == artType {
					if rec.SourceURL != nil {
						url = *rec.SourceURL
						break
					}
				}
			}
			if url == "" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", "*")
			http.Redirect(w, r, url, http.StatusFound)

		default:
			http.NotFound(w, r)
		}
	}
}

func downloadHandler(tokens interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}, libSvc interface {
	GetMediaItem(ctx context.Context, id string) (*domainlibrary.MediaItem, error)
}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		token := r.Header.Get("Authorization")
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		} else if t := r.URL.Query().Get("token"); t != "" {
			token = t
		} else {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		if _, err := tokens.ValidateAccessToken(token); err != nil {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		item, err := libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil || item.FilePath == "" {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		fi, err := os.Stat(item.FilePath)
		if err != nil {
			http.Error(w, `{"message":"file not found"}`, http.StatusNotFound)
			return
		}
		filename := filepath.Base(item.FilePath)
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(fi.Size(), 10))
		w.Header().Set("Access-Control-Allow-Origin", "*")
		http.ServeFile(w, r, item.FilePath)
	}
}

func browseDirsHandler(tokens interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}) http.HandlerFunc {
	type entry struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		claims, err := tokens.ValidateAccessToken(authHeader[7:])
		if err != nil || !claims.IsAdmin {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}

		dir := r.URL.Query().Get("path")
		if dir != "" {
			dir = filepath.Clean(dir)
			if !filepath.IsAbs(dir) || !strings.HasPrefix(dir, "/media") {
				http.Error(w, `{"message":"invalid path"}`, http.StatusBadRequest)
				return
			}
		}

		skip := map[string]bool{"cdrom": true, "floppy": true, "usb": true, "sr0": true, "proc": true, "sys": true, "dev": true}
		result := []entry{}

		if dir == "" {
			// List all /media* roots + their subdirs
			roots, _ := filepath.Glob("/media*")
			for _, root := range roots {
				info, err := os.Stat(root)
				if err != nil || !info.IsDir() {
					continue
				}
				name := filepath.Base(root)
				result = append(result, entry{Name: name, Path: root})
				entries, err := os.ReadDir(root)
				if err != nil {
					continue
				}
				for _, e := range entries {
					if !e.IsDir() || e.Name()[0] == '.' || skip[e.Name()] {
						continue
					}
					result = append(result, entry{Name: name + "/" + e.Name(), Path: root + "/" + e.Name()})
				}
			}
		} else {
			base := dir
			if len(base) > 1 && base[len(base)-1] == '/' {
				base = base[:len(base)-1]
			}
			dirEntries, err := os.ReadDir(dir)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte("[]"))
				return
			}
			for _, e := range dirEntries {
				if !e.IsDir() || e.Name()[0] == '.' || skip[e.Name()] {
					continue
				}
				result = append(result, entry{Name: e.Name(), Path: base + "/" + e.Name()})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		jsonEncodeResponse(w, result)
	}
}

func jsonEncodeResponse(w http.ResponseWriter, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"message":"encode error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func loadOrGenerateRSAKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("jwt key at %s is corrupted (invalid PEM)", path)
		}
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("jwt key at %s: %w", path, err)
		}
		if info, err := os.Stat(path); err == nil && info.Mode().Perm()&0077 != 0 {
			slog.Warn("jwt key has loose permissions, fixing to 0600", slog.String("path", path), slog.String("mode", fmt.Sprintf("%04o", info.Mode().Perm())))
			if err := os.Chmod(path, 0600); err != nil {
				return nil, fmt.Errorf("cannot fix jwt key permissions: %w", err)
			}
		}
		return key, nil
	}

	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("cannot create jwt key directory %s: %w", dir, err)
		}
	}

	slog.Info("generating RSA key", slog.String("path", path))
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("cannot save jwt key to %s: %w", path, err)
	}
	defer f.Close()
	pem.Encode(f, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return key, nil
}

func registerAdminRoutes(mux *http.ServeMux, metaSvc *appmetadata.Service, tmdb *metadataadapter.TMDBProvider, tokenGen *auth.JWTGenerator) {
	requireAdmin := func(r *http.Request) bool {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			return false
		}
		claims, err := tokenGen.ValidateAccessToken(strings.TrimPrefix(h, "Bearer "))
		return err == nil && claims.IsAdmin
	}

	// POST /api/v1/admin/media/{id}/refresh-metadata  body: {"tmdb_id":"123","media_type":"movie"}
	mux.HandleFunc("/api/v1/admin/media/", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// route: /api/v1/admin/media/{id}/refresh-metadata  or /api/v1/admin/media/{id}/search-metadata
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/media/"), "/")
		if len(parts) < 2 {
			http.NotFound(w, r)
			return
		}
		mediaItemID := parts[0]
		action := parts[1]

		switch {
		case action == "refresh-metadata" && r.Method == http.MethodPost:
			var body struct {
				TmdbID    string `json:"tmdb_id"`
				ImdbID    string `json:"imdb_id"`
				MediaType string `json:"media_type"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			var refreshErr error
			if body.ImdbID != "" {
				refreshErr = metaSvc.RefreshItemMetadataByIMDB(r.Context(), mediaItemID, body.ImdbID, body.MediaType)
			} else {
				refreshErr = metaSvc.RefreshItemMetadata(r.Context(), mediaItemID, body.TmdbID, body.MediaType)
			}
			if refreshErr != nil {
				slog.Error("refresh metadata", slog.Any("err", refreshErr))
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		case action == "search-metadata" && r.Method == http.MethodGet:
			query := r.URL.Query().Get("query")
			mediaType := r.URL.Query().Get("type")
			if query == "" || mediaType == "" {
				http.Error(w, "query and type required", http.StatusBadRequest)
				return
			}
			results, err := tmdb.Search(r.Context(), query, mediaType)
			if err != nil {
				slog.Error("search metadata", slog.Any("err", err))
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			type searchResult struct {
				TmdbID    string  `json:"tmdb_id"`
				Title     string  `json:"title"`
				Year      *int    `json:"year,omitempty"`
				Overview  string  `json:"overview"`
				PosterURL string  `json:"poster_url,omitempty"`
			}
			out := make([]searchResult, 0, len(results))
			for _, r := range results {
				sr := searchResult{
					Title:    r.Title,
					Year:     r.Year,
					Overview: r.Overview,
				}
				if tmdbID, ok := r.ProviderIDs["tmdb"]; ok {
					sr.TmdbID = tmdbID
				}
				for _, art := range r.Artwork {
					if art.Type == "poster" {
						sr.PosterURL = art.URL
						break
					}
				}
				out = append(out, sr)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(out)

		default:
			http.NotFound(w, r)
		}
	})
}

func registerGateway(grpcAddr string) http.Handler {
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error("grpc-gateway dial failed", slog.Any("err", err))
		return nil
	}
	ctx := context.Background()
	mux := runtime.NewServeMux()
	for _, fn := range []func(context.Context, *runtime.ServeMux, *grpc.ClientConn) error{
		eveyv1.RegisterLibraryServiceHandler,
		eveyv1.RegisterUserServiceHandler,
		eveyv1.RegisterPlaybackServiceHandler,
		eveyv1.RegisterTranscodingServiceHandler,
		eveyv1.RegisterMKVServiceHandler,
		eveyv1.RegisterMetadataServiceHandler,
		eveyv1.RegisterAdminServiceHandler,
		eveyv1.RegisterActivityServiceHandler,
		eveyv1.RegisterNotificationServiceHandler,
		eveyv1.RegisterFeedServiceHandler,
		eveyv1.RegisterWebhookServiceHandler,
	} {
		if err := fn(ctx, mux, conn); err != nil {
			slog.Error("gateway register failed", slog.Any("err", err))
		}
	}
	return mux
}

func transcodeHandler(tokenGen interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}, libSvc interface {
	GetMediaItem(ctx context.Context, id string) (*domainlibrary.MediaItem, error)
	GetMediaStreams(ctx context.Context, mediaItemID string) ([]*domainlibrary.MediaStream, error)
}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		token := r.Header.Get("Authorization")
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		} else if t := r.URL.Query().Get("token"); t != "" {
			token = t
		} else {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		if _, err := tokenGen.ValidateAccessToken(token); err != nil {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		item, err := libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil || item.FilePath == "" {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		audioTrack := 0
		if at := r.URL.Query().Get("audioTrack"); at != "" {
			if n, err2 := strconv.Atoi(at); err2 == nil && n >= 0 {
				audioTrack = n
			}
		}
		videoScale := 0
		if vs := r.URL.Query().Get("videoScale"); vs != "" {
			if n, err2 := strconv.Atoi(vs); err2 == nil && n > 0 {
				videoScale = n
			}
		}
		remuxOnly := r.URL.Query().Get("remux") == "1"
		needsVideoTranscode := false
		needsAudioTranscode := true
		var streams []*domainlibrary.MediaStream
		if s, err := libSvc.GetMediaStreams(r.Context(), mediaID); err == nil {
			streams = s
			for _, s := range streams {
				slog.Debug("transcode: stream", slog.String("media_id", mediaID), slog.String("type", s.StreamType), slog.String("codec", s.Codec), slog.Bool("default", s.IsDefault))
				if s.StreamType == "video" && s.Codec != "h264" && !remuxOnly {
					needsVideoTranscode = true
				}
				if s.StreamType == "audio" && s.IsDefault {
					switch s.Codec {
					case "aac", "mp3", "opus":
						needsAudioTranscode = false
					}
				}
			}
		} else {
			slog.Error("transcode: GetMediaStreams failed", slog.String("media_id", mediaID), slog.Any("err", err))
		}

		hdrInfo := hdr.DetectHDR(streams)
		clientSupportsHDR := r.URL.Query().Get("hdr") == "true"
		needsToneMap := hdrInfo.IsHDR && !clientSupportsHDR
		if needsToneMap {
			needsVideoTranscode = true
		}

		slog.Debug("transcode: decision", slog.String("media_id", mediaID), slog.Bool("needsVideoTranscode", needsVideoTranscode), slog.Bool("needsAudioTranscode", needsAudioTranscode), slog.Bool("remuxOnly", remuxOnly), slog.Bool("needsToneMap", needsToneMap))

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		args := []string{
			"-i", item.FilePath,
			"-map", "0:v:0",
			"-map", fmt.Sprintf("0:a:%d", audioTrack),
		}

		if needsToneMap && !remuxOnly {
			toneMapFilter := hdr.BuildToneMapFilter(hdrInfo, hdr.DefaultToneMapOptions())
			if videoScale > 0 {
				args = append(args, "-vf", fmt.Sprintf("scale=-2:%d,%s", videoScale, toneMapFilter))
			} else {
				args = append(args, "-vf", toneMapFilter)
			}
			args = append(args, "-c:v", "libx264", "-crf", "23", "-preset", "fast")
		} else if videoScale > 0 {
			args = append(args, "-vf", fmt.Sprintf("scale=-2:%d", videoScale), "-c:v", "libx264", "-crf", "23", "-preset", "fast")
		} else if needsVideoTranscode {
			args = append(args, "-c:v", "libx264", "-crf", "23", "-preset", "veryfast", "-tune", "zerolatency", "-threads", "0")
		} else {
			args = append(args, "-c:v", "copy")
		}
		if needsAudioTranscode {
			args = append(args, "-c:a", "aac", "-ac", "2", "-b:a", "192k")
		} else {
			args = append(args, "-c:a", "copy")
		}
		args = append(args, "-movflags", "frag_keyframe+empty_moov+default_base_moof",
			"-f", "mp4", "-")
		slog.Debug("transcode: ffmpeg", slog.String("media_id", mediaID), slog.String("args", strings.Join(args, " ")))
		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			slog.Error("ffmpeg stdout pipe", slog.Any("err", err))
			http.Error(w, "transcode failed", http.StatusInternalServerError)
			return
		}
		if err := cmd.Start(); err != nil {
			slog.Error("ffmpeg start", slog.Any("err", err))
			http.Error(w, "transcode failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Accept-Ranges", "none")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		go func() { <-r.Context().Done(); cancel() }()
		io.Copy(w, stdout)
		if err := cmd.Wait(); err != nil && ctx.Err() == nil {
			slog.Warn("ffmpeg transcode", slog.String("media_id", mediaID), slog.Any("err", err))
		}
	}
}

func subtitleHandler(tokenGen interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}, libSvc interface {
	GetMediaItem(ctx context.Context, id string) (*domainlibrary.MediaItem, error)
}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := r.PathValue("id")
		trackStr := r.PathValue("track")
		token := r.Header.Get("Authorization")
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		} else if t := r.URL.Query().Get("token"); t != "" {
			token = t
		} else {
			http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
			return
		}
		if _, err := tokenGen.ValidateAccessToken(token); err != nil {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		trackIndex, _ := strconv.Atoi(trackStr)
		item, err := libSvc.GetMediaItem(r.Context(), mediaID)
		if err != nil || item.FilePath == "" {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		cmd := exec.CommandContext(ctx, "ffmpeg",
			"-i", item.FilePath,
			"-map", fmt.Sprintf("0:s:%d", trackIndex),
			"-f", "webvtt",
			"-",
		)
		w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		cmd.Stdout = w
		if err := cmd.Start(); err != nil {
			slog.Error("ffmpeg subtitle start", slog.Any("err", err))
			return
		}
		go func() { <-r.Context().Done(); cancel() }()
		if err := cmd.Wait(); err != nil && ctx.Err() == nil {
			slog.Warn("ffmpeg subtitle", slog.String("media_id", mediaID), slog.Any("err", err))
		}
	}
}

func libraryStatsHandler(tokenGen interface {
	ValidateAccessToken(string) (*domainuser.Claims, error)
}, libSvc interface {
	ListLibraries(ctx context.Context) ([]*domainlibrary.Library, error)
}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if strings.HasPrefix(token, "Bearer ") {
			token = token[7:]
		} else if t := r.URL.Query().Get("token"); t != "" {
			token = t
		}
		if _, err := tokenGen.ValidateAccessToken(token); err != nil {
			http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
			return
		}
		libs, err := libSvc.ListLibraries(r.Context())
		if err != nil {
			http.Error(w, `{"message":"internal"}`, http.StatusInternalServerError)
			return
		}
		type libStat struct {
			ID            string  `json:"id"`
			LastScannedAt *string `json:"lastScannedAt"`
		}
		stats := make([]libStat, 0, len(libs))
		for _, lib := range libs {
			var ts *string
			if lib.LastScannedAt != nil {
				s := lib.LastScannedAt.UTC().Format(time.RFC3339)
				ts = &s
			}
			stats = append(stats, libStat{ID: lib.ID, LastScannedAt: ts})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"stats": stats})
	}
}

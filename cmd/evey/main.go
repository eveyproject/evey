package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	_ "github.com/lib/pq"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	appactivity "github.com/gravity-zero/evey/internal/app/activity"
	applibrary "github.com/gravity-zero/evey/internal/app/library"
	appmetadata "github.com/gravity-zero/evey/internal/app/metadata"
	appmkv "github.com/gravity-zero/evey/internal/app/mkv"
	appplayback "github.com/gravity-zero/evey/internal/app/playback"
	appplugin "github.com/gravity-zero/evey/internal/app/plugin"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	appuser "github.com/gravity-zero/evey/internal/app/user"
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
	"github.com/gravity-zero/evey/internal/pkg/auth"
	"github.com/gravity-zero/evey/internal/domain/metadata"
	"github.com/gravity-zero/evey/internal/pkg/eventbus"
)

const version = "0.1.0"

func main() {
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// --- Database ---
	db, err := sql.Open("postgres", cfg.DB.DSN())
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}
	log.Println("connected to postgres")

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

	// --- Infrastructure ---
	searchAdapter, _ := meili.NewSearchAdapter(cfg.Meilisearch.URL, cfg.Meilisearch.APIKey)
	scanner := filesystem.NewScanner()
	transcoder := ffmpeg.NewTranscoder("ffmpeg", cfg.Transcoding.CacheDir)
	hwDetector := ffmpeg.NewHWDetector("ffmpeg")
	ebmlReader := ebml.NewReader()
	ebmlWriter := ebml.NewWriter("ffmpeg")
	bus := eventbus.New()
	hasher := auth.BcryptHasher{}
	tmdbProvider := metadataadapter.NewTMDBProvider(os.Getenv("EVEY_TMDB_API_KEY"), cfg.Metadata.Language, nil)
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
		scanner, searchAdapter, bus,
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
		bus, []metadata.MetadataProvider{tmdbProvider},
	)

	activitySvc := appactivity.NewService(
		loginHistoryRepo, serverMetricRepo, auditLogRepo, playSessionStatsRepo, playHistoryRepo,
		bus,
	)

	pluginSvc := appplugin.NewService(
		pluginRepo, pluginRuntimeRepo, pluginRunner, manifestParser,
		bus,
	)

	// --- Auth interceptor ---
	authInterceptor := interceptors.NewAuthInterceptor(tokenGen)

	// --- gRPC server ---
	grpcSrv := grpcapi.NewGRPCServer(
		librarySvc, userSvc, playbackSvc, transcodingSvc,
		mkvSvc, pluginSvc, metadataSvc, activitySvc,
		userRepo, version,
		grpc.ChainUnaryInterceptor(
			interceptors.LoggingUnary(),
			authInterceptor.Unary(),
		),
		grpc.ChainStreamInterceptor(
			interceptors.LoggingStream(),
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
		httpSrv = &http.Server{
			Addr:    fmt.Sprintf(":%d", httpPort),
			Handler: gwMux,
		}
	}

	// --- Graceful shutdown ---
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 2)

	go func() {
		if err := grpcSrv.Start(grpcAddr); err != nil {
			errCh <- err
		}
	}()

	if httpSrv != nil {
		go func() {
			log.Printf("HTTP gateway listening on :%d", httpPort)
			if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
		}()
	}

	select {
	case <-ctx.Done():
		log.Println("shutting down...")
	case err := <-errCh:
		return err
	}

	grpcSrv.Stop()
	if httpSrv != nil {
		httpSrv.Shutdown(context.Background())
	}
	log.Println("shutdown complete")
	return nil
}

func loadOrGenerateRSAKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		block, _ := pem.Decode(data)
		if block != nil {
			return x509.ParsePKCS1PrivateKey(block.Bytes)
		}
	}
	log.Printf("generating RSA key (no key at %s)", path)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	f, err := os.Create(path)
	if err != nil {
		log.Printf("cannot save key to %s: %v (using ephemeral)", path, err)
		return key, nil
	}
	defer f.Close()
	pem.Encode(f, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return key, nil
}

func registerGateway(grpcAddr string) http.Handler {
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("grpc-gateway dial failed: %v", err)
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
	} {
		if err := fn(ctx, mux, conn); err != nil {
			log.Printf("gateway register failed: %v", err)
		}
	}
	return mux
}

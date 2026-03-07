package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/lib/pq"
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
	"github.com/gravity-zero/evey/internal/adapters/driven/postgres"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi"
	"github.com/gravity-zero/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/gravity-zero/evey/internal/config"
	"github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/gravity-zero/evey/internal/domain/user"
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

	if err := runMigrations(db); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}

	// --- Repositories ---
	userRepo := postgres.NewUserRepository(db)
	libraryRepo := postgres.NewLibraryRepository(db)
	mediaItemRepo := postgres.NewMediaItemRepository(db)
	watchProgressRepo := postgres.NewWatchProgressRepository(db)
	playSessionRepo := postgres.NewPlaySessionRepository(db)

	// --- Infrastructure ---
	searchAdapter := meili.NewSearchAdapter(cfg.Meilisearch.URL, cfg.Meilisearch.APIKey)
	scanner := filesystem.NewScanner()
	transcoder := ffmpeg.NewTranscoder("ffmpeg")
	hwDetector := ffmpeg.NewHWDetector()
	ebmlReader := ebml.NewReader()
	ebmlWriter := ebml.NewWriter()
	eventBus := fakes.NewFakeEventBus()

	// TODO: replace with real JWT-based TokenGenerator using cfg.JWT
	var tokenGen user.TokenGenerator

	// --- Application services ---
	librarySvc := applibrary.NewService(
		libraryRepo, mediaItemRepo,
		nil, nil, nil,
		scanner, searchAdapter, eventBus,
	)

	userSvc := appuser.NewService(
		userRepo,
		nil, nil, nil, nil, nil, nil, nil,
		nil, tokenGen, eventBus,
	)

	playbackSvc := appplayback.NewService(
		watchProgressRepo, playSessionRepo, userRepo, eventBus,
	)

	transcodingSvc := apptranscoding.NewService(
		transcoder, hwDetector,
		nil, nil,
		eventBus, cfg.Transcoding.MaxConcurrent,
	)

	mkvSvc := appmkv.NewService(ebmlReader, ebmlWriter)

	metadataSvc := appmetadata.NewService(
		nil, nil, nil,
		eventBus, nil,
	)

	activitySvc := appactivity.NewService(
		nil, nil, nil, nil,
		eventBus,
	)

	pluginSvc := appplugin.NewService(
		nil, nil, nil, nil,
		eventBus,
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

func runMigrations(db *sql.DB) error {
	data, err := os.ReadFile("internal/adapters/driven/postgres/migrations/000001_init.sql")
	if err != nil {
		log.Printf("migration file not found, skipping: %v", err)
		return nil
	}
	if _, err := db.Exec(string(data)); err != nil {
		return fmt.Errorf("exec migration: %w", err)
	}
	log.Println("migrations applied")
	return nil
}

func registerGateway(grpcAddr string) http.Handler {
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("grpc-gateway dial failed: %v", err)
		return nil
	}
	_ = conn
	// TODO: when generated gw code exists, register handlers:
	//   gw := runtime.NewServeMux()
	//   eveyv1.RegisterLibraryServiceHandler(ctx, gw, conn)
	//   ...
	//   return gw
	return nil
}

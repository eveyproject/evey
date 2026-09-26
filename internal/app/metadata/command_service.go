package metadata

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/domain/shared"
)

type CommandServiceOption func(*CommandService)

func WithEnricher(enricher ItemEnricher, mediaItems library.MediaItemQueryRepo, itemUpdater MediaItemUpdater) CommandServiceOption {
	return func(s *CommandService) {
		s.enricher = enricher
		s.mediaItems = mediaItems
		s.itemUpdater = itemUpdater
	}
}

func WithMediaPersons(repo library.MediaPersonRepository) CommandServiceOption {
	return func(s *CommandService) { s.mediaPersons = repo }
}

type CommandService struct {
	overrides metadata.MetadataOverrideRepository
	// artwork is split into its ADR-0009 command/query halves; both fields are
	// populated from the same concrete repository (see NewCommandService), the
	// split only narrows what each call site in this package can reach.
	artworkCmd   metadata.ArtworkCommandRepo
	artworkQuery metadata.ArtworkQueryRepo
	downloader   metadata.ArtworkDownloader
	events       shared.EventBus
	providers    []metadata.MetadataProvider
	mediaItems   library.MediaItemQueryRepo
	itemUpdater  MediaItemUpdater
	mediaPersons library.MediaPersonRepository
	enricher     ItemEnricher
	serverConfig shared.ServerConfigRepository
	workerWg     sync.WaitGroup
	// shutdownCtx is the service-lifetime context. Set once at construction
	// (race-free), used by detached background jobs (orphan drain) so they are
	// cancelled at shutdown instead of running on context.Background() forever.
	shutdownCtx context.Context

	// auto-download settings. Atomic: SetAutoDownload runs on the server-config
	// event goroutine while enrichment workers read them concurrently during a
	// scan (data race, -race-detectable). Lock-free avoids any ordering issue
	// with the struct's other mutexes.
	autoDownloadEnabled atomic.Bool
	dataDir             atomic.Pointer[string]

	// in-memory job registry for background artwork jobs (poll via GetDownloadJob)
	jobs *downloadJobRegistry
}

// WithShutdownCtx wires the service-lifetime context so detached background
// jobs (the orphan drain) are cancelled at shutdown and awaited by
// WaitWorkers, instead of running on context.Background() invisibly past a
// redeploy against a DB pool the server thinks is drained.
// WithServerConfig enables the resumable cursor of AutoRefreshAllItems: the
// sweep persists its page offset so a server restart mid-walk (the sweep runs
// as a detached goroutine and dies with the process) resumes where it stopped
// on the next trigger instead of re-enriching the whole catalogue.
func WithServerConfig(cfg shared.ServerConfigRepository) CommandServiceOption {
	return func(s *CommandService) { s.serverConfig = cfg }
}

func WithShutdownCtx(ctx context.Context) CommandServiceOption {
	return func(s *CommandService) { s.shutdownCtx = ctx }
}

func WithAutoDownload(dataDir string, enabled bool) CommandServiceOption {
	return func(s *CommandService) {
		s.dataDir.Store(&dataDir)
		s.autoDownloadEnabled.Store(enabled)
	}
}

func NewCommandService(
	overrides metadata.MetadataOverrideRepository,
	artwork metadata.ArtworkRepository,
	downloader metadata.ArtworkDownloader,
	events shared.EventBus,
	providers []metadata.MetadataProvider,
	opts ...CommandServiceOption,
) *CommandService {
	s := &CommandService{
		overrides:    overrides,
		artworkCmd:   artwork,
		artworkQuery: artwork,
		downloader:   downloader,
		events:       events,
		providers:    providers,
		jobs:         newDownloadJobRegistry(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// SetAutoDownload updates the auto-download setting at runtime (e.g. after server-config change).
func (s *CommandService) SetAutoDownload(dataDir string, enabled bool) {
	s.dataDir.Store(&dataDir)
	s.autoDownloadEnabled.Store(enabled)
}

// getDataDir reads the atomic dataDir ("" if never set).
func (s *CommandService) getDataDir() string {
	if p := s.dataDir.Load(); p != nil {
		return *p
	}
	return ""
}

// collectionRelinker is the optional saga-repair capability, asserted rather
// than added to ItemEnricher so the narrow port every fake implements stays
// narrow.
type collectionRelinker interface {
	RelinkCollections(ctx context.Context, limit int) (RelinkReport, error)
}

// RelinkCollections repairs saga membership for movies that belong to no
// collection. Returns an all-zero report when no enricher is wired.
func (s *CommandService) RelinkCollections(ctx context.Context, limit int) (RelinkReport, error) {
	r, ok := s.enricher.(collectionRelinker)
	if !ok {
		return RelinkReport{}, nil
	}
	return r.RelinkCollections(ctx, limit)
}

// publishItemUpdated announces a metadata change a viewer can see (webhook
// media.item.updated), next to the finer-grained metadata.* event.
func (s *CommandService) publishItemUpdated(ctx context.Context, mediaItemID shared.MediaItemID, reason string) {
	evt := library.MediaItemUpdated{MediaItemID: mediaItemID, Reason: reason, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
}

package metadata

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/domain/shared"
)

// PeopleBioEnricher enriches bios for people missing biography data.
type PeopleBioEnricher interface {
	EnrichAllPeopleBios(ctx context.Context, log *slog.Logger) (int, error)
}

type CommandServiceOption func(*CommandService)

func WithEnricher(enricher ItemEnricher, mediaItems library.MediaItemRepository) CommandServiceOption {
	return func(s *CommandService) {
		s.enricher = enricher
		s.mediaItems = mediaItems
	}
}

func WithMediaPersons(repo library.MediaPersonRepository) CommandServiceOption {
	return func(s *CommandService) { s.mediaPersons = repo }
}

type CommandService struct {
	overrides    metadata.MetadataOverrideRepository
	artwork      metadata.ArtworkRepository
	downloader   metadata.ArtworkDownloader
	events       shared.EventBus
	providers    []metadata.MetadataProvider
	mediaItems   library.MediaItemRepository
	mediaPersons library.MediaPersonRepository
	enricher     ItemEnricher
	workerWg     sync.WaitGroup

	// auto-download settings. Atomic: SetAutoDownload runs on the server-config
	// event goroutine while enrichment workers read them concurrently during a
	// scan (data race, -race-detectable). Lock-free avoids any ordering issue
	// with the struct's other mutexes.
	autoDownloadEnabled atomic.Bool
	dataDir             atomic.Pointer[string]

	// in-memory job registry for background artwork jobs (poll via GetDownloadJob)
	jobs *downloadJobRegistry
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
		overrides:  overrides,
		artwork:    artwork,
		downloader: downloader,
		events:     events,
		providers:  providers,
		jobs:       newDownloadJobRegistry(),
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

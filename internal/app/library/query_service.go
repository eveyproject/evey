package library

import (
	"context"
	"errors"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

type QueryServiceOption func(*QueryService)

func WithPeople(people library.PersonQueryRepo) QueryServiceOption {
	return func(s *QueryService) { s.people = people }
}

func WithChapters(chapters library.ChapterQueryRepo) QueryServiceOption {
	return func(s *QueryService) { s.chapters = chapters }
}

func WithTrickplay(trickplay library.TrickplayQueryRepo) QueryServiceOption {
	return func(s *QueryService) { s.trickplay = trickplay }
}

func WithSegments(segments library.MediaSegmentQueryRepo) QueryServiceOption {
	return func(s *QueryService) { s.segments = segments }
}

func WithTranslations(
	mediaTranslations library.MediaItemTranslationQueryRepo,
	personTranslations library.PersonTranslationQueryRepo,
) QueryServiceOption {
	return func(s *QueryService) {
		s.mediaTranslations = mediaTranslations
		s.personTranslations = personTranslations
	}
}

type QueryService struct {
	libraries          library.LibraryQueryRepo
	mediaItems         library.MediaItemQueryRepo
	streams            library.MediaStreamQueryRepo
	search             library.SearchIndex
	people             library.PersonQueryRepo
	mediaTranslations  library.MediaItemTranslationQueryRepo
	personTranslations library.PersonTranslationQueryRepo
	chapters           library.ChapterQueryRepo
	trickplay          library.TrickplayQueryRepo
	segments           library.MediaSegmentQueryRepo
}

func NewQueryService(
	libraries library.LibraryQueryRepo,
	mediaItems library.MediaItemQueryRepo,
	streams library.MediaStreamQueryRepo,
	search library.SearchIndex,
	opts ...QueryServiceOption,
) *QueryService {
	s := &QueryService{
		libraries:  libraries,
		mediaItems: mediaItems,
		streams:    streams,
		search:     search,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *QueryService) GetMediaItemTranslation(ctx context.Context, mediaItemID, locale string) (*library.MediaItemTranslation, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.GetMediaItemTranslation")
	defer span.End()
	if s.mediaTranslations == nil {
		return nil, shared.ErrNotConfigured
	}
	return s.mediaTranslations.GetByMediaItemAndLocale(ctx, mediaItemID, locale)
}

func (s *QueryService) GetPersonTranslation(ctx context.Context, personID, locale string) (*library.PersonTranslation, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.GetPersonTranslation")
	defer span.End()
	if s.personTranslations == nil {
		return nil, shared.ErrNotConfigured
	}
	return s.personTranslations.GetByPersonAndLocale(ctx, personID, locale)
}

func (s *QueryService) GetPerson(ctx context.Context, id string) (*library.Person, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.GetPerson")
	defer span.End()
	if s.people == nil {
		return nil, shared.ErrNotConfigured
	}
	return s.people.GetByID(ctx, id)
}

func (s *QueryService) GetLibrary(ctx context.Context, id string) (*library.Library, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.GetLibrary")
	defer span.End()
	return s.libraries.GetByID(ctx, shared.LibraryID(id))
}

func (s *QueryService) ListLibraries(ctx context.Context) ([]*library.Library, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.ListLibraries")
	defer span.End()
	return s.libraries.List(ctx, library.ListLibrariesFilter{})
}

func (s *QueryService) GetMediaItem(ctx context.Context, id string) (*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.GetMediaItem")
	defer span.End()
	return s.mediaItems.GetByID(ctx, shared.MediaItemID(id))
}

func (s *QueryService) GetMediaItemsByIDs(ctx context.Context, ids []string) ([]*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.GetMediaItemsByIDs")
	defer span.End()
	typedIDs := make([]shared.MediaItemID, len(ids))
	for i, id := range ids {
		typedIDs[i] = shared.MediaItemID(id)
	}
	return s.mediaItems.GetByIDs(ctx, typedIDs)
}

func (s *QueryService) ListMediaItems(ctx context.Context, filter library.ListMediaItemsFilter) ([]*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.ListMediaItems")
	defer span.End()
	return s.mediaItems.List(ctx, filter)
}

func (s *QueryService) CountMediaItems(ctx context.Context, filter library.ListMediaItemsFilter) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.CountMediaItems")
	defer span.End()
	return s.mediaItems.Count(ctx, filter)
}

// ReindexSearch rebuilds the search index from the database for the given
// library (empty = all libraries), without contacting metadata providers.
// Backs POST /api/v1/admin/search/reindex.
func (s *QueryService) ReindexSearch(ctx context.Context, libraryID string, onProgress func(done, total int)) (int, error) {
	return ReindexSearchIndex(ctx, s.mediaItems, s.search, libraryID, onProgress)
}

func (s *QueryService) GetCast(ctx context.Context, mediaItemID string) ([]library.CastMember, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.GetCast")
	defer span.End()
	return s.mediaItems.GetCastByMediaItem(ctx, shared.MediaItemID(mediaItemID))
}

func (s *QueryService) GetCastForItems(ctx context.Context, mediaItemIDs []string) (map[string][]library.CastMember, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.GetCastForItems")
	defer span.End()
	typedIDs := make([]shared.MediaItemID, len(mediaItemIDs))
	for i, id := range mediaItemIDs {
		typedIDs[i] = shared.MediaItemID(id)
	}
	typed, err := s.mediaItems.GetCastForItems(ctx, typedIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]library.CastMember, len(typed))
	for k, v := range typed {
		out[string(k)] = v
	}
	return out, nil
}

func (s *QueryService) GetSeriesAggregate(ctx context.Context, seriesID string) (library.SeriesAggregate, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.GetSeriesAggregate")
	defer span.End()
	return s.mediaItems.SeriesAggregate(ctx, shared.MediaItemID(seriesID))
}

func (s *QueryService) GetMediaStreams(ctx context.Context, mediaItemID string) ([]*library.MediaStream, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.GetMediaStreams")
	defer span.End()
	return s.streams.GetByMediaItemID(ctx, shared.MediaItemID(mediaItemID))
}

func (s *QueryService) GetChapters(ctx context.Context, mediaItemID string) ([]*library.Chapter, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.GetChapters")
	defer span.End()
	if s.chapters == nil {
		return []*library.Chapter{}, nil
	}
	return s.chapters.GetByMediaItemID(ctx, shared.MediaItemID(mediaItemID))
}

func (s *QueryService) GetSegments(ctx context.Context, mediaItemID string) ([]*library.MediaSegment, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.GetSegments")
	defer span.End()
	if s.segments == nil {
		return []*library.MediaSegment{}, nil
	}
	return s.segments.GetByMediaItemID(ctx, shared.MediaItemID(mediaItemID))
}

func (s *QueryService) GetTrickplayManifest(ctx context.Context, mediaItemID string) (*library.TrickplayManifest, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.GetTrickplayManifest")
	defer span.End()
	if s.trickplay == nil {
		return nil, errors.New("trickplay not configured")
	}
	return s.trickplay.GetByMediaItemID(ctx, shared.MediaItemID(mediaItemID))
}

func (s *QueryService) SearchMedia(ctx context.Context, query string, filters map[string]string, sort []string, limit int64) ([]library.SearchHit, int64, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.SearchMedia")
	defer span.End()
	return s.search.Search(ctx, query, filters, sort, limit)
}

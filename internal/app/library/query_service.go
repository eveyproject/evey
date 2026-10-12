package library

import (
	"context"
	"errors"
	"strings"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
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

// WithServerConfig enables config-driven serving behaviour (currently the
// segment confidence gate, see GetSegments). nil keeps the defaults.
func WithServerConfig(cfg shared.ServerConfigRepository) QueryServiceOption {
	return func(s *QueryService) { s.serverConfig = cfg }
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
	serverConfig       shared.ServerConfigRepository
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

// GetMediaItemTranslations batch-fetches one locale's translations for many
// items, keyed by media item ID. Returns an empty map (not an error) when no
// translation repo is configured, so callers can overlay best-effort.
func (s *QueryService) GetMediaItemTranslations(ctx context.Context, mediaItemIDs []string, locale string) (map[string]*library.MediaItemTranslation, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.GetMediaItemTranslations")
	defer span.End()
	if s.mediaTranslations == nil || locale == "" || len(mediaItemIDs) == 0 {
		return map[string]*library.MediaItemTranslation{}, nil
	}
	return s.mediaTranslations.GetByMediaItemsAndLocale(ctx, mediaItemIDs, locale)
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

func (s *QueryService) GetMediaItemByFilePath(ctx context.Context, path string) (*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.GetMediaItemByFilePath")
	defer span.End()
	return s.mediaItems.GetByFilePath(ctx, path)
}

// NextEpisodeOf returns the next episode after the given one in the same
// series - same season's next index_number, or first episode of the next
// season if the current is the season finale. Nil + no error when the
// current episode is the series finale.
func (s *QueryService) NextEpisodeOf(ctx context.Context, episodeID string) (*library.MediaItem, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.NextEpisodeOf")
	defer span.End()
	return s.mediaItems.NextEpisodeOf(ctx, shared.MediaItemID(episodeID))
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

// ListGenres returns the distinct, sorted genres present in a library, folded
// onto one label per genre.
//
// The stored values are whatever each provider returned at scan time, so a
// library that has seen TMDB in two languages plus the OMDB enricher lists the
// same genre several ways - a real one held 28 entries for 20 genres, with
// "Drama" next to "Drame" and IMDb's "Sci-Fi" next to "Science-Fiction". Folding
// happens here rather than in SQL because it is a presentation concern and
// needs the language; the matching side lives in ExpandGenreAliases, and the two
// must stay consistent or the filter panel offers a label that selects nothing.
func (s *QueryService) ListGenres(ctx context.Context, libraryID shared.LibraryID) ([]string, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.ListGenres")
	defer span.End()
	stored, err := s.mediaItems.ListGenres(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	return library.CanonicalGenres(stored, s.displayLanguage(ctx)), nil
}

// displayLanguage is the language genre labels are rendered in. The server
// setting is the right source here: ListGenres feeds a filter panel whose
// options must match what the library pages already display, and those follow
// the server language too.
//
// No configured language returns "", which tells CanonicalGenres to fold
// duplicates WITHOUT translating. Picking a language on the operator's behalf
// would show French to an English server just because that is what the code
// happened to default to. A read failure is treated the same way: a filter panel
// is not worth failing a request over, and not translating is the safe answer.
func (s *QueryService) displayLanguage(ctx context.Context) string {
	if s.serverConfig == nil {
		return ""
	}
	e, err := s.serverConfig.Get(ctx, "server.language")
	if err != nil || e == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(e.Value))
}

// ReindexSearch moved to CommandService (it WRITES the index). The query
// service keeps `search` only for the read-side Search below.

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

// GetStreamsForItems batch-fetches streams for one page of items, keyed by
// item id (batch to avoid N+1, like GetCastForItems).
func (s *QueryService) GetStreamsForItems(ctx context.Context, mediaItemIDs []string) (map[string][]*library.MediaStream, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.GetStreamsForItems")
	defer span.End()
	typedIDs := make([]shared.MediaItemID, 0, len(mediaItemIDs))
	for _, id := range mediaItemIDs {
		typedIDs = append(typedIDs, shared.MediaItemID(id))
	}
	typed, err := s.streams.GetByMediaItemIDs(ctx, typedIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]*library.MediaStream, len(typed))
	for k, v := range typed {
		out[string(k)] = v
	}
	return out, nil
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
	segs, err := s.segments.GetByMediaItemID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil {
		return nil, err
	}
	// Abstention gate: a low-confidence segment is stored (it stays available
	// for eval/training) but not served - a wrong skip button costs more than
	// a missing one. Threshold is admin-tunable; 0 disables the gate.
	cfg := loadIntroConfig(ctx, s.serverConfig)
	minConf := cfg.MinServeConfidence
	// Fresh slice, never segs[:0]: compacting in place would corrupt a shared
	// backing array the day a cache decorator hands the same slice to every
	// caller.
	out := make([]*library.MediaSegment, 0, len(segs))
	for _, sg := range segs {
		// Community rows are never served (user decision 2026-08-25): their
		// timestamps describe someone else's copy of the media, and the gold
		// measure had the same source perfect on one title and 232s off on
		// another. Stored rows stay - the sweep replaces them with file-derived
		// detections and the eval keeps scoring them. All segment types: the
		// someone-else's-copy argument does not care what the segment marks.
		if library.IsCommunityIntroSource(sg.Source) {
			continue
		}
		// Product rule (user decision 2026-08-06): under 5 seconds there is no
		// skip-intro button, whatever the source or confidence. Chapters are
		// exempt from the detection-side floor, so a 3s chapter labelled
		// "intro" reaches storage - this is the one point every player is
		// behind. Not tunable, unlike the confidence gate.
		if sg.Type == "intro" && sg.EndMs-sg.StartMs < minServeIntroMs {
			continue
		}
		// Detector floor: below 12 seconds a DETECTED intro is a false
		// positive, not a short intro. Measured 2026-08-24 on what production
		// actually serves (19 intros, read-only dump): the three shortest are
		// 5 000, 5 347 and 10 000 ms and are exactly the three gold false
		// positives; the next one up is 18 015 ms and is a true positive. The
		// safe margin is [10 001, 18 014] ms - 12 000 sits in the middle.
		// An mkv chapter is exempt: a muxer wrote it, no detector guessed it,
		// and it keeps the absolute 5s floor above.
		if sg.Type == "intro" && sg.Source != "mkv_chapter" && sg.EndMs-sg.StartMs < library.MinDetectedIntroMs {
			continue
		}
		if minConf > 0 && sg.Confidence < minConf {
			continue
		}
		// Credits whose start sits implausibly far from the end of the file are
		// a mid-scene fade the detector called "the credits": serving them
		// raises a skip prompt in the middle of the content, minutes early.
		// Restricted to blackdetect, the ONLY source whose EndMs is the file
		// duration - that invariant is what makes the span the lead, with no
		// extra lookup. A chapter or provider row can end before the file does,
		// where EndMs-StartMs UNDERSTATES the lead and the bound would silently
		// under-police it; those need the real runtime, which this call does not
		// load. 100% of the end_credits measured in prod are blackdetect, so the
		// restriction costs no coverage today. Refusal is at the SERVE only, the
		// row stays stored for eval/training.
		if sg.Type == "end_credits" && sg.Source == "blackdetect" &&
			library.CreditsLeadImplausible(sg.EndMs-sg.StartMs, sg.EndMs, cfg.MaxServeCreditsLeadMs) {
			continue
		}
		out = append(out, sg)
	}
	return out, nil
}

// minServeIntroMs is the serving-side floor on intro length: below it no
// player shows a skip button. Distinct from minHeuristicIntroMs, which polices
// what in-house detectors may EMIT - this one polices what anyone may SEE,
// authoritative sources included.
const minServeIntroMs = 5_000

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

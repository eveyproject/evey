// Package catalog works out what a library is missing: episodes of the
// series it holds and films of the sagas it started, from what the catalog
// source (TMDB) says they contain. The server alone calls the source, at its
// own pace; readers (an admin plugin page) get the worked-out gaps.
package catalog

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

const (
	// refreshAfter is how old a stored answer may get before it is asked
	// again: a series gains episodes weekly at most.
	refreshAfter = 7 * 24 * time.Hour
	// batch is how many series or sagas one pass checks.
	batch = 50
	// pace is the gap between two source calls: the server's own key serves
	// every instance, it must never be hammered.
	pace = 500 * time.Millisecond
	// passEvery is how often the refresher looks for stale answers.
	passEvery = 6 * time.Hour
)

// Service refreshes the stored catalog and works out the gaps.
type Service struct {
	repo   library.CatalogRepo
	source library.CatalogSource
	now    func() time.Time
	sleep  func(context.Context, time.Duration) bool
}

func NewService(repo library.CatalogRepo, source library.CatalogSource) *Service {
	return &Service{repo: repo, source: source, now: time.Now, sleep: sleepCtx}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// Run refreshes stale answers a few minutes after start, then every 6 hours,
// until ctx ends. Without a source (no TMDB key) it does nothing.
func (s *Service) Run(ctx context.Context) {
	if s.source == nil {
		return
	}
	if !s.sleep(ctx, 5*time.Minute) {
		return
	}
	for {
		s.RefreshOnce(ctx)
		if !s.sleep(ctx, passEvery) {
			return
		}
	}
}

// RefreshOnce asks the source about the series and sagas whose stored answer
// is missing or older than a week, one call every half second. A failure is
// logged and retried at the next pass.
func (s *Service) RefreshOnce(ctx context.Context) {
	older := s.now().Add(-refreshAfter)
	series, err := s.repo.SeriesToCheck(ctx, older, batch)
	if err != nil {
		slog.WarnContext(ctx, "catalog: list series to check", slog.Any("err", err))
	}
	for _, sr := range series {
		eps, err := s.source.SeriesEpisodes(ctx, sr.TmdbID)
		if err != nil {
			slog.WarnContext(ctx, "catalog: series episodes", slog.String("series", sr.ItemID), slog.Any("err", err))
		} else if err := s.repo.ReplaceSeriesEpisodes(ctx, sr.ItemID, eps, s.now()); err != nil {
			slog.WarnContext(ctx, "catalog: store series episodes", slog.String("series", sr.ItemID), slog.Any("err", err))
		}
		if !s.sleep(ctx, pace) {
			return
		}
	}
	sagas, err := s.repo.CollectionsToCheck(ctx, older, batch)
	if err != nil {
		slog.WarnContext(ctx, "catalog: list sagas to check", slog.Any("err", err))
	}
	for _, c := range sagas {
		parts, err := s.source.CollectionParts(ctx, c.TmdbID)
		if err != nil {
			slog.WarnContext(ctx, "catalog: saga parts", slog.String("collection", c.ID), slog.Any("err", err))
		} else if err := s.repo.ReplaceCollectionParts(ctx, c.ID, parts, s.now()); err != nil {
			slog.WarnContext(ctx, "catalog: store saga parts", slog.String("collection", c.ID), slog.Any("err", err))
		}
		if !s.sleep(ctx, pace) {
			return
		}
	}
}

// MissingEpisode is one episode the library does not hold.
type MissingEpisode struct {
	Episode int
	Title   string
	AirDate *time.Time
}

// MissingSeason groups a series' missing episodes by season. Whole: not one
// episode of that season is held.
type MissingSeason struct {
	Season   int
	Whole    bool
	Episodes []MissingEpisode
}

// SeriesGaps is what one series misses. FromCatalog: worked out from the
// catalog source; otherwise only from the gaps in the library's own numbering
// (a series the source does not know).
type SeriesGaps struct {
	SeriesID    string
	Title       string
	Year        int
	FromCatalog bool
	Seasons     []MissingSeason
}

// SagaGaps is what one saga misses: its released films the library does not
// hold, the saga having at least one film held.
type SagaGaps struct {
	CollectionID string
	Name         string
	Owned        int
	Missing      []library.CollectionPart
}

// Gaps is the whole report.
type Gaps struct {
	Series []SeriesGaps
	Sagas  []SagaGaps
}

// Gaps works out what the libraries (all when libraryIDs is empty) miss.
// Only what is already out counts: an episode or a film without a date, or
// dated later than today, is not missing yet. Specials (season 0) are left
// out: few libraries keep them and they would drown the rest.
func (s *Service) Gaps(ctx context.Context, libraryIDs []shared.LibraryID) (Gaps, error) {
	today := s.now()
	series, err := s.repo.ListSeries(ctx, libraryIDs)
	if err != nil {
		return Gaps{}, err
	}
	ids := make([]string, len(series))
	for i, sr := range series {
		ids[i] = sr.ItemID
	}
	expected, err := s.repo.ExpectedEpisodes(ctx, ids)
	if err != nil {
		return Gaps{}, err
	}
	owned, err := s.repo.OwnedEpisodes(ctx, ids)
	if err != nil {
		return Gaps{}, err
	}
	var out Gaps
	for _, sr := range series {
		have := map[[2]int]bool{}
		for _, o := range owned[sr.ItemID] {
			have[[2]int{o.Season, o.Episode}] = true
		}
		var g SeriesGaps
		if exp := expected[sr.ItemID]; len(exp) > 0 {
			g = catalogGaps(exp, have, today)
			g.FromCatalog = true
		} else {
			g = numberingGaps(owned[sr.ItemID])
		}
		if len(g.Seasons) == 0 {
			continue
		}
		g.SeriesID, g.Title, g.Year = sr.ItemID, sr.Title, sr.Year
		out.Series = append(out.Series, g)
	}

	sagas, ownedFilms, err := s.repo.SagasOwned(ctx, libraryIDs)
	if err != nil {
		return Gaps{}, err
	}
	cids := make([]string, len(sagas))
	for i, c := range sagas {
		cids[i] = c.ID
	}
	parts, err := s.repo.CollectionParts(ctx, cids)
	if err != nil {
		return Gaps{}, err
	}
	for _, c := range sagas {
		held := map[int]bool{}
		for _, id := range ownedFilms[c.ID] {
			held[id] = true
		}
		var missing []library.CollectionPart
		for _, p := range parts[c.ID] {
			if held[p.TmdbID] || p.ReleaseDate == nil || p.ReleaseDate.After(today) {
				continue
			}
			missing = append(missing, p)
		}
		if len(missing) > 0 {
			out.Sagas = append(out.Sagas, SagaGaps{CollectionID: c.ID, Name: c.Name, Owned: len(held), Missing: missing})
		}
	}
	return out, nil
}

func catalogGaps(expected []library.ExpectedEpisode, have map[[2]int]bool, today time.Time) SeriesGaps {
	bySeason := map[int][]MissingEpisode{}
	total := map[int]int{}
	for _, e := range expected {
		if e.Season <= 0 || e.AirDate == nil || e.AirDate.After(today) {
			continue
		}
		total[e.Season]++
		if !have[[2]int{e.Season, e.Episode}] {
			bySeason[e.Season] = append(bySeason[e.Season], MissingEpisode{Episode: e.Episode, Title: e.Title, AirDate: e.AirDate})
		}
	}
	return SeriesGaps{Seasons: seasonsOf(bySeason, total)}
}

// numberingGaps reads only the library's own numbers: episodes missing below
// the highest held in a season, and seasons missing below the highest held.
func numberingGaps(owned []library.OwnedEpisode) SeriesGaps {
	maxEp := map[int]int{}
	held := map[[2]int]bool{}
	maxSeason := 0
	for _, o := range owned {
		if o.Season <= 0 || o.Episode <= 0 {
			continue
		}
		held[[2]int{o.Season, o.Episode}] = true
		if o.Episode > maxEp[o.Season] {
			maxEp[o.Season] = o.Episode
		}
		if o.Season > maxSeason {
			maxSeason = o.Season
		}
	}
	bySeason := map[int][]MissingEpisode{}
	total := map[int]int{}
	for season := 1; season <= maxSeason; season++ {
		if maxEp[season] == 0 {
			bySeason[season] = nil // a whole season, its size unknown
			total[season] = 0
			continue
		}
		total[season] = maxEp[season]
		for ep := 1; ep <= maxEp[season]; ep++ {
			if !held[[2]int{season, ep}] {
				bySeason[season] = append(bySeason[season], MissingEpisode{Episode: ep})
			}
		}
	}
	return SeriesGaps{Seasons: seasonsOf(bySeason, total)}
}

func seasonsOf(bySeason map[int][]MissingEpisode, total map[int]int) []MissingSeason {
	var out []MissingSeason
	for season, eps := range bySeason {
		whole := total[season] == 0 || len(eps) == total[season]
		if len(eps) == 0 && total[season] != 0 {
			continue
		}
		out = append(out, MissingSeason{Season: season, Whole: whole, Episodes: eps})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Season < out[j].Season })
	return out
}

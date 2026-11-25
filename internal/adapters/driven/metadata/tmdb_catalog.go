package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// seasonsPerCall is how many seasons one /tv call carries: TMDB takes at
// most 20 appended responses.
const seasonsPerCall = 20

var _ library.CatalogSource = (*TMDBProvider)(nil)

type tmdbSeasonEpisodes struct {
	Episodes []struct {
		EpisodeNumber int    `json:"episode_number"`
		SeasonNumber  int    `json:"season_number"`
		Name          string `json:"name"`
		AirDate       string `json:"air_date"`
	} `json:"episodes"`
}

// SeriesEpisodes lists every episode of a series, season by season: one call
// for the seasons, then their episodes 20 seasons per call. Specials (season
// 0) are included; the caller decides what to count.
func (p *TMDBProvider) SeriesEpisodes(ctx context.Context, tmdbSeriesID string) ([]library.ExpectedEpisode, error) {
	ctx, span := tracing.StartSpan(ctx, "metadata.tmdb.SeriesEpisodes")
	defer span.End()
	if _, err := strconv.Atoi(tmdbSeriesID); err != nil {
		return nil, fmt.Errorf("tmdb series id %q: not a number", tmdbSeriesID)
	}
	base := fmt.Sprintf("%s/tv/%s?language=%s", p.baseURL, tmdbSeriesID, url.QueryEscape(p.language()))
	body, err := p.doRequestWithRetry(ctx, base)
	if err != nil {
		return nil, fmt.Errorf("tmdb series request: %w", err)
	}
	var show struct {
		Seasons []struct {
			SeasonNumber int `json:"season_number"`
		} `json:"seasons"`
	}
	if err := json.Unmarshal(body, &show); err != nil {
		return nil, fmt.Errorf("tmdb series unmarshal: %w", err)
	}
	seasons := make([]int, 0, len(show.Seasons))
	for _, s := range show.Seasons {
		seasons = append(seasons, s.SeasonNumber)
	}
	sort.Ints(seasons)

	var out []library.ExpectedEpisode
	for start := 0; start < len(seasons); start += seasonsPerCall {
		chunk := seasons[start:min(start+seasonsPerCall, len(seasons))]
		appends := make([]string, len(chunk))
		for i, n := range chunk {
			appends[i] = "season/" + strconv.Itoa(n)
		}
		u := base + "&append_to_response=" + strings.Join(appends, ",")
		body, err := p.doRequestWithRetry(ctx, u)
		if err != nil {
			return nil, fmt.Errorf("tmdb seasons request: %w", err)
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, fmt.Errorf("tmdb seasons unmarshal: %w", err)
		}
		for _, n := range chunk {
			part, ok := raw["season/"+strconv.Itoa(n)]
			if !ok {
				continue
			}
			var s tmdbSeasonEpisodes
			if err := json.Unmarshal(part, &s); err != nil {
				return nil, fmt.Errorf("tmdb season %d unmarshal: %w", n, err)
			}
			for _, e := range s.Episodes {
				out = append(out, library.ExpectedEpisode{
					Season: n, Episode: e.EpisodeNumber, Title: e.Name, AirDate: parseTMDBDate(e.AirDate),
				})
			}
		}
	}
	return out, nil
}

// CollectionParts lists the films of a saga.
func (p *TMDBProvider) CollectionParts(ctx context.Context, tmdbCollectionID int) ([]library.CollectionPart, error) {
	ctx, span := tracing.StartSpan(ctx, "metadata.tmdb.CollectionParts")
	defer span.End()
	u := fmt.Sprintf("%s/collection/%d?language=%s", p.baseURL, tmdbCollectionID, url.QueryEscape(p.language()))
	body, err := p.doRequestWithRetry(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("tmdb collection request: %w", err)
	}
	var c struct {
		Parts []struct {
			ID          int    `json:"id"`
			Title       string `json:"title"`
			ReleaseDate string `json:"release_date"`
			PosterPath  string `json:"poster_path"`
		} `json:"parts"`
	}
	if err := json.Unmarshal(body, &c); err != nil {
		return nil, fmt.Errorf("tmdb collection unmarshal: %w", err)
	}
	out := make([]library.CollectionPart, 0, len(c.Parts))
	for _, pt := range c.Parts {
		poster := ""
		if pt.PosterPath != "" {
			poster = "https://image.tmdb.org/t/p/w500" + pt.PosterPath
		}
		out = append(out, library.CollectionPart{TmdbID: pt.ID, Title: pt.Title, ReleaseDate: parseTMDBDate(pt.ReleaseDate), PosterURL: poster})
	}
	return out, nil
}

func parseTMDBDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

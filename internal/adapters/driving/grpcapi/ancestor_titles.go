package grpcapi

import (
	"context"
	"log/slog"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/library"
)

// fillAncestorTitles stamps series/season display titles onto converted media
// items: episodes get both, seasons get the series title. One batch fetch per
// ancestor level, shared by every read endpoint that returns episode cards.
// Best-effort: a failed fetch leaves the titles empty instead of failing the
// response.
func fillAncestorTitles(ctx context.Context, fetch func(context.Context, []string) ([]*library.MediaItem, error), items []*eveyv1.MediaItem) {
	parentIDs := make([]string, 0)
	seen := map[string]struct{}{}
	for _, it := range items {
		if it == nil || it.ParentId == "" {
			continue
		}
		if it.Type != eveyv1.MediaItemType_MEDIA_ITEM_TYPE_EPISODE && it.Type != eveyv1.MediaItemType_MEDIA_ITEM_TYPE_SEASON {
			continue
		}
		if _, ok := seen[it.ParentId]; !ok {
			seen[it.ParentId] = struct{}{}
			parentIDs = append(parentIDs, it.ParentId)
		}
	}
	if len(parentIDs) == 0 {
		return
	}
	parents := fetchItemMap(ctx, fetch, parentIDs)

	// The series above each episode's season is one more level up.
	seriesIDs := make([]string, 0)
	seenSeries := map[string]struct{}{}
	for _, it := range items {
		if it == nil || it.Type != eveyv1.MediaItemType_MEDIA_ITEM_TYPE_EPISODE {
			continue
		}
		if season := parents[it.ParentId]; season != nil && season.ParentID != "" {
			if _, ok := seenSeries[season.ParentID]; !ok {
				seenSeries[season.ParentID] = struct{}{}
				seriesIDs = append(seriesIDs, season.ParentID)
			}
		}
	}
	series := fetchItemMap(ctx, fetch, seriesIDs)
	stampAncestorTitles(items, parents, series)
}

// stampAncestorTitles writes the series/season display titles onto already-
// converted items from resolved ancestor maps (season id -> season for
// episodes / series id -> series for seasons, and series id -> series). Callers
// that have already walked the hierarchy for another reason pass their maps
// here instead of re-fetching. The single place the episode/season labelling
// rule lives.
func stampAncestorTitles(items []*eveyv1.MediaItem, seasonOrSeriesParents, series map[string]*library.MediaItem) {
	for _, it := range items {
		if it == nil {
			continue
		}
		switch it.Type {
		case eveyv1.MediaItemType_MEDIA_ITEM_TYPE_EPISODE:
			if season := seasonOrSeriesParents[it.ParentId]; season != nil {
				it.SeasonTitle = season.Title
				if sr := series[season.ParentID]; sr != nil {
					it.SeriesTitle = sr.Title
				}
			}
		case eveyv1.MediaItemType_MEDIA_ITEM_TYPE_SEASON:
			if sr := seasonOrSeriesParents[it.ParentId]; sr != nil {
				it.SeriesTitle = sr.Title
			}
		}
	}
}

// fetchItemMap batch-resolves media items by ID into a lookup map.
func fetchItemMap(ctx context.Context, fetch func(context.Context, []string) ([]*library.MediaItem, error), ids []string) map[string]*library.MediaItem {
	m := map[string]*library.MediaItem{}
	if len(ids) == 0 {
		return m
	}
	mis, err := fetch(ctx, ids)
	if err != nil {
		slog.WarnContext(ctx, "batch fetch media items", slog.Any("err", err))
		return m
	}
	for _, mi := range mis {
		if mi != nil {
			m[string(mi.ID)] = mi
		}
	}
	return m
}
